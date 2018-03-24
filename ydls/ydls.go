package ydls

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/wader/ydls/ffmpeg"
	"github.com/wader/ydls/id3v2"
	"github.com/wader/ydls/rereader"
	"github.com/wader/ydls/stringprioset"
	"github.com/wader/ydls/timerange"
	"github.com/wader/ydls/writelogger"
	"github.com/wader/ydls/youtubedl"
)

const maxProbeBytes = 20 * 1024 * 1024

type MediaType uint

const (
	MediaAudio MediaType = iota
	MediaVideo
	MediaUnknown
)

func (m MediaType) String() string {
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

func logOrDiscard(l *log.Logger) *log.Logger {
	if l != nil {
		return l
	}

	return log.New(ioutil.Discard, "", 0)
}

func metadataFromYoutubeDLInfo(yi youtubedl.Info) ffmpeg.Metadata {
	return ffmpeg.Metadata{
		Artist:  firstNonEmpty(yi.Artist, yi.Creator, yi.Uploader),
		Title:   yi.Title,
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
	r := strings.NewReplacer("/", "_", "\\", "_")
	return r.Replace(filename)
}

func findYDLFormat(formats []youtubedl.Format, media MediaType, codecs stringprioset.Set) (youtubedl.Format, bool) {
	var sorted []youtubedl.Format

	// filter out only audio or video formats
	for _, f := range formats {
		if media == MediaAudio && f.NormACodec == "" ||
			media == MediaVideo && f.NormVCodec == "" {
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
		switch media {
		case MediaAudio:
			oi = order{
				codec: fi.NormACodec,
				br:    fi.ABR,
				tbr:   fi.NormBR,
				id:    fi.FormatID,
			}
			oj = order{
				codec: fj.NormACodec,
				br:    fj.ABR,
				tbr:   fj.NormBR,
				id:    fj.FormatID,
			}
		case MediaVideo:
			oi = order{
				codec: fi.NormVCodec,
				br:    fi.VBR,
				tbr:   fi.NormBR,
				id:    fi.FormatID,
			}
			oj = order{
				codec: fj.NormVCodec,
				br:    fj.VBR,
				tbr:   fj.NormBR,
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
	ctx context.Context, ydl youtubedl.Info, filter string, debugLog *log.Logger,
) (*downloadProbeReadCloser, error) {
	log := logOrDiscard(debugLog)

	ydlStderr := writelogger.New(log, fmt.Sprintf("ydl-dl %s stderr> ", filter))
	dr, err := ydl.Download(ctx, filter, ydlStderr)
	if err != nil {
		return nil, err
	}

	rr := rereader.NewReReadCloser(dr.Reader)

	dprc := &downloadProbeReadCloser{
		downloadResult: dr,
		reader:         rr,
	}

	ffprobeStderr := writelogger.New(log, fmt.Sprintf("ffprobe %s stderr> ", filter))
	dprc.probeInfo, err = ffmpeg.Probe(
		ctx,
		ffmpeg.Reader{Reader: io.LimitReader(rr, maxProbeBytes)},
		log,
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
	Config Config
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

// DownloadOptions download options
type DownloadOptions struct {
	URL         string
	Format      string
	Codecs      []string            // force codecs
	Retranscode bool                // force retranscode even if same input codec
	TimeRange   timerange.TimeRange // time range limit
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

// ParseDownloadOptions parse options based on curret config
func (ydls *YDLS) ParseDownloadOptions(url string, formatName string, optStrings []string) (DownloadOptions, error) {
	if formatName == "" {
		return DownloadOptions{
			URL:    url,
			Format: "",
		}, nil
	}

	format, formatFound := ydls.Config.Formats.FindByName(formatName)
	if !formatFound {
		return DownloadOptions{}, fmt.Errorf("unknown format %s", formatName)
	}

	opts := DownloadOptions{
		URL:    url,
		Format: formatName,
	}

	codecNames := map[string]bool{}
	for _, s := range format.Streams {
		for _, c := range s.Codecs {
			codecNames[c.Name] = true
		}
	}

	for _, opt := range optStrings {
		if opt == "retranscode" {
			opts.Retranscode = true
		} else if _, ok := codecNames[opt]; ok {
			opts.Codecs = append(opts.Codecs, opt)
		} else if tr, trErr := timerange.NewFromString(opt); trErr == nil {
			opts.TimeRange = tr
		} else {
			return DownloadOptions{}, fmt.Errorf("unknown opt %s", opt)
		}
	}

	return opts, nil
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
func (ydls *YDLS) Download(ctx context.Context, options DownloadOptions, debugLog *log.Logger) (DownloadResult, error) {
	log := logOrDiscard(debugLog)

	log.Printf("URL: %s", options.URL)
	log.Printf("Output format: %s", options.Format)

	ydlStdout := writelogger.New(log, "ydl-info stdout> ")
	ydl, err := youtubedl.NewFromURL(ctx, options.URL, ydlStdout)
	if err != nil {
		log.Printf("Failed to download: %s", err)
		return DownloadResult{}, err
	}

	log.Printf("Title: %s", ydl.Title)
	log.Printf("Available youtubedl formats:")
	for _, f := range ydl.Formats {
		log.Printf("  %s", f)
	}

	if options.Format == "" {
		return ydls.downloadRaw(ctx, log, ydl)
	}

	return ydls.downloadFormat(ctx, log, options, ydl)
}

func (ydls *YDLS) downloadRaw(ctx context.Context, log *log.Logger, ydl youtubedl.Info) (DownloadResult, error) {
	dprc, err := downloadAndProbeFormat(ctx, ydl, "best", log)
	if err != nil {
		return DownloadResult{}, err
	}

	log.Printf("Probed format %s", dprc.probeInfo)

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
		dr.Filename = safeFilename(ydl.Title + "." + outFormat.Ext)
	} else {
		dr.MIMEType = "application/octet-stream"
		dr.Filename = safeFilename(ydl.Title + ".raw")
	}

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
func (ydls *YDLS) downloadFormat(ctx context.Context, log *log.Logger, options DownloadOptions, ydl youtubedl.Info) (DownloadResult, error) {
	type streamDownloadMap struct {
		stream    Stream
		ydlFormat youtubedl.Format
		download  *downloadProbeReadCloser
	}

	dr := DownloadResult{
		waitCh: make(chan struct{}),
	}

	outFormat, outFormatFound := ydls.Config.Formats.FindByName(options.Format)
	if !outFormatFound {
		return DownloadResult{}, fmt.Errorf("could not find format %s", options.Format)
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

	dr.MIMEType = outFormat.MIMEType
	dr.Filename = safeFilename(ydl.Title + "." + outFormat.Ext)

	log.Printf("Best format for streams:")

	streamDownloads := []streamDownloadMap{}
	for _, s := range outFormat.Streams {
		preferredCodecs := s.CodecNames
		optionsCodecCommon := stringprioset.New(options.Codecs).Intersect(s.CodecNames)
		if !optionsCodecCommon.Empty() {
			preferredCodecs = optionsCodecCommon
		}

		if ydlFormat, ydlsFormatFound := findYDLFormat(
			ydl.Formats,
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
			dprc, err := downloadAndProbeFormat(ctx, ydl, formatID, log)
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
	ffmpegFormatFlags := make([]string, len(outFormat.FormatFlags))
	copy(ffmpegFormatFlags, outFormat.FormatFlags)

	for _, sdm := range streamDownloads {
		var ffmpegCodec ffmpeg.Codec
		var codec Codec

		codec = chooseCodec(
			sdm.stream.Codecs,
			options.Codecs,
			codecsFromProbeInfo(sdm.download.probeInfo),
		)

		if sdm.stream.Media == MediaAudio {
			if !options.Retranscode && codec.Name == sdm.download.probeInfo.AudioCodec() {
				ffmpegCodec = ffmpeg.AudioCodec("copy")
			} else {
				ffmpegCodec = ffmpeg.AudioCodec(firstNonEmpty(ydls.Config.CodecMap[codec.Name], codec.Name))
			}
		} else if sdm.stream.Media == MediaVideo {
			if !options.Retranscode && codec.Name == sdm.download.probeInfo.VideoCodec() {
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

	if !options.TimeRange.IsZero() {
		if options.TimeRange.Start != 0 {
			inputFlags = append(inputFlags, "-ss", ffmpeg.DurationToPosition(options.TimeRange.Start))
		}
		outputFlags = []string{"-to", ffmpeg.DurationToPosition(options.TimeRange.Duration())}
	}

	metadata := metadataFromYoutubeDLInfo(ydl)
	for _, sdm := range streamDownloads {
		metadata = metadata.Merge(sdm.download.probeInfo.Format.Tags)
	}

	firstOutFormat, _ := outFormat.Formats.First()
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
		if outFormat.Prepend == "id3v2" {
			id3v2.Write(w, id3v2FramesFromMetadata(metadata, ydl))
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
