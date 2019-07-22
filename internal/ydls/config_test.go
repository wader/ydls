package ydls

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/wader/ydls/internal/ffmpeg"
	"github.com/wader/ydls/internal/timerange"
)

type bufferCloser struct {
	bytes.Buffer
}

func (bc *bufferCloser) Close() error {
	return nil
}

func TestFFmpegHasFormatsCodecs(t *testing.T) {
	if !testExternal {
		t.Skip("TEST_EXTERNAL")
	}

	type codec struct {
		codec     string
		specifier string
	}
	codecs := map[ffmpeg.Codec]codec{}

	ydls := ydlsFromEnv(t)

	// collect unique codecs
	for _, f := range ydls.Config.Formats {
		for _, s := range f.Streams {
			for _, c := range s.Codecs {
				codecName := firstNonEmpty(ydls.Config.CodecMap[c.Name], c.Name)
				if s.Media == MediaAudio {
					codecs[ffmpeg.AudioCodec(codecName)] = codec{codec: codecName, specifier: "a"}
				} else if s.Media == MediaVideo {
					codecs[ffmpeg.VideoCodec(codecName)] = codec{codec: codecName, specifier: "v"}
				}
			}
		}
	}

	dummy, dummyErr := ffmpeg.Dummy("matroska", "mp3", "h264")
	if dummyErr != nil {
		log.Fatal(dummyErr)
	}
	dummyBuf, dummyBufErr := ioutil.ReadAll(dummy)
	if dummyBufErr != nil {
		log.Fatal(dummyBufErr)
	}

	for ffcodec, codec := range codecs {
		t.Run(codec.codec, func(t *testing.T) {
			dummyReader := bytes.NewReader(dummyBuf)

			output := &bufferCloser{}

			ffmpegP := &ffmpeg.FFmpeg{
				Streams: []ffmpeg.Stream{
					ffmpeg.Stream{
						Maps: []ffmpeg.Map{
							ffmpeg.Map{
								Input:     ffmpeg.Reader{Reader: dummyReader},
								Specifier: codec.specifier,
								Codec:     ffcodec,
							},
						},
						Format: ffmpeg.Format{Name: "matroska"},
						Output: ffmpeg.Writer{Writer: output},
					},
				},
				DebugLog: nil, //log.New(os.Stdout, "debug> ", 0),
				Stderr:   nil, //printwriter.New(log.New(os.Stdout, "stderr> ", 0), ""),
			}

			if err := ffmpegP.Start(context.Background()); err != nil {
				t.Errorf("ffmpeg start failed for %s: %v", codec, err)
			} else if err := ffmpegP.Wait(); err != nil {
				t.Errorf("ffmpeg wait failed for %s: %v", codec, err)
			}
		})
	}

}

func TestFormats(t *testing.T) {
	if !testExternal {
		t.Skip("TEST_EXTERNAL")
	}

	ydls := ydlsFromEnv(t)

	for _, c := range []struct {
		MediaRawURL      string
		audioOnly        bool
		expectedFilename string
	}{
		{soundcloudTestAudioURL, true, "Avalon Emerson Live at Printworks London"},
		{youtubeTestVideoURL, false, "TEST VIDEO"},
	} {
		for formatName, format := range ydls.Config.Formats {
			if firstFormat, _ := format.Formats.First(); firstFormat == "rss" {
				continue
			}

			t.Run(formatName+"-"+c.MediaRawURL, func(t *testing.T) {
				defer leakChecks(t)()

				hasVideo := false
				for _, s := range format.Streams {
					if s.Media == MediaVideo {
						hasVideo = true
						break
					}
				}

				if c.audioOnly && hasVideo {
					t.Logf("skip, test stream is audio only\n")
					return
				}

				ctx, cancelFn := context.WithCancel(context.Background())

				dr, err := ydls.Download(
					ctx,
					DownloadOptions{
						RequestOptions: RequestOptions{
							MediaRawURL: c.MediaRawURL,
							Format:      &format,
							TimeRange:   timerange.TimeRange{Stop: timerange.Duration(1 * time.Second)},
						},
					},
				)
				if err != nil {
					cancelFn()
					t.Errorf("download failed: %s", err)
					return
				}

				const limitBytes = 10 * 1024 * 1024
				pi, err := ffmpeg.Probe(ctx, ffmpeg.Reader{Reader: io.LimitReader(dr.Media, limitBytes)}, nil, nil)
				dr.Media.Close()
				dr.Wait()
				cancelFn()
				if err != nil {
					t.Errorf("probe failed: %s", err)
					return
				}

				if !strings.HasPrefix(dr.Filename, c.expectedFilename) {
					t.Errorf("expected filename '%s' found '%s'", c.expectedFilename, dr.Filename)
					return
				}
				if format.MIMEType != dr.MIMEType {
					t.Errorf("expected MIME type '%s' found '%s'", format.MIMEType, dr.MIMEType)
					return
				}
				if !format.Formats.Member(pi.FormatName()) {
					t.Errorf("expected format %s found %s", format.Formats, pi.FormatName())
					return
				}

				for i := 0; i < len(format.Streams); i++ {
					formatStream := format.Streams[i]
					probeStream := pi.Streams[i]

					if !formatStream.CodecNames.Member(probeStream.CodecName) {
						t.Errorf("expected codec %s found %s", formatStream.CodecNames, probeStream.CodecName)
						return
					}
				}

				if format.Prepend == "id3v2" {
					if pi.Format.Tags.Title == "" {
						t.Errorf("expected id3v2 title tag")
					}
				}

				t.Logf("OK (probed %s)", pi)
			})
		}
	}
}

func TestRawFormat(t *testing.T) {
	if !testExternal {
		t.Skip("TEST_EXTERNAL")
	}

	ydls := ydlsFromEnv(t)

	defer leakChecks(t)()

	ctx, cancelFn := context.WithCancel(context.Background())

	dr, err := ydls.Download(ctx,
		DownloadOptions{
			RequestOptions: RequestOptions{
				MediaRawURL: youtubeTestVideoURL,
			},
		},
	)
	if err != nil {
		cancelFn()
		t.Errorf("%s: %s: download failed: %s", youtubeTestVideoURL, "raw", err)
		return
	}

	pi, err := ffmpeg.Probe(ctx, ffmpeg.Reader{Reader: io.LimitReader(dr.Media, 10*1024*1024)}, nil, nil)
	dr.Media.Close()
	dr.Wait()
	cancelFn()
	if err != nil {
		t.Errorf("%s: %s: probe failed: %s", youtubeTestVideoURL, "raw", err)
		return
	}

	t.Logf("%s: %s: OK (probed %s)\n", youtubeTestVideoURL, "raw", pi)
}

func TestFindByFormatCodecs(t *testing.T) {
	ydls := ydlsFromEnv(t)

	for i, c := range []struct {
		format   string
		codecs   []string
		expected string
	}{
		{"mp3", []string{"mp3"}, "mp3"},
		{"flac", []string{"flac"}, "flac"},
		{"mov", []string{"alac"}, "alac"},
		{"mov", []string{"aac", "h264"}, "mp4"},
		{"matroska", []string{"vorbis", "vp8"}, "mkv"},
		{"matroska", []string{"opus", "vp9"}, "mkv"},
		{"matroska", []string{"aac", "h264"}, "mkv"},
		{"matroska", []string{"vp8", "vorbis"}, "mkv"},
		{"matroska", []string{"vorbis", "vp8"}, "mkv"},
		{"mpegts", []string{"aac", "h264"}, "ts"},
		{"", []string{}, ""},
	} {
		_, actualFormatName := ydls.Config.Formats.FindByFormatCodecs(c.format, c.codecs)
		if c.expected != actualFormatName {
			t.Errorf("%d: expected format %s, got %s", i, c.expected, actualFormatName)
		}
	}

}
