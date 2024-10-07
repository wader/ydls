package ydls

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wader/goutubedl"
	"github.com/wader/logutils/printwriter"
	"golang.org/x/sync/singleflight"

	"github.com/wader/ydls/internal/ffmpeg"
	"github.com/wader/ydls/internal/id3v2"
	"github.com/wader/ydls/internal/iso639"
	"github.com/wader/ydls/internal/linkicon"
	"github.com/wader/ydls/internal/rereader"
	"github.com/wader/ydls/internal/rss"
	"github.com/wader/ydls/internal/stringprioset"
)

// Printer used for log and debug
type Printer interface {
	Print(v ...interface{})
	Printf(format string, v ...interface{})
}

type nopPrinter struct{}

func (nopPrinter) Print(v ...interface{})                 {}
func (nopPrinter) Printf(format string, v ...interface{}) {}

const maxProbeBytes = 20 * 1024 * 1024

type mediaType uint

const (
	MediaAudio mediaType = iota
	MediaVideo
	MediaUnknown
)

func (m mediaType) String() string {
	switch m {
	case MediaAudio:
		return "audio"
	case MediaVideo:
		return "video"
	default:
		return "unknown"
	}
}

func firstNonEmpty(sl ...string) string {
	for _, s := range sl {
		if s != "" {
			return s
		}
	}
	return ""
}

func metadataFromYoutubeDLInfo(yi goutubedl.Info) ffmpeg.Metadata {
	return ffmpeg.Metadata{
		Artist:  firstNonEmpty(yi.Artist, yi.Series, yi.Channel, yi.Creator, yi.Uploader),
		Title:   firstNonEmpty(yi.Title, yi.AltTitle, yi.Episode, yi.Album, yi.Chapter),
		Comment: yi.Description,
	}
}

func id3v2FramesFromMetadata(m ffmpeg.Metadata, yi goutubedl.Info) []id3v2.Frame {
	frames := []id3v2.Frame{
		&id3v2.TextFrame{ID: "TPE1", Text: m.Artist},
		&id3v2.TextFrame{ID: "TIT2", Text: m.Title},
		&id3v2.COMMFrame{Language: "XXX", Description: "", Text: m.Comment},
	}
	if yi.Duration > 0 {
		frames = append(frames, &id3v2.TextFrame{
			ID:   "TLEN",
			Text: fmt.Sprintf("%d", uint32(yi.Duration*1000)),
		})
	}
	if len(yi.ThumbnailBytes) > 0 {
		frames = append(frames, &id3v2.APICFrame{
			MIMEType:    http.DetectContentType(yi.ThumbnailBytes),
			PictureType: id3v2.PictureTypeOther,
			Description: "",
			Data:        yi.ThumbnailBytes,
		})
	}

	return frames
}

func safeFilename(filename string, ext string) string {
	// some fs has a max 255 bytes length limit
	maxFilenameLen := 255 - (1 + len(ext))
	if len(filename) > maxFilenameLen {
		filename = filename[0:maxFilenameLen]
	}
	r := strings.NewReplacer(`/`, `_`, `\`, `_`)
	return r.Replace(filename) + "." + ext
}

// translate youtube-dl codec name to ffmpeg codec name
func ffmpegCodecFromYDLCodec(c string) (string, bool) {
	codecNameNormalizeMap := map[string]string{
		"none": "",
		"avc1": "h264",
		"mp4a": "aac",
		"mp4v": "h264",
		"h265": "hevc",
		"av01": "av1",
	}

	// "  NAME.something  " -> "name"
	c = strings.Trim(c, " ")
	c = strings.ToLower(c)
	p := strings.SplitN(c, ".", 2)
	c = p[0]

	if n, ok := codecNameNormalizeMap[c]; ok {
		return n, true
	}

	return c, false
}

// guess ffmpeg codecs based on ext
func ffmepgCodecsFromExt(ext string) (acodec string, vcodec string) {
	switch strings.ToLower(ext) {
	case "wav":
		return "wav", ""
	case "mp3":
		return "mp3", ""
	case "ogg":
		return "vorbis", ""
	case "opus":
		return "opus", ""
	case "ogv":
		return "vorbis", "theora"
	case "m4a",
		"aac":
		return "aac", ""
	case "mp4",
		"m4v",
		"mov",
		"3gp":
		return "aac", "h264"
	case "weba":
		return "opus", ""
	case "webm":
		return "opus", "vp9"
	case "flv":
		return "aac", "h264"
	case "mpeg":
		return "mp2", "mpeg2video"
	}
	return "", ""
}

func sortYDLFormats(formats []goutubedl.Format, mediaType mediaType, codecs stringprioset.Set) []goutubedl.Format {
	type sortFormat struct {
		codec  string
		br     float64
		tbr    float64
		format goutubedl.Format
	}
	var sortFormats []sortFormat

	// filter out formats that don't have the media we want
	for _, f := range formats {
		var s sortFormat

		switch mediaType {
		case MediaAudio:
			codec, codecFound := ffmpegCodecFromYDLCodec(f.ACodec)
			if !codecFound {
				codec, _ = ffmepgCodecsFromExt(f.Ext)
			}
			s = sortFormat{
				format: f,
				codec:  codec,
				br:     f.ABR,
				tbr:    f.TBR,
			}
		case MediaVideo:
			codec, codecFound := ffmpegCodecFromYDLCodec(f.VCodec)
			if !codecFound {
				_, codec = ffmepgCodecsFromExt(f.Ext)
			}
			s = sortFormat{
				format: f,
				codec:  codec,
				br:     f.VBR,
				tbr:    f.TBR,
			}
		}

		if s.codec == "" {
			continue
		}

		sortFormats = append(sortFormats, s)
	}

	// sort by has-codec, media bitrate, total bitrate, format id
	sort.Slice(sortFormats, func(i int, j int) bool {
		si := sortFormats[i]
		sj := sortFormats[j]

		// codecs argument will always be only audio or only video codecs
		switch a, b := codecs.Member(si.codec), codecs.Member(sj.codec); {
		case a && !b:
			return true
		case !a && b:
			return false
		}

		switch a, b := si.br, sj.br; {
		case a > b:
			return true
		case a < b:
			return false
		}

		switch a, b := si.tbr, sj.tbr; {
		case a > b:
			return true
		case a < b:
			return false
		}

		return strings.Compare(si.format.FormatID, sj.format.FormatID) > 0
	})

	var sorted []goutubedl.Format
	for _, s := range sortFormats {
		sorted = append(sorted, s.format)
	}

	return sorted
}

type downloadProbeReadCloser struct {
	filter         string
	downloadResult *goutubedl.DownloadResult
	probeInfo      ffmpeg.ProbeInfo
	reader         io.ReadCloser
}

func (d *downloadProbeReadCloser) Read(p []byte) (n int, err error) {
	return d.reader.Read(p)
}

func (d *downloadProbeReadCloser) Close() error {
	d.reader.Close()
	return nil
}

func downloadAndProbeFormat(
	ctx context.Context, ydlResult goutubedl.Result, filter string, debugLog Printer,
) (*downloadProbeReadCloser, error) {
	dr, err := ydlResult.Download(ctx, filter)
	if err != nil {
		return nil, err
	}

	rr := rereader.NewReReadCloser(dr)

	dprc := &downloadProbeReadCloser{
		filter:         filter,
		downloadResult: dr,
		reader:         rr,
	}

	ffprobeStderrPW := printwriter.NewWithPrefix(debugLog, fmt.Sprintf("ffprobe %s stderr> ", filter))
	dprc.probeInfo, err = ffmpeg.Probe(
		ctx,
		ffmpeg.Reader{Reader: io.LimitReader(rr, maxProbeBytes)},
		debugLog,
		ffprobeStderrPW,
	)
	if err != nil {
		rr.Close()
		ffprobeStderrPW.Close()
		return nil, err
	}
	// restart and replay buffer data used when probing
	rr.Restarted = true

	return dprc, nil
}

// YDLS youtubedl downloader with some extras
type YDLS struct {
	Config Config // parsed config
}

// NewFromFile new YDLs using config file
func NewFromFile(configPath string) (YDLS, error) {
	configFile, err := os.Open(configPath)
	if err != nil {
		return YDLS{}, err
	}
	defer configFile.Close()
	config, err := parseConfig(configFile)
	if err != nil {
		return YDLS{}, err
	}

	return YDLS{Config: config}, nil
}

// DownloadOptions dowload options
type DownloadOptions struct {
	RequestOptions RequestOptions
	BaseURL        *url.URL
	DebugLog       Printer
	HTTPClient     *http.Client
	Retries        int
}

// DownloadResult download result
type DownloadResult struct {
	Media    io.ReadCloser
	Filename string
	MIMEType string
	waitCh   chan struct{}
}

// Wait for download resources to cleanup
func (dr DownloadResult) Wait() {
	<-dr.waitCh
}

func chooseCodec(formatCodecs []Codec, optionCodecs []string, probedCodecs []string) Codec {
	findCodec := func(codecs []string) (Codec, bool) {
		for _, c := range codecs {
			for _, fc := range formatCodecs {
				if fc.Name == c {
					return fc, true
				}
			}
		}
		return Codec{}, false
	}

	// prefer option codec, probed codec then first format codec
	if codec, ok := findCodec(optionCodecs); ok {
		return codec
	}
	if codec, ok := findCodec(probedCodecs); ok {
		return codec
	}

	// TODO: could return false if there is no formats but only happens with very weird config

	// default use first codec
	return formatCodecs[0]
}

func codecsFromProbeInfo(pi ffmpeg.ProbeInfo) []string {
	var codecs []string

	if c := pi.AudioCodec(); c != "" {
		codecs = append(codecs, c)
	}
	if c := pi.VideoCodec(); c != "" {
		codecs = append(codecs, c)
	}

	return codecs
}

// Download downloads media from URL using context and makes sure output is in specified format
func (ydls *YDLS) Download(ctx context.Context, options DownloadOptions) (DownloadResult, error) {
	attempts := options.Retries + 1
	var err error
	var dr DownloadResult

	for i := 0; i < attempts; i++ {
		dr, err = ydls.download(ctx, options, i)
		if err == nil || ctx.Err() != nil {
			break
		}
	}
	return dr, err
}

func (ydls *YDLS) download(ctx context.Context, options DownloadOptions, attempt int) (DownloadResult, error) {
	if options.DebugLog == nil {
		options.DebugLog = nopPrinter{}
	}
	if options.HTTPClient == nil {
		options.HTTPClient = http.DefaultClient
	}

	log := options.DebugLog

	log.Printf("URL: %s attempt %d", options.RequestOptions.MediaRawURL, attempt)

	ydlOptions := goutubedl.Options{
		DebugLog:   log,
		HTTPClient: options.HTTPClient,
		StderrFn: func(cmd *exec.Cmd) io.Writer {
			return printwriter.NewWithPrefix(log, fmt.Sprintf("%s stderr> ", filepath.Base(cmd.Args[0])))
		},
		Downloader: ydls.Config.GoutubeDL.Downloader,
	}

	var firstFormats string
	if options.RequestOptions.Format != nil {
		firstFormats, _ = options.RequestOptions.Format.Formats.First()
		if firstFormats == "rss" {
			ydlOptions.Type = goutubedl.TypePlaylist
			ydlOptions.PlaylistEnd = options.RequestOptions.Items
		} else {
			ydlOptions.Type = goutubedl.TypeSingle
			ydlOptions.DownloadThumbnail = true
		}

		if !options.RequestOptions.Format.SubtitleCodecs.Empty() {
			ydlOptions.DownloadSubtitles = true
		}
	}

	ydlResult, err := goutubedl.New(ctx, options.RequestOptions.MediaRawURL, ydlOptions)
	if err != nil {
		log.Printf("Failed to download: %s", err)
		return DownloadResult{}, err
	}

	log.Printf("Title: %s", ydlResult.Info.Title)

	if options.RequestOptions.Format == nil {
		return ydls.downloadRaw(ctx, log, ydlResult)
	} else if firstFormats == "rss" {
		return ydls.downloadRSS(ctx, log, options, ydlResult)
	}

	return ydls.downloadFormat(ctx, log, options, ydlResult)
}

func (ydls *YDLS) downloadRSS(
	ctx context.Context,
	log Printer,
	options DownloadOptions,
	ydlResult goutubedl.Result) (DownloadResult, error) {

	// if no thumbnil try best effort to find a good favicon
	linkIconRawURL := ""
	webpageRawURL := ydlResult.Info.WebpageURL
	if ydlResult.Info.Thumbnail == "" && webpageRawURL != "" {
		resp, respErr := options.HTTPClient.Get(webpageRawURL)
		if respErr == nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			linkIconRawURL, _ = linkicon.Find(webpageRawURL, string(body))
		}
	}

	r, w := io.Pipe()
	waitCh := make(chan struct{})

	// this needs to use a goroutine to have same api as DownloadFormat etc
	go func() {
		_, _ = w.Write([]byte(xml.Header))
		rssRoot := RSSFromYDLSInfo(
			options,
			ydlResult.Info,
			linkIconRawURL,
		)
		feedWriter := xml.NewEncoder(w)
		feedWriter.Indent("", "  ")
		_ = feedWriter.Encode(rssRoot)
		w.Close()
		close(waitCh)
	}()

	return DownloadResult{
		Media:    r,
		MIMEType: rss.MIMEType,
		waitCh:   waitCh,
	}, nil
}

func (ydls *YDLS) downloadRaw(ctx context.Context, debugLog Printer, ydlResult goutubedl.Result) (DownloadResult, error) {
	dprc, err := downloadAndProbeFormat(ctx, ydlResult, "", debugLog)
	if err != nil {
		return DownloadResult{}, err
	}

	dr := DownloadResult{
		waitCh: make(chan struct{}),
	}

	// see if we know about the probed format, otherwise fallback to "raw"
	outFormat, outFormatName := ydls.Config.Formats.FindByFormatCodecs(
		dprc.probeInfo.FormatName(),
		codecsFromProbeInfo(dprc.probeInfo),
	)

	if outFormatName != "" {
		dr.MIMEType = outFormat.MIMEType
		dr.Filename = safeFilename(ydlResult.Info.Title, outFormat.Ext)
	} else {
		outFormatName = "raw"
		dr.MIMEType = "application/octet-stream"
		dr.Filename = safeFilename(ydlResult.Info.Title, "raw")
	}

	log.Printf("Output format: %s (probed %s)", outFormatName, dprc.probeInfo)

	var w io.WriteCloser
	dr.Media, w = io.Pipe()

	go func() {
		n, err := io.Copy(w, dprc)
		dprc.Close()
		w.Close()
		log.Printf("Copy done (n=%v err=%v)", n, err)
		close(dr.waitCh)
	}()

	return dr, nil
}

// TODO: messy, needs refactor
func (ydls *YDLS) downloadFormat(
	ctx context.Context,
	log Printer,
	options DownloadOptions,
	ydlResult goutubedl.Result) (DownloadResult, error) {
	type streamDownloadMap struct {
		stream     Stream
		ydlFormats []goutubedl.Format
		download   *downloadProbeReadCloser
	}

	dr := DownloadResult{
		waitCh: make(chan struct{}),
	}

	var closeOnDone []io.Closer
	var subtitlesTempDir string
	cleanupOnDoneFn := func() {
		for _, c := range closeOnDone {
			c.Close()
		}
		if subtitlesTempDir != "" {
			os.RemoveAll(subtitlesTempDir)
		}
	}
	deferCloseFn := cleanupOnDoneFn
	defer func() {
		// will be nil if cmd starts and goroutine takes care of closing instead
		if deferCloseFn != nil {
			deferCloseFn()
		}
	}()

	dr.MIMEType = options.RequestOptions.Format.MIMEType
	dr.Filename = safeFilename(ydlResult.Info.Title, options.RequestOptions.Format.Ext)

	if options.RequestOptions.Format != nil {
		log.Printf("Output format: %s", options.RequestOptions.Format.Name)
	}

	log.Printf("Available youtubedl formats:")
	for _, f := range ydlResult.Formats() {
		log.Printf("  %s", f)
	}

	log.Printf("Sorted youtubedl formats for streams:")

	streamDownloads := []streamDownloadMap{}
	for _, s := range options.RequestOptions.Format.Streams {
		preferredCodecs := s.CodecNames
		optionsCodecCommon := stringprioset.New(options.RequestOptions.Codecs).Intersect(s.CodecNames)
		if !optionsCodecCommon.Empty() {
			preferredCodecs = optionsCodecCommon
		}

		if ydlFormats := sortYDLFormats(
			ydlResult.Formats(),
			s.Media,
			preferredCodecs,
		); len(ydlFormats) > 0 {
			streamDownloads = append(streamDownloads, streamDownloadMap{
				stream:     s,
				ydlFormats: ydlFormats,
			})

			log.Printf("  %s %s:", s.Media, preferredCodecs)
			for _, ydlFormat := range ydlFormats {
				log.Printf("    %s", ydlFormat)
			}
		} else {
			if s.Required {
				return DownloadResult{}, fmt.Errorf("found no required %s source stream", s.Media)
			}
			log.Printf("Found no optional %s source stream, skipping", s.Media)
		}
	}

	if len(streamDownloads) == 0 {
		return DownloadResult{}, fmt.Errorf("no useful source streams found")
	}

	type downloadProbeResult struct {
		err      error
		download *downloadProbeReadCloser
	}

	downloads := map[string]downloadProbeResult{}
	var downloadsMutex sync.Mutex
	var downloadsWG sync.WaitGroup
	// uses singleflight as more than one stream can select the same formats
	var downloadSFG singleflight.Group

	downloadsWG.Add(len(streamDownloads))
	for _, sd := range streamDownloads {
		go func(ydlFormats []goutubedl.Format) {
			defer downloadsWG.Done()

			for _, ydlFormat := range ydlFormats {
				dprcVal, dprcErr, _ := downloadSFG.Do(ydlFormat.FormatID, func() (interface{}, error) {
					return downloadAndProbeFormat(ctx, ydlResult, ydlFormat.FormatID, log)
				})
				dprc := dprcVal.(*downloadProbeReadCloser)

				downloadsMutex.Lock()
				if _, found := downloads[ydlFormat.FormatID]; !found {
					downloads[ydlFormat.FormatID] = downloadProbeResult{
						download: dprc,
						err:      dprcErr,
					}
				}
				downloadsMutex.Unlock()

				// stop if we found a working format for stream
				if dprcErr == nil {
					break
				}
			}
		}(sd.ydlFormats)
	}
	downloadsWG.Wait()

	for _, d := range downloads {
		if d.err == nil {
			closeOnDone = append(closeOnDone, d.download)
		}
	}

	downloadErrors := map[string]error{}
	streamsReadyCount := 0
	for sdI, sd := range streamDownloads {
		for _, ydlFormat := range sd.ydlFormats {
			dprc := downloads[ydlFormat.FormatID]
			if dprc.err != nil {
				downloadErrors[ydlFormat.FormatID] = dprc.err
				continue
			}
			streamDownloads[sdI].download = dprc.download
			streamsReadyCount++
			break
		}
	}
	if streamsReadyCount != len(streamDownloads) {
		return DownloadResult{}, fmt.Errorf("failed download or probe: %s", downloadErrors)
	}

	log.Printf("Skipped download errors: %v", downloadErrors)

	log.Printf("Stream to format mapping:")

	var ffmpegMaps []ffmpeg.Map
	ffmpegFormatFlags := make([]string, len(options.RequestOptions.Format.FormatFlags))
	copy(ffmpegFormatFlags, options.RequestOptions.Format.FormatFlags)

	for _, sdm := range streamDownloads {
		var ffmpegCodec ffmpeg.Codec

		codec := chooseCodec(
			sdm.stream.Codecs,
			options.RequestOptions.Codecs,
			codecsFromProbeInfo(sdm.download.probeInfo),
		)

		probeAudioCodec := sdm.download.probeInfo.AudioCodec()
		probeVideoCodec := sdm.download.probeInfo.VideoCodec()

		if sdm.stream.Media == MediaAudio && probeAudioCodec != "" {
			if !options.RequestOptions.Retranscode && codec.Name == probeAudioCodec {
				ffmpegCodec = ffmpeg.AudioCodec("copy")
			} else {
				ffmpegCodec = ffmpeg.AudioCodec(firstNonEmpty(ydls.Config.CodecMap[codec.Name], codec.Name))
			}
		} else if sdm.stream.Media == MediaVideo && probeVideoCodec != "" {
			if !options.RequestOptions.Retranscode && codec.Name == probeVideoCodec {
				ffmpegCodec = ffmpeg.VideoCodec("copy")
			} else {
				ffmpegCodec = ffmpeg.VideoCodec(firstNonEmpty(ydls.Config.CodecMap[codec.Name], codec.Name))
			}
		} else {
			if sdm.stream.Required {
				return DownloadResult{}, fmt.Errorf("no media found for required %v stream (%s:%s)",
					sdm.stream.Media, probeAudioCodec, probeVideoCodec)
			}
			log.Printf("No media found for optional %v stream (%s:%s)",
				sdm.stream.Media, probeAudioCodec, probeVideoCodec)
			continue
		}

		ffmpegMaps = append(ffmpegMaps, ffmpeg.Map{
			Input:      ffmpeg.Reader{Reader: sdm.download},
			Specifier:  sdm.stream.Specifier,
			Codec:      ffmpegCodec,
			CodecFlags: codec.Flags,
		})
		ffmpegFormatFlags = append(ffmpegFormatFlags, codec.FormatFlags...)

		log.Printf("  %s (%s) ydl:%s probed:%s -> %s (%s)",
			sdm.stream.Media,
			sdm.stream.Specifier,
			sdm.download.filter,
			sdm.download.probeInfo,
			codec.Name,
			ydls.Config.CodecMap[codec.Name],
		)
	}

	if len(ffmpegMaps) == 0 {
		return DownloadResult{}, fmt.Errorf("no media found")
	}

	if !options.RequestOptions.Format.SubtitleCodecs.Empty() && len(ydlResult.Info.Subtitles) > 0 {
		log.Printf("Subtitles:")

		subtitleFfprobeStderr := printwriter.NewWithPrefix(log, "subtitle ffprobe stderr> ")
		subtitleCount := 0
		for _, subtitles := range ydlResult.Info.Subtitles {
			for _, subtitle := range subtitles {
				subtitleProbeInfo, subtitleProbErr := ffmpeg.Probe(
					ctx,
					ffmpeg.Reader{Reader: bytes.NewReader(subtitle.Bytes)},
					log,
					subtitleFfprobeStderr)

				if subtitleProbErr != nil {
					log.Printf("  %s %s: error skipping: %s", subtitle.Language, subtitle.Ext, subtitleProbErr)
					continue
				}

				// make sure some subtitle was found
				// ffprobe for ffmpeg 5.1 (and later?) only report error but does not exit with non-zero
				subtitleCodecName := subtitleProbeInfo.SubtitleCodec()
				if subtitleCodecName == "" {
					log.Printf("  %s %s: no subtitle stream found, skipping", subtitle.Language, subtitle.Ext)
					continue
				} else {
					log.Printf("  %s %s: probed: %s", subtitle.Language, subtitle.Ext, subtitleCodecName)
				}

				if subtitlesTempDir == "" {
					tempDir, tempDirErr := os.MkdirTemp("", "ydls-subtitle")
					if tempDirErr != nil {
						return DownloadResult{}, fmt.Errorf("failed to create subtitles tempdir: %s", tempDirErr)
					}
					subtitlesTempDir = tempDir
				}

				subtitleFile := filepath.Join(subtitlesTempDir, fmt.Sprintf("%s.%s", subtitle.Language, subtitle.Ext))
				if err := os.WriteFile(subtitleFile, subtitle.Bytes, 0600); err != nil {
					return DownloadResult{}, fmt.Errorf("failed to write subtitle file: %s", err)
				}

				var subtitleCodec ffmpeg.Codec
				if options.RequestOptions.Format.SubtitleCodecs.Member(subtitleCodecName) {
					subtitleCodec = ffmpeg.SubtitleCodec("copy")
				} else {
					firstSubtitleCodecName, _ := options.RequestOptions.Format.SubtitleCodecs.First()
					subtitleCodec = ffmpeg.SubtitleCodec(firstSubtitleCodecName)
				}

				subtitleMap := ffmpeg.Map{
					Input:     ffmpeg.URL(subtitleFile),
					Specifier: "s:0",
					Codec:     subtitleCodec,
				}

				// ffmpeg expects 3 letter iso639 language code
				if longCode, ok := iso639.ShortToLong[subtitle.Language]; ok {
					subtitleMap.CodecFlags = []string{
						fmt.Sprintf("-metadata:s:s:%d", subtitleCount), "language=" + longCode,
					}
				}

				ffmpegMaps = append(ffmpegMaps, subtitleMap)

				subtitleCount++
				break
			}
		}
	} else {
		log.Printf("No subtitles found")
	}

	ffmpegStderrPW := printwriter.NewWithPrefix(log, "ffmpeg stderr> ")
	ffmpegR, ffmpegW := io.Pipe()
	closeOnDone = append(closeOnDone, ffmpegR)

	var inputFlags []string
	var outputFlags []string
	inputFlags = append(inputFlags, ydls.Config.InputFlags...)
	outputFlags = append(outputFlags, ydls.Config.OutputFlags...)

	if !options.RequestOptions.TimeRange.IsZero() {
		if !options.RequestOptions.TimeRange.Start.IsZero() {
			inputFlags = append(inputFlags,
				"-ss", ffmpeg.DurationToPosition(time.Duration(options.RequestOptions.TimeRange.Start)),
			)
		}
		outputFlags = []string{"-to", ffmpeg.DurationToPosition(options.RequestOptions.TimeRange.Duration())}
	}

	metadata := metadataFromYoutubeDLInfo(ydlResult.Info)
	for _, sdm := range streamDownloads {
		metadata = metadata.Merge(sdm.download.probeInfo.Format.Tags)
	}

	firstOutFormat, _ := options.RequestOptions.Format.Formats.First()
	ffmpegP := &ffmpeg.FFmpeg{
		Streams: []ffmpeg.Stream{
			{
				InputFlags:  inputFlags,
				OutputFlags: outputFlags,
				Maps:        ffmpegMaps,
				Format: ffmpeg.Format{
					Name:  firstOutFormat,
					Flags: ffmpegFormatFlags,
				},
				Metadata: metadata,
				Output:   ffmpeg.Writer{Writer: ffmpegW},
			},
		},
		DebugLog: log,
		Stderr:   ffmpegStderrPW,
	}

	if err := ffmpegP.Start(ctx); err != nil {
		return DownloadResult{}, err
	}

	// goroutine will take care of closing
	deferCloseFn = nil

	var w io.WriteCloser
	dr.Media, w = io.Pipe()
	closeOnDone = append(closeOnDone, w)

	go func() {
		// TODO: ffmpeg mp3enc id3 writer does not work with streamed output
		// (id3v2 header length update requires seek)
		if options.RequestOptions.Format.Prepend == "id3v2" {
			_, _ = id3v2.Encode(w, id3v2FramesFromMetadata(metadata, ydlResult.Info))
		}
		log.Printf("Starting to copy")
		n, err := io.Copy(w, ffmpegR)

		log.Printf("Copy ffmpeg done (n=%v err=%v)", n, err)

		cleanupOnDoneFn()
		_ = ffmpegP.Wait()
		ffmpegStderrPW.Close()

		log.Printf("Done")

		close(dr.waitCh)
	}()

	return dr, nil
}
