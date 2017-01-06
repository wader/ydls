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
	"sync"

	"github.com/wader/ydls/ffmpeg"
	"github.com/wader/ydls/id3v2"
	"github.com/wader/ydls/rereadcloser"
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

func writeID3v2FromYoutueDLInfo(w io.Writer, i *youtubedl.Info) {
	frames := []id3v2.Frame{
		&id3v2.TextFrame{ID: "TPE1", Text: firstNonEmpty(i.Artist, i.Creator, i.Uploader)},
		&id3v2.TextFrame{ID: "TIT2", Text: i.Title},
		&id3v2.COMMFrame{Language: "XXX", Description: "", Text: i.Description},
	}
	if i.Duration > 0 {
		frames = append(frames, &id3v2.TextFrame{ID: "TLEN", Text: fmt.Sprintf("%d", uint32(i.Duration*1000))})
	}
	if len(i.ThumbnailBytes) > 0 {
		frames = append(frames, &id3v2.APICFrame{
			MIMEType:    http.DetectContentType(i.ThumbnailBytes),
			PictureType: id3v2.PictureTypeOther,
			Description: "",
			Data:        i.ThumbnailBytes,
		})
	}

	id3v2.Write(w, frames)
}

func findFormat(formats []*youtubedl.Format, protocol string, aCodecs *prioStringSet, vCodecs *prioStringSet) *youtubedl.Format {
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

func findBestFormats(ydlFormats []*youtubedl.Format, format *Format) (aFormat *youtubedl.Format, vFormat *youtubedl.Format) {
	type neededFormat struct {
		aCodecs    *prioStringSet
		vCodecs    *prioStringSet
		aYDLFormat **youtubedl.Format
		vYDLFormat **youtubedl.Format
	}

	var neededFormats []*neededFormat

	// match exactly, both audio/video codecs found or not found
	neededFormats = append(neededFormats, &neededFormat{
		format.ACodecs.PrioStringSet(),
		format.VCodecs.PrioStringSet(),
		&aFormat, &vFormat,
	})

	if !format.ACodecs.empty() {
		// matching audio codec with any video codec
		neededFormats = append(neededFormats, &neededFormat{format.ACodecs.PrioStringSet(), nil, &aFormat, nil})
		// match any audio codec and no video
		neededFormats = append(neededFormats, &neededFormat{nil, &prioStringSet{}, &aFormat, nil})
		// match any audio and video codec
		neededFormats = append(neededFormats, &neededFormat{nil, nil, &aFormat, nil})
	}
	if !format.VCodecs.empty() {
		// same logic as above
		neededFormats = append(neededFormats, &neededFormat{nil, format.VCodecs.PrioStringSet(), nil, &vFormat})
		neededFormats = append(neededFormats, &neededFormat{&prioStringSet{}, nil, nil, &vFormat})
		neededFormats = append(neededFormats, &neededFormat{nil, nil, nil, &vFormat})
	}

	// TODO: if only audio => stream with lowest video br?

	for _, proto := range []string{"https", "http", "*"} {
		for _, f := range neededFormats {
			m := findFormat(ydlFormats, proto, f.aCodecs, f.vCodecs)

			if m == nil {
				continue
			}

			if f.aYDLFormat != nil && *f.aYDLFormat == nil && m.NormACodec != "" {
				*f.aYDLFormat = m
			}
			if f.vYDLFormat != nil && *f.vYDLFormat == nil && m.NormVCodec != "" {
				*f.vYDLFormat = m
			}

			if (format.ACodecs.empty() || aFormat != nil) &&
				(format.VCodecs.empty() || vFormat != nil) {
				break
			}
		}
	}

	return aFormat, vFormat
}

func downloadAndProbeFormat(ctx context.Context, ydl *youtubedl.Info, filter string, debugLog *log.Logger) (r io.ReadCloser, pi *ffmpeg.ProbeInfo, err error) {
	log := logOrDiscard(debugLog)

	ydlStderr := writelogger.New(log, fmt.Sprintf("ydl-dl %s stderr> ", filter))
	r, err = ydl.Download(ctx, filter, ydlStderr)
	if err != nil {
		return nil, nil, err
	}

	rr := rereadcloser.New(r)
	ffprobeStderr := writelogger.New(log, fmt.Sprintf("ffprobe %s stderr> ", filter))
	const maxProbeByteSize = 10 * 1024 * 1024
	pi, err = ffmpeg.Probe(ctx, io.LimitReader(rr, maxProbeByteSize), log, ffprobeStderr)
	if err != nil {
		return nil, nil, err
	}
	// restart and replay buffer data used when probing
	rr.Restarted = true

	return rr, pi, nil
}

// YDLS youtubedl downloader with some extras
type YDLS struct {
	Formats *Formats
}

// NewFromFile new YDLs using formats file
func NewFromFile(formatsPath string) (*YDLS, error) {
	formatsFile, err := os.Open(formatsPath)
	if err != nil {
		return nil, err
	}
	defer formatsFile.Close()
	formats, err := parseFormats(formatsFile)
	if err != nil {
		return nil, err
	}

	return &YDLS{Formats: formats}, nil
}

// DownloadResult download result
type DownloadResult struct {
	Media    io.ReadCloser
	Filename string
	MIMEType string
}

// Download downloads media from URL using context and makes sure output is in specified format
func (ydls *YDLS) Download(ctx context.Context, url string, formatName string, debugLog *log.Logger) (*DownloadResult, error) {
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

	log := logOrDiscard(debugLog)

	log.Printf("URL: %s", url)

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

	dr := &DownloadResult{}

	var outFormat *Format

	if formatName == "" {
		var probeInfo *ffmpeg.ProbeInfo
		dr.Media, probeInfo, err = downloadAndProbeFormat(ctx, ydl, "best[protocol=https]/best[protocol=http]/best", log)
		if err != nil {
			return nil, err
		}

		log.Printf("Probed format %s", probeInfo)

		// see if we know about the probed format, otherwise fallback to "raw"
		outFormat = ydls.Formats.Find(probeInfo.FormatName(), probeInfo.ACodec(), probeInfo.VCodec())
	} else {
		outFormat = ydls.Formats.FindByName(formatName)
		if outFormat == nil {
			return nil, fmt.Errorf("could not find format")
		}

		aYDLFormat, vYDLFormat := findBestFormats(ydl.Formats, outFormat)

		log.Printf("Best youtubedl match for %s a=%s v=%s", formatName, aYDLFormat, vYDLFormat)

		var aProbeInfo *ffmpeg.ProbeInfo
		var aReader io.ReadCloser
		var aErr error
		var vProbeInfo *ffmpeg.ProbeInfo
		var vReader io.ReadCloser
		var vErr error

		// FIXME: messy

		if aYDLFormat != nil && vYDLFormat != nil {
			if aYDLFormat != vYDLFormat {
				var probeWG sync.WaitGroup
				probeWG.Add(2)
				go func() {
					defer probeWG.Done()
					aReader, aProbeInfo, aErr = downloadAndProbeFormat(ctx, ydl, aYDLFormat.FormatID, log)
				}()
				go func() {
					defer probeWG.Done()
					vReader, vProbeInfo, vErr = downloadAndProbeFormat(ctx, ydl, vYDLFormat.FormatID, log)
				}()
				probeWG.Wait()
				if aReader != nil {
					closeOnDone = append(closeOnDone, aReader)
				}
				if vReader != nil {
					closeOnDone = append(closeOnDone, vReader)
				}
			} else {
				aReader, aProbeInfo, aErr = downloadAndProbeFormat(ctx, ydl, aYDLFormat.FormatID, log)
				vReader, vProbeInfo, vErr = aReader, aProbeInfo, aErr
				if aReader != nil {
					closeOnDone = append(closeOnDone, aReader)
				}
			}
		} else if aYDLFormat != nil && vYDLFormat == nil {
			aReader, aProbeInfo, aErr = downloadAndProbeFormat(ctx, ydl, aYDLFormat.FormatID, log)
			if aReader != nil {
				closeOnDone = append(closeOnDone, aReader)
			}
		} else {
			aReader, aProbeInfo, aErr = downloadAndProbeFormat(ctx, ydl, "best", log)
			vReader, vProbeInfo, vErr = aReader, aProbeInfo, aErr
			if aReader != nil {
				closeOnDone = append(closeOnDone, aReader)
			}
		}
		if aErr != nil || vErr != nil {
			return nil, fmt.Errorf("failed to probe")
		}

		log.Printf("Stream mapping:")
		var streamMaps []ffmpeg.StreamMap
		ffmpegFormatFlags := outFormat.FormatFlags

		if len(outFormat.ACodecs) > 0 && aProbeInfo != nil && aProbeInfo.ACodec() != "" {
			var outputCodecFormat *FormatCodec
			var ffmpegCodec string
			outputCodecFormat = outFormat.ACodecs.findByCodecName(aProbeInfo.ACodec())
			if outputCodecFormat == nil {
				outputCodecFormat = outFormat.ACodecs.first()
				ffmpegCodec = outputCodecFormat.Codec
			} else {
				ffmpegCodec = "copy"
			}

			ydlACodec := "n/a"
			if aYDLFormat != nil {
				ydlACodec = aYDLFormat.NormACodec
			}

			streamMaps = append(streamMaps, ffmpeg.StreamMap{
				Reader:     aReader,
				Specifier:  "a:0",
				Codec:      "acodec:" + ffmpegCodec,
				CodecFlags: outputCodecFormat.CodecFlags,
			})
			ffmpegFormatFlags = append(ffmpegFormatFlags, outputCodecFormat.FormatFlags...)

			log.Printf("  audio probed:%s ydl:%s -> %s", aProbeInfo.ACodec(), ydlACodec, ffmpegCodec)
		}
		if len(outFormat.VCodecs) > 0 && vProbeInfo != nil && vProbeInfo.VCodec() != "" {
			var outputCodecFormat *FormatCodec
			var ffmpegCodec string
			outputCodecFormat = outFormat.VCodecs.findByCodecName(vProbeInfo.VCodec())
			if outputCodecFormat == nil {
				outputCodecFormat = outFormat.VCodecs.first()
				ffmpegCodec = outputCodecFormat.Codec
			} else {
				ffmpegCodec = "copy"
			}

			ydlVCodec := "n/a"
			if vYDLFormat != nil {
				ydlVCodec = vYDLFormat.NormVCodec
			}

			streamMaps = append(streamMaps, ffmpeg.StreamMap{
				Reader:     vReader,
				Specifier:  "v:0",
				Codec:      "vcodec:" + ffmpegCodec,
				CodecFlags: outputCodecFormat.CodecFlags,
			})
			ffmpegFormatFlags = append(ffmpegFormatFlags, outputCodecFormat.FormatFlags...)

			log.Printf("  video probed:%s ydl:%s -> %s", vProbeInfo.VCodec(), ydlVCodec, ffmpegCodec)
		}

		var ffmpegStderr io.Writer
		ffmpegStderr = writelogger.New(log, "ffmpeg stderr> ")
		ffmpegR, ffmpegW := io.Pipe()
		closeOnDone = append(closeOnDone, ffmpegR)

		ffmpegP := &ffmpeg.FFmpeg{
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
				writeID3v2FromYoutueDLInfo(w, ydl)
			}
			io.Copy(w, ffmpegR)
			closeOnDoneFn()
			ffmpegP.Wait()
		}()
	}

	if outFormat == nil {
		dr.MIMEType = "application/octet-stream"
		dr.Filename = ydl.Title + ".raw"
	} else {
		dr.MIMEType = outFormat.MIMEType
		dr.Filename = ydl.Title + "." + outFormat.Ext
	}

	return dr, nil
}
