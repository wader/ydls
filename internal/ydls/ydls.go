package ydls

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wader/ydls/internal/ffmpeg"
	"github.com/wader/ydls/internal/id3v2"
	"github.com/wader/ydls/internal/linkicon"
	"github.com/wader/ydls/internal/rereader"
	"github.com/wader/ydls/internal/rss"
	"github.com/wader/ydls/internal/stringprioset"
	"github.com/wader/ydls/internal/writelogger"
	"github.com/wader/ydls/internal/youtubedl"
)

// Printer used for log and debug
type Printer interface {
	Printf(format string, v ...interface{})
}

type nopPrinter struct{}

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

func metadataFromYoutubeDLInfo(yi youtubedl.Info) ffmpeg.Metadata {
	return ffmpeg.Metadata{
		Artist:  firstNonEmpty(yi.Artist, yi.Creator, yi.Uploader),
		Title:   firstNonEmpty(yi.Title, yi.Episode),
		Comment: yi.Description,
	}
}

func id3v2FramesFromMetadata(m ffmpeg.Metadata, yi youtubedl.Info) []id3v2.Frame {
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

func safeFilename(filename string) string {
	r := strings.NewReplacer(`/`, `_`, `\`, `_`)
	return r.Replace(filename)
}

func findYDLFormat(formats []youtubedl.Format, mediaType mediaType, codecs stringprioset.Set) (youtubedl.Format, bool) {
	var sorted []youtubedl.Format

	// filter out only audio or video formats
	for _, f := range formats {
		if mediaType == MediaAudio && f.NormalizedACodec() == "" ||
			mediaType == MediaVideo && f.NormalizedVCodec() == "" {
			continue
		}

		sorted = append(sorted, f)
	}

	// sort by has-codec, media bitrate, total bitrate, format id
	sort.Slice(sorted, func(i int, j int) bool {
		type order struct {
			codec string
			br    float64
			tbr   float64
			id    string
		}

		fi := sorted[i]
		fj := sorted[j]
		var oi order
		var oj order
		switch mediaType {
		case MediaAudio:
			oi = order{
				codec: fi.NormalizedACodec(),
				br:    fi.ABR,
				tbr:   fi.NormalizedBR(),
				id:    fi.FormatID,
			}
			oj = order{
				codec: fj.NormalizedACodec(),
				br:    fj.ABR,
				tbr:   fj.NormalizedBR(),
				id:    fj.FormatID,
			}
		case MediaVideo:
			oi = order{
				codec: fi.NormalizedVCodec(),
				br:    fi.VBR,
				tbr:   fi.NormalizedBR(),
				id:    fi.FormatID,
			}
			oj = order{
				codec: fj.NormalizedVCodec(),
				br:    fj.VBR,
				tbr:   fj.NormalizedBR(),
				id:    fj.FormatID,
			}
		}

		// codecs argument will always be only audio or only video codecs
		switch a, b := codecs.Member(oi.codec), codecs.Member(oj.codec); {
		case a && !b:
			return true
		case !a && b:
			return false
		}

		switch a, b := oi.br, oj.br; {
		case a > b:
			return true
		case a < b:
			return false
		}

		switch a, b := oi.tbr, oj.tbr; {
		case a > b:
			return true
		case a < b:
			return false
		}

		if strings.Compare(oi.id, oj.id) > 0 {
			return false
		}

		return true
	})

	if len(sorted) > 0 {
		return sorted[0], true
	}

	return youtubedl.Format{}, false
}

type downloadProbeReadCloser struct {
	downloadResult *youtubedl.DownloadResult
	probeInfo      ffmpeg.ProbeInfo
	reader         io.ReadCloser
}

func (d *downloadProbeReadCloser) Read(p []byte) (n int, err error) {
	return d.reader.Read(p)
}

func (d *downloadProbeReadCloser) Close() error {
	d.reader.Close()
	d.downloadResult.Wait()
	return nil
}

func downloadAndProbeFormat(
	ctx context.Context, ydlResult youtubedl.Result, filter string, debugLog Printer,
) (*downloadProbeReadCloser, error) {
	dr, err := ydlResult.Download(ctx, filter)
	if err != nil {
		return nil, err
	}

	rr := rereader.NewReReadCloser(dr.Reader)

	dprc := &downloadProbeReadCloser{
		downloadResult: dr,
		reader:         rr,
	}

	ffprobeStderr := writelogger.New(debugLog, fmt.Sprintf("ffprobe %s stderr> ", filter))
	dprc.probeInfo, err = ffmpeg.Probe(
		ctx,
		ffmpeg.Reader{Reader: io.LimitReader(rr, maxProbeBytes)},
		debugLog,
		ffprobeStderr,
	)
	if err != nil {
		dr.Reader.Close()
		dr.Wait()
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
	if options.DebugLog == nil {
		options.DebugLog = nopPrinter{}
	}
	if options.HTTPClient == nil {
		options.HTTPClient = http.DefaultClient
	}

	log := options.DebugLog

	log.Printf("URL: %s", options.RequestOptions.MediaRawURL)

	ydlOptions := youtubedl.Options{
		DebugLog:   log,
		HTTPClient: options.HTTPClient,
	}

	if options.RequestOptions.Format != nil {
		log.Printf("Output format: %s", options.RequestOptions.Format.Name)
	}

	var firstFormats string
	if options.RequestOptions.Format != nil {
		firstFormats, _ = options.RequestOptions.Format.Formats.First()
		if firstFormats == "rss" {
			ydlOptions.YesPlaylist = true
			ydlOptions.SkipThumbnails = true
			ydlOptions.PlaylistEnd = options.RequestOptions.Items
		}
	}

	ydlResult, err := youtubedl.New(ctx, options.RequestOptions.MediaRawURL, ydlOptions)
	if err != nil {
		log.Printf("Failed to download: %s", err)
		return DownloadResult{}, err
	}

	log.Printf("Title: %s", ydlResult.Info.Title)
	if !ydlOptions.YesPlaylist {
		log.Printf("Available youtubedl formats:")
		for _, f := range ydlResult.Formats() {
			log.Printf("  %s", f)
		}
	}

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
	ydlResult youtubedl.Result) (DownloadResult, error) {

	// if no thumbnil try best effort to find a good favicon
	linkIconRawURL := ""
	webpageRawURL := ydlResult.Info.WebpageURL
	if ydlResult.Info.Thumbnail == "" && webpageRawURL != "" {
		resp, respErr := options.HTTPClient.Get(webpageRawURL)
		if respErr == nil {
			body, _ := ioutil.ReadAll(resp.Body)
			resp.Body.Close()
			linkIconRawURL, _ = linkicon.Find(webpageRawURL, string(body))
		}
	}

	r, w := io.Pipe()
	waitCh := make(chan struct{})

	// this needs to use a goroutine to have same api as DownloadFormat etc
	go func() {
		w.Write([]byte(xml.Header))
		rssRoot := RSSFromYDLSInfo(
			options,
			ydlResult.Info,
			linkIconRawURL,
		)
		feedWriter := xml.NewEncoder(w)
		feedWriter.Indent("", "  ")
		feedWriter.Encode(rssRoot)
		w.Close()
		close(waitCh)
	}()

	return DownloadResult{
		Media:    r,
		MIMEType: rss.MIMEType,
		waitCh:   waitCh,
	}, nil
}

func (ydls *YDLS) downloadRaw(ctx context.Context, debugLog Printer, ydlResult youtubedl.Result) (DownloadResult, error) {
	dprc, err := downloadAndProbeFormat(ctx, ydlResult, "best", debugLog)
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
		dr.Filename = safeFilename(ydlResult.Info.Title + "." + outFormat.Ext)
	} else {
		outFormatName = "raw"
		dr.MIMEType = "application/octet-stream"
		dr.Filename = safeFilename(ydlResult.Info.Title + ".raw")
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
	ydlResult youtubedl.Result) (DownloadResult, error) {
	type streamDownloadMap struct {
		stream    Stream
		ydlFormat youtubedl.Format
		download  *downloadProbeReadCloser
	}

	dr := DownloadResult{
		waitCh: make(chan struct{}),
	}

	var closeOnDone []io.Closer
	closeOnDoneFn := func() {
		for _, c := range closeOnDone {
			c.Close()
		}
	}
	deferCloseFn := closeOnDoneFn
	defer func() {
		// will be nil if cmd starts and goroutine takes care of closing instead
		if deferCloseFn != nil {
			deferCloseFn()
		}
	}()

	dr.MIMEType = options.RequestOptions.Format.MIMEType
	dr.Filename = safeFilename(ydlResult.Info.Title + "." + options.RequestOptions.Format.Ext)

	log.Printf("Best format for streams:")

	streamDownloads := []streamDownloadMap{}
	for _, s := range options.RequestOptions.Format.Streams {
		preferredCodecs := s.CodecNames
		optionsCodecCommon := stringprioset.New(options.RequestOptions.Codecs).Intersect(s.CodecNames)
		if !optionsCodecCommon.Empty() {
			preferredCodecs = optionsCodecCommon
		}

		if ydlFormat, ydlsFormatFound := findYDLFormat(
			ydlResult.Formats(),
			s.Media,
			preferredCodecs,
		); ydlsFormatFound {
			streamDownloads = append(streamDownloads, streamDownloadMap{
				stream:    s,
				ydlFormat: ydlFormat,
			})

			log.Printf("  %s: %s", preferredCodecs, ydlFormat)
		} else {
			return DownloadResult{}, fmt.Errorf("no %s stream found", s.Media)
		}
	}

	uniqueFormatIDs := map[string]bool{}
	for _, sdm := range streamDownloads {
		uniqueFormatIDs[sdm.ydlFormat.FormatID] = true
	}

	type downloadProbeResult struct {
		err      error
		download *downloadProbeReadCloser
	}

	downloads := map[string]downloadProbeResult{}
	var downloadsMutex sync.Mutex
	var downloadsWG sync.WaitGroup

	downloadsWG.Add(len(uniqueFormatIDs))
	for formatID := range uniqueFormatIDs {
		go func(formatID string) {
			dprc, err := downloadAndProbeFormat(ctx, ydlResult, formatID, log)
			downloadsMutex.Lock()
			downloads[formatID] = downloadProbeResult{err: err, download: dprc}
			downloadsMutex.Unlock()
			downloadsWG.Done()
		}(formatID)
	}
	downloadsWG.Wait()

	for _, d := range downloads {
		if d.err == nil {
			closeOnDone = append(closeOnDone, d.download)
		}
	}

	for formatID, d := range downloads {
		// TODO: more than one error?
		if d.err != nil {
			return DownloadResult{}, fmt.Errorf("failed to probe: %s: %s", formatID, d.err)
		}
		if d.download == nil {
			return DownloadResult{}, fmt.Errorf("failed to download: %s", formatID)
		}
	}

	for i, sdm := range streamDownloads {
		streamDownloads[i].download = downloads[sdm.ydlFormat.FormatID].download
	}

	log.Printf("Stream mapping:")

	var ffmpegMaps []ffmpeg.Map
	ffmpegFormatFlags := make([]string, len(options.RequestOptions.Format.FormatFlags))
	copy(ffmpegFormatFlags, options.RequestOptions.Format.FormatFlags)

	for _, sdm := range streamDownloads {
		var ffmpegCodec ffmpeg.Codec
		var codec Codec

		codec = chooseCodec(
			sdm.stream.Codecs,
			options.RequestOptions.Codecs,
			codecsFromProbeInfo(sdm.download.probeInfo),
		)

		if sdm.stream.Media == MediaAudio {
			if !options.RequestOptions.Retranscode && codec.Name == sdm.download.probeInfo.AudioCodec() {
				ffmpegCodec = ffmpeg.AudioCodec("copy")
			} else {
				ffmpegCodec = ffmpeg.AudioCodec(firstNonEmpty(ydls.Config.CodecMap[codec.Name], codec.Name))
			}
		} else if sdm.stream.Media == MediaVideo {
			if !options.RequestOptions.Retranscode && codec.Name == sdm.download.probeInfo.VideoCodec() {
				ffmpegCodec = ffmpeg.VideoCodec("copy")
			} else {
				ffmpegCodec = ffmpeg.VideoCodec(firstNonEmpty(ydls.Config.CodecMap[codec.Name], codec.Name))
			}
		} else {
			return DownloadResult{}, fmt.Errorf("unknown media type %v", sdm.stream.Media)
		}

		ffmpegMaps = append(ffmpegMaps, ffmpeg.Map{
			Input:      ffmpeg.Reader{Reader: sdm.download},
			Specifier:  sdm.stream.Specifier,
			Codec:      ffmpegCodec,
			CodecFlags: codec.Flags,
		})
		ffmpegFormatFlags = append(ffmpegFormatFlags, codec.FormatFlags...)

		log.Printf(" %s ydl:%s probed:%s -> %s (%s)",
			sdm.stream.Specifier,
			sdm.ydlFormat,
			sdm.download.probeInfo,
			codec.Name,
			ydls.Config.CodecMap[codec.Name],
		)
	}

	var ffmpegStderr io.Writer
	ffmpegStderr = writelogger.New(log, "ffmpeg stderr> ")
	ffmpegR, ffmpegW := io.Pipe()
	closeOnDone = append(closeOnDone, ffmpegR)

	var inputFlags []string
	var outputFlags []string
	inputFlags = append(inputFlags, ydls.Config.InputFlags...)

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
			ffmpeg.Stream{
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
		Stderr:   ffmpegStderr,
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
			id3v2.Write(w, id3v2FramesFromMetadata(metadata, ydlResult.Info))
		}
		log.Printf("Starting to copy")
		n, err := io.Copy(w, ffmpegR)

		log.Printf("Copy ffmpeg done (n=%v err=%v)", n, err)

		closeOnDoneFn()
		ffmpegP.Wait()

		log.Printf("Done")

		close(dr.waitCh)
	}()

	return dr, nil
}
