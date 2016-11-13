package ydls

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sort"
	"sync"

	"github.com/wader/ydls/ffmpeg"
	"github.com/wader/ydls/youtubedl"
)

func writeID3v2FromYoutueDLInfo(w io.Writer, i *youtubedl.Info) {
	id3v2Frames := []id3v2Frame{
		&textFrame{"TPE1", firstNonEmpty(i.Artist, i.Creator, i.Uploader)},
		&textFrame{"TIT2", i.Title},
		&commFrame{"XXX", "", i.Description},
	}
	if i.Duration > 0 {
		id3v2Frames = append(id3v2Frames, &textFrame{"TLEN", fmt.Sprintf("%d", uint32(i.Duration*1000))})
	}
	if len(i.ThumbnailBytes) > 0 {
		id3v2Frames = append(id3v2Frames, &apicFrame{
			http.DetectContentType(i.ThumbnailBytes),
			id3v2PictureTypeOther,
			"",
			i.ThumbnailBytes,
		})
	}

	id3v2WriteHeader(w, id3v2Frames)
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

func downloadAndProbeFormat(ydl *youtubedl.Info, filter string, debugLog *log.Logger) (r io.ReadCloser, pi *ffmpeg.ProbeInfo, err error) {
	var ydlStderr io.Writer
	if debugLog != nil {
		ydlStderr = &loggerWriter{Logger: debugLog, Prefix: "ydl 2> "}
	}
	r, err = ydl.Download(filter, ydlStderr)
	if err != nil {
		return nil, nil, err
	}

	rr := &reReadCloser{ReadCloser: r}

	var ffprobeStderr io.Writer
	if debugLog != nil {
		ffprobeStderr = &loggerWriter{Logger: debugLog, Prefix: fmt.Sprintf("ffprobe %s 2> ", filter)}
	}
	const maxProbeByteSize = 10 * 1024 * 1024
	pi, err = ffmpeg.FFprobe(io.LimitReader(rr, maxProbeByteSize), debugLog, ffprobeStderr)
	if err != nil {
		return nil, nil, err
	}
	// restart and replay buffer data used when probing
	rr.Restarted = true

	return rr, pi, nil
}

// YDLs youtubedl downloader with some extras
type YDLs struct {
	Formats *Formats
}

// NewFromFile new YDLs using formats file
func NewFromFile(formatsPath string) (*YDLs, error) {
	formatsFile, err := os.Open(formatsPath)
	if err != nil {
		return nil, err
	}
	defer formatsFile.Close()
	formats, err := parseFormats(formatsFile)
	if err != nil {
		return nil, err
	}

	return &YDLs{Formats: formats}, nil
}

// Download downloads media from URL and makes sure output is in specified format
func (ydls *YDLs) Download(url string, formatName string, debugLog *log.Logger) (r io.ReadCloser, filename string, mimeType string, err error) {
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

	log := log.New(ioutil.Discard, "", 0)
	if debugLog != nil {
		log = debugLog
	}

	log.Printf("URL: %s", url)

	var ydlStdout io.Writer
	if debugLog != nil {
		ydlStdout = &loggerWriter{Logger: debugLog, Prefix: "ydl 1> "}
	}
	ydl, err := youtubedl.NewFromURL(url, ydlStdout)
	if err != nil {
		log.Printf("Failed to download: %s", err)
		return nil, "", "", err
	}

	log.Printf("Title: %s", ydl.Title)
	log.Printf("Available youtubedl formats:")
	for _, f := range ydl.Formats {
		log.Printf("  %s", f)
	}

	var outFormat *Format

	if formatName == "" {
		var probedInfo *ffmpeg.ProbeInfo
		r, probedInfo, err = downloadAndProbeFormat(ydl, "best[protocol=https]/best[protocol=http]/best", debugLog)
		if err != nil {
			return nil, "", "", err
		}

		log.Printf("Probed format %s", probedInfo)

		// see if we know about the probed format, otherwise fallback to "raw"
		outFormat = ydls.Formats.Find(probedInfo.FormatName(), probedInfo.ACodec(), probedInfo.VCodec())
	} else {
		outFormat = ydls.Formats.FindByName(formatName)
		if outFormat == nil {
			return nil, "", "", fmt.Errorf("could not find format")
		}

		aYDLFormat, vYDLFormat := findBestFormats(ydl.Formats, outFormat)

		log.Printf("Best youtubedl match for %s a=%s v=%s", formatName, aYDLFormat, vYDLFormat)

		var aProbedFormat *ffmpeg.ProbeInfo
		var aReader io.ReadCloser
		var aErr error
		var vProbedFormat *ffmpeg.ProbeInfo
		var vReader io.ReadCloser
		var vErr error

		// FIXME: messy

		if aYDLFormat != nil && vYDLFormat != nil {
			if aYDLFormat != vYDLFormat {
				var probeWG sync.WaitGroup
				probeWG.Add(2)
				go func() {
					defer probeWG.Done()
					aReader, aProbedFormat, aErr = downloadAndProbeFormat(ydl, aYDLFormat.FormatID, debugLog)
				}()
				go func() {
					defer probeWG.Done()
					vReader, vProbedFormat, vErr = downloadAndProbeFormat(ydl, vYDLFormat.FormatID, debugLog)
				}()
				probeWG.Wait()
				if aReader != nil {
					closeOnDone = append(closeOnDone, aReader)
				}
				if vReader != nil {
					closeOnDone = append(closeOnDone, vReader)
				}
			} else {
				aReader, aProbedFormat, aErr = downloadAndProbeFormat(ydl, aYDLFormat.FormatID, debugLog)
				vReader, vProbedFormat, vErr = aReader, aProbedFormat, aErr
				if aReader != nil {
					closeOnDone = append(closeOnDone, aReader)
				}
			}
		} else if aYDLFormat != nil && vYDLFormat == nil {
			aReader, aProbedFormat, aErr = downloadAndProbeFormat(ydl, aYDLFormat.FormatID, debugLog)
			if aReader != nil {
				closeOnDone = append(closeOnDone, aReader)
			}
		} else {
			aReader, aProbedFormat, aErr = downloadAndProbeFormat(ydl, "best", debugLog)
			vReader, vProbedFormat, vErr = aReader, aProbedFormat, aErr
			if aReader != nil {
				closeOnDone = append(closeOnDone, aReader)
			}
		}
		if aErr != nil || vErr != nil {
			return nil, "", "", fmt.Errorf("failed to probe")
		}

		log.Printf("Stream mapping:")
		var maps []ffmpeg.Map
		ffmpegFormatFlags := outFormat.FormatFlags

		if len(outFormat.ACodecs) > 0 && aProbedFormat != nil && aProbedFormat.ACodec() != "" {
			var outputCodecFormat *FormatCodec
			var ffmpegCodec string
			outputCodecFormat = outFormat.ACodecs.findByCodecName(aProbedFormat.ACodec())
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

			maps = append(maps, ffmpeg.Map{
				Input:           aReader,
				Kind:            "audio",
				StreamSpecifier: "a:0",
				Codec:           ffmpegCodec,
				Flags:           outputCodecFormat.CodecFlags,
			})
			ffmpegFormatFlags = append(ffmpegFormatFlags, outputCodecFormat.FormatFlags...)

			log.Printf("  audio probed:%s ydl:%s -> %s", aProbedFormat.ACodec(), ydlACodec, ffmpegCodec)
		}
		if len(outFormat.VCodecs) > 0 && vProbedFormat != nil && vProbedFormat.VCodec() != "" {
			var outputCodecFormat *FormatCodec
			var ffmpegCodec string
			outputCodecFormat = outFormat.VCodecs.findByCodecName(vProbedFormat.VCodec())
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

			maps = append(maps, ffmpeg.Map{
				Input:           vReader,
				Kind:            "video",
				StreamSpecifier: "v:0",
				Codec:           ffmpegCodec,
				Flags:           outputCodecFormat.CodecFlags,
			})
			ffmpegFormatFlags = append(ffmpegFormatFlags, outputCodecFormat.FormatFlags...)

			log.Printf("  video probed:%s ydl:%s -> %s", vProbedFormat.VCodec(), ydlVCodec, ffmpegCodec)
		}

		var ffmpegStderr io.Writer
		if debugLog != nil {
			ffmpegStderr = &loggerWriter{Logger: debugLog, Prefix: "ffmpeg 2> "}
		}
		ffmpegR, ffmpegW := io.Pipe()
		closeOnDone = append(closeOnDone, ffmpegR)

		ffmpegP := &ffmpeg.FFmpeg{
			Maps:     maps,
			Format:   ffmpeg.Format{Name: outFormat.Formats.first(), Flags: ffmpegFormatFlags},
			DebugLog: debugLog,
			Stdout:   ffmpegW,
			Stderr:   ffmpegStderr,
		}

		if err := ffmpegP.StartWaitForData(); err != nil {
			return nil, "", "", err
		}

		// goroutine will take care of closing
		deferCloseFn = nil

		var w io.WriteCloser
		r, w = io.Pipe()
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
		mimeType = "application/octet-stream"
		filename = ydl.Title + ".raw"
	} else {
		mimeType = outFormat.MIMEType
		filename = ydl.Title + "." + outFormat.Ext
	}

	return r, filename, mimeType, nil
}
