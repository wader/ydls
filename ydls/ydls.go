package ydls

// TODO: messy, needs rewrite

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
	"github.com/wader/ydls/writelogger"
	"github.com/wader/ydls/youtubedl"
)

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

func id3v2FramesFromYoutueDLInfo(i *youtubedl.Info) []id3v2.Frame {
	frames := []id3v2.Frame{
		&id3v2.TextFrame{ID: "TPE1", Text: firstNonEmpty(i.Artist, i.Creator, i.Uploader)},
		&id3v2.TextFrame{ID: "TIT2", Text: i.Title},
		&id3v2.COMMFrame{Language: "XXX", Description: "", Text: i.Description},
	}
	if i.Duration > 0 {
		frames = append(frames, &id3v2.TextFrame{
			ID:   "TLEN",
			Text: fmt.Sprintf("%d", uint32(i.Duration*1000)),
		})
	}
	if len(i.ThumbnailBytes) > 0 {
		frames = append(frames, &id3v2.APICFrame{
			MIMEType:    http.DetectContentType(i.ThumbnailBytes),
			PictureType: id3v2.PictureTypeOther,
			Description: "",
			Data:        i.ThumbnailBytes,
		})
	}

	return frames
}

func safeFilename(filename string) string {
	r := strings.NewReplacer("/", "_", "\\", "_")
	return r.Replace(filename)
}

func findFormat(formats []*youtubedl.Format, protocol string, aCodecs prioStringSet, vCodecs prioStringSet) *youtubedl.Format {
	var matched []*youtubedl.Format

	for _, f := range formats {
		if protocol != "*" && f.Protocol != protocol {
			continue
		}
		if !(aCodecs == nil || (f.NormACodec == "" && aCodecs.empty()) || aCodecs.member(f.NormACodec)) {
			continue
		}
		if !(vCodecs == nil || (f.NormVCodec == "" && vCodecs.empty()) || vCodecs.member(f.NormVCodec)) {
			continue
		}

		matched = append(matched, f)
	}

	sort.Sort(youtubedl.FormatByNormBR(matched))

	if len(matched) > 0 {
		return matched[0]
	}

	return nil
}

func findBestFormats(ydlFormats []*youtubedl.Format, aCodecs prioStringSet, vCodecs prioStringSet) (aFormat *youtubedl.Format, vFormat *youtubedl.Format) {
	type neededFormat struct {
		aCodecs    prioStringSet
		vCodecs    prioStringSet
		aYDLFormat **youtubedl.Format
		vYDLFormat **youtubedl.Format
	}

	var neededFormats []*neededFormat

	// match exactly, both audio/video codecs found or not found
	neededFormats = append(neededFormats, &neededFormat{
		aCodecs,
		vCodecs,
		&aFormat, &vFormat,
	})

	if !aCodecs.empty() {
		// matching audio codec with any video codec
		neededFormats = append(neededFormats, &neededFormat{aCodecs, nil, &aFormat, nil})
		// match any audio codec and no video
		neededFormats = append(neededFormats, &neededFormat{nil, prioStringSet{}, &aFormat, nil})
		// match any audio and video codec
		neededFormats = append(neededFormats, &neededFormat{nil, nil, &aFormat, nil})
	}
	if !vCodecs.empty() {
		// same logic as above
		neededFormats = append(neededFormats, &neededFormat{nil, vCodecs, nil, &vFormat})
		neededFormats = append(neededFormats, &neededFormat{prioStringSet{}, nil, nil, &vFormat})
		neededFormats = append(neededFormats, &neededFormat{nil, nil, nil, &vFormat})
	}

	// TODO: if only audio => stream with lowest video br?

	for _, f := range neededFormats {
		m := findFormat(ydlFormats, "*", f.aCodecs, f.vCodecs)

		if m == nil {
			continue
		}

		if f.aYDLFormat != nil && *f.aYDLFormat == nil && m.NormACodec != "" {
			*f.aYDLFormat = m
		}
		if f.vYDLFormat != nil && *f.vYDLFormat == nil && m.NormVCodec != "" {
			*f.vYDLFormat = m
		}

		if (aCodecs.empty() || aFormat != nil) &&
			(vCodecs.empty() || vFormat != nil) {
			break
		}
	}

	return aFormat, vFormat
}

type downloadProbeReadCloser struct {
	downloadResult *youtubedl.DownloadResult
	probeInfo      *ffmpeg.ProbeInfo
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

func downloadAndProbeFormat(ctx context.Context, ydl *youtubedl.Info, filter string, debugLog *log.Logger) (*downloadProbeReadCloser, error) {
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
	const maxProbeByteSize = 10 * 1024 * 1024
	dprc.probeInfo, err = ffmpeg.Probe(ctx, io.LimitReader(rr, maxProbeByteSize), log, ffprobeStderr)
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

// DownloadOptions optional download options
type DownloadOptions struct {
	DebugLog    *log.Logger
	ForceACodec string
	ForceVCodec string
}

// DownloadResult download result
type DownloadResult struct {
	Media    io.ReadCloser
	Filename string
	MIMEType string
	waitCh   chan struct{}
}

// Wait for download resources to cleanup
func (dr *DownloadResult) Wait() {
	<-dr.waitCh
}

func chooseFormatCodec(formats prioFormatCodecSet, probedCodec string) FormatCodec {
	if codecFormat, ok := formats.findByCodec(probedCodec); ok {
		codecFormat.Codec = "copy"
		return codecFormat
	}

	// TODO: could return false if there is no formats but means very weird format.json
	codecFormat, _ := formats.first()
	return codecFormat
}

func fancyYDLFormatName(ydlFormat *youtubedl.Format) string {
	if ydlFormat == nil {
		return "n/a"
	}
	return ydlFormat.String()
}

// Download downloads media from URL using context and makes sure output is in specified format
func (ydls *YDLS) Download(ctx context.Context, url string, formatName string, options DownloadOptions) (*DownloadResult, error) {
	log := logOrDiscard(options.DebugLog)

	log.Printf("URL: %s", url)
	log.Printf("Output format: %s", formatName)

	var ydlStdout io.Writer
	ydlStdout = writelogger.New(log, "ydl-new stdout> ")
	ydl, err := youtubedl.NewFromURL(ctx, url, ydlStdout)
	if err != nil {
		log.Printf("Failed to download: %s", err)
		return nil, err
	}

	log.Printf("Title: %s", ydl.Title)
	log.Printf("Available youtubedl formats:")
	for _, f := range ydl.Formats {
		log.Printf("  %s", f)
	}

	dr := &DownloadResult{
		waitCh: make(chan struct{}, 1),
	}

	if formatName == "" {
		dprc, err := downloadAndProbeFormat(ctx, ydl, "best", log)
		if err != nil {
			return nil, err
		}

		log.Printf("Probed format %s", dprc.probeInfo)

		// see if we know about the probed format, otherwise fallback to "raw"
		outFormat := ydls.Config.Formats.Find(dprc.probeInfo.FormatName(), dprc.probeInfo.ACodec(), dprc.probeInfo.VCodec())
		if outFormat != nil {
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

	outFormat := ydls.Config.Formats.FindByName(options.Format)
	if outFormat == nil {
		return nil, fmt.Errorf("could not find format")
	}

	dr.MIMEType = outFormat.MIMEType
	dr.Filename = safeFilename(ydl.Title + "." + outFormat.Ext)

	aCodecs := outFormat.ACodecs.PrioStringSet()
	vCodecs := outFormat.VCodecs.PrioStringSet()

	if options.ForceACodec != "" {
		aCodecs = aCodecs.NewAndMoveFirst(options.ForceACodec)
		if aCodecs == nil {
			return nil, fmt.Errorf("could not find audio codec \"%s\" for format %s", options.ForceACodec, outFormat.Name)
		}
	}
	if options.ForceVCodec != "" {
		vCodecs = vCodecs.NewAndMoveFirst(options.ForceVCodec)
		if vCodecs == nil {
			return nil, fmt.Errorf("could not find video codec \"%s\" for format %s", options.ForceVCodec, outFormat.Name)
		}
	}

	aYDLFormat, vYDLFormat := findBestFormats(ydl.Formats, aCodecs, vCodecs)

	log.Printf("Best format %s (%s) %s (%s)", aYDLFormat, aCodecs, vYDLFormat, vCodecs)

	var aDprc *downloadProbeReadCloser
	var aErr error
	var vDprc *downloadProbeReadCloser
	var vErr error

	if aYDLFormat != nil && vYDLFormat != nil {
		if aYDLFormat != vYDLFormat {
			// audio and video in different formats, download simultaneously
			var probeWG sync.WaitGroup
			probeWG.Add(2)
			go func() {
				defer probeWG.Done()
				aDprc, aErr = downloadAndProbeFormat(ctx, ydl, aYDLFormat.FormatID, log)
			}()
			go func() {
				defer probeWG.Done()
				vDprc, vErr = downloadAndProbeFormat(ctx, ydl, vYDLFormat.FormatID, log)
			}()
			probeWG.Wait()
			if aDprc != nil {
				closeOnDone = append(closeOnDone, aDprc)
			}
			if vDprc != nil {
				closeOnDone = append(closeOnDone, vDprc)
			}
		} else {
			// audio and video in same format
			aDprc, aErr = downloadAndProbeFormat(ctx, ydl, aYDLFormat.FormatID, log)
			vDprc, vErr = aDprc, aErr
			if aDprc != nil {
				closeOnDone = append(closeOnDone, aDprc)
			}
		}
	} else if aYDLFormat != nil && vYDLFormat == nil {
		// only audio format
		aDprc, aErr = downloadAndProbeFormat(ctx, ydl, aYDLFormat.FormatID, log)
		if aDprc != nil {
			closeOnDone = append(closeOnDone, aDprc)
		}
	} else {
		// don't know, download and probe
		aDprc, aErr = downloadAndProbeFormat(ctx, ydl, "best", log)
		vDprc, vErr = aDprc, aErr
		if aDprc != nil {
			closeOnDone = append(closeOnDone, aDprc)
		}
	}
	if aErr != nil || vErr != nil {
		return nil, fmt.Errorf("failed to probe")
	}

	log.Printf("Stream mapping:")

	var streamMaps []ffmpeg.StreamMap
	ffmpegFormatFlags := make([]string, len(outFormat.FormatFlags))
	copy(ffmpegFormatFlags, outFormat.FormatFlags)

	if len(outFormat.ACodecs) > 0 && aDprc.probeInfo != nil && aDprc.probeInfo.ACodec() != "" {
		codecFormat := chooseFormatCodec(outFormat.ACodecs, aDprc.probeInfo.ACodec())
		streamMaps = append(streamMaps, ffmpeg.StreamMap{
			Reader:     aDprc,
			Specifier:  "a:0",
			Codec:      "acodec:" + codecFormat.Codec,
			CodecFlags: codecFormat.CodecFlags,
		})
		ffmpegFormatFlags = append(ffmpegFormatFlags, codecFormat.FormatFlags...)

		log.Printf("  audio %s probed:%s -> %s",
			fancyYDLFormatName(aYDLFormat),
			aDprc.probeInfo,
			codecFormat.Codec,
		)
	}
	if len(outFormat.VCodecs) > 0 && vDprc.probeInfo != nil && vDprc.probeInfo.VCodec() != "" {
		codecFormat := chooseFormatCodec(outFormat.VCodecs, vDprc.probeInfo.VCodec())
		streamMaps = append(streamMaps, ffmpeg.StreamMap{
			Reader:     vDprc,
			Specifier:  "v:0",
			Codec:      "vcodec:" + codecFormat.Codec,
			CodecFlags: codecFormat.CodecFlags,
		})
		ffmpegFormatFlags = append(ffmpegFormatFlags, codecFormat.FormatFlags...)

		log.Printf("  video %s probed:%s -> %s",
			fancyYDLFormatName(vYDLFormat),
			vDprc.probeInfo,
			codecFormat.Codec,
		)
	}

	var ffmpegStderr io.Writer
	ffmpegStderr = writelogger.New(log, "ffmpeg stderr> ")
	ffmpegR, ffmpegW := io.Pipe()
	closeOnDone = append(closeOnDone, ffmpegR)

	ffmpegP := &ffmpeg.FFmpeg{
		InputFlags: ydls.Config.InputFlags,
		StreamMaps: streamMaps,
		Format:     ffmpeg.Format{Name: outFormat.Formats.first(), Flags: ffmpegFormatFlags},
		DebugLog:   log,
		Stdout:     ffmpegW,
		Stderr:     ffmpegStderr,
	}

	if err := ffmpegP.Start(ctx); err != nil {
		return nil, err
	}

	// goroutine will take care of closing
	deferCloseFn = nil

	var w io.WriteCloser
	dr.Media, w = io.Pipe()
	closeOnDone = append(closeOnDone, w)

	go func() {
		if outFormat.Prepend == "id3v2" {
			id3v2.Write(w, id3v2FramesFromYoutueDLInfo(ydl))
		}
		n, err := io.Copy(w, ffmpegR)

		log.Printf("Copy ffmpeg done (n=%v err=%v)", n, err)

		closeOnDoneFn()
		ffmpegP.Wait()

		log.Printf("Done")

		close(dr.waitCh)
	}()

	return dr, nil
}
