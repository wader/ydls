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

	"github.com/wader/ydls/ffmpeg"
	"github.com/wader/ydls/leaktest"
	"github.com/wader/ydls/timerange"
)

type bufferCloser struct {
	bytes.Buffer
}

func (bc *bufferCloser) Close() error {
	return nil
}

func TestFFmpegHasFormatsCodecs(t *testing.T) {
	if !testFfmpeg {
		t.Skip("TEST_FFMPEG env not set")
	}

	aCodecs := map[string]bool{}
	vCodecs := map[string]bool{}

	ydls := ydlsFromEnv(t)

	// collect unique codecs
	for _, f := range ydls.Config.Formats {
		for _, c := range f.ACodecs {
			aCodecs[c.Codec] = true
		}
		for _, c := range f.VCodecs {
			vCodecs[c.Codec] = true
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

	for codecType, codecs := range map[string]map[string]bool{"a": aCodecs, "v": vCodecs} {
		for codecName := range codecs {
			t.Logf("Testing: %s", codecName)

			dummyReader := bytes.NewReader(dummyBuf)

			output := &bufferCloser{}

			ffmpegP := &ffmpeg.FFmpeg{
				StreamMaps: []ffmpeg.StreamMap{
					ffmpeg.StreamMap{
						Reader:    dummyReader,
						Specifier: codecType + ":0",
						Codec:     codecType + "codec:" + firstNonEmpty(ydls.Config.CodecMap[codecName], codecName),
					},
				},
				Format:   ffmpeg.Format{Name: "matroska"},
				DebugLog: nil, //log.New(os.Stdout, "debug> ", 0),
				Stderr:   nil, //writelogger.New(log.New(os.Stdout, "stderr> ", 0), ""),
				Stdout:   output,
			}

			if err := ffmpegP.Start(context.Background()); err != nil {
				t.Errorf("ffmpeg start failed for %s: %v", codecName, err)
			} else if err := ffmpegP.Wait(); err != nil {
				t.Errorf("ffmpeg wait failed for %s: %v", codecName, err)
			}
		}
	}
}

func TestFormats(t *testing.T) {
	if !testNetwork || !testFfmpeg || !testYoutubeldl {
		t.Skip("TEST_NETWORK, TEST_FFMPEG, TEST_YOUTUBEDL env not set")
	}

	ydls := ydlsFromEnv(t)

	for _, c := range []struct {
		url              string
		audioOnly        bool
		expectedFilename string
	}{
		{"https://soundcloud.com/timsweeney/thedrifter", true, "BIS Radio Show #793 with The Drifter"},
		{youtubeTestVideoURL, false, "TEST VIDEO"},
	} {
		for _, f := range ydls.Config.Formats {
			func() {
				defer leaktest.Check(t)()

				if c.audioOnly && len(f.VCodecs) > 0 {
					t.Logf("%s: %s: skip, audio only\n", c.url, f.Name)
					return
				}

				ctx, cancelFn := context.WithCancel(context.Background())

				dr, err := ydls.Download(
					ctx,
					DownloadOptions{
						URL:       c.url,
						Format:    f.Name,
						TimeRange: timerange.TimeRange{Stop: 1 * time.Second},
					},
					nil,
				)
				if err != nil {
					cancelFn()
					t.Errorf("%s: %s: download failed: %s", c.url, f.Name, err)
					return
				}

				pi, err := ffmpeg.Probe(ctx, io.LimitReader(dr.Media, 10*1024*1024), nil, nil)
				dr.Media.Close()
				dr.Wait()
				cancelFn()
				if err != nil {
					t.Errorf("%s: %s: probe failed: %s", c.url, f.Name, err)
					return
				}

				if !strings.HasPrefix(dr.Filename, c.expectedFilename) {
					t.Errorf("%s: %s: expected filename '%s' found '%s'", c.url, f.Name, c.expectedFilename, dr.Filename)
					return
				}
				if f.MIMEType != dr.MIMEType {
					t.Errorf("%s: %s: expected MIME type '%s' found '%s'", c.url, f.Name, f.MIMEType, dr.MIMEType)
					return
				}
				if !stringsContains([]string(f.Formats), pi.FormatName()) {
					t.Errorf("%s: %s: expected format %s found %s", c.url, f.Name, f.Formats, pi.FormatName())
					return
				}
				if len(f.ACodecs.CodecNames()) != 0 && !stringsContains(f.ACodecs.CodecNames(), pi.ACodec()) {
					t.Errorf("%s: %s: expected acodec %s found %s", c.url, f.Name, f.ACodecs.CodecNames(), pi.ACodec())
					return
				}
				if len(f.VCodecs.CodecNames()) != 0 && !stringsContains(f.VCodecs.CodecNames(), pi.VCodec()) {
					t.Errorf("%s: %s: expected vcodec %s found %s", c.url, f.Name, f.VCodecs.CodecNames(), pi.VCodec())
					return
				}
				if f.Prepend == "id3v2" {
					if _, ok := pi.Format["tags"]; !ok {
						t.Errorf("%s: %s: expected id3v2 tag", c.url, f.Name)
					}
				}

				t.Logf("%s: %s: OK (probed %s)\n", c.url, f.Name, pi)
			}()
		}
	}
}

func TestRawFormat(t *testing.T) {
	if !testNetwork || !testFfmpeg || !testYoutubeldl {
		t.Skip("TEST_NETWORK, TEST_FFMPEG, TEST_YOUTUBEDL env not set")
	}

	ydls := ydlsFromEnv(t)

	defer leaktest.Check(t)()

	ctx, cancelFn := context.WithCancel(context.Background())

	dr, err := ydls.Download(ctx, DownloadOptions{URL: youtubeTestVideoURL}, nil)
	if err != nil {
		cancelFn()
		t.Errorf("%s: %s: download failed: %s", youtubeTestVideoURL, "raw", err)
		return
	}

	pi, err := ffmpeg.Probe(ctx, io.LimitReader(dr.Media, 10*1024*1024), nil, nil)
	dr.Media.Close()
	dr.Wait()
	cancelFn()
	if err != nil {
		t.Errorf("%s: %s: probe failed: %s", youtubeTestVideoURL, "raw", err)
		return
	}

	t.Logf("%s: %s: OK (probed %s)\n", youtubeTestVideoURL, "raw", pi)
}
