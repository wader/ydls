package ydls

// TODO: test close reader prematurely

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/wader/ydls/ffmpeg"
	"github.com/wader/ydls/leaktest"
	"github.com/wader/ydls/stringprioset"
	"github.com/wader/ydls/timerange"
	"github.com/wader/ydls/youtubedl"
)

const youtubeTestVideoURL = "https://www.youtube.com/watch?v=C0DPdy98e4c"
const soundcloudTestAudioURL = "https://soundcloud.com/timsweeney/thedrifter"

var testNetwork = os.Getenv("TEST_NETWORK") != ""
var testYoutubeldl = os.Getenv("TEST_YOUTUBEDL") != ""
var testFfmpeg = os.Getenv("TEST_FFMPEG") != ""

func stringsContains(strings []string, s string) bool {
	for _, ss := range strings {
		if ss == s {
			return true
		}
	}

	return false
}

func ydlsFromEnv(t *testing.T) YDLS {
	ydls, err := NewFromFile(os.Getenv("CONFIG"))
	if err != nil {
		t.Fatalf("failed to read config: %s", err)
	}

	return ydls
}

func TestSafeFilename(t *testing.T) {
	for _, c := range []struct {
		s      string
		expect string
	}{
		{"aba", "aba"},
		{"a/a", "a_a"},
		{"a\\a", "a_a"},
	} {
		actual := safeFilename(c.s)
		if actual != c.expect {
			t.Errorf("%s, got %v expected %v", c.s, actual, c.expect)
		}
	}
}

func TestForceCodec(t *testing.T) {
	if !testNetwork || !testFfmpeg || !testYoutubeldl {
		t.Skip("TEST_NETWORK, TEST_FFMPEG, TEST_YOUTUBEDL env not set")
	}

	ydls := ydlsFromEnv(t)

	defer leaktest.Check(t)()

	const formatName = "mkv"
	mkvFormat, _ := ydls.Config.Formats.FindByName(formatName)
	forceCodecs := []string{"opus", "vp9"}

	// make sure codecs are not the perfered ones
	for _, s := range mkvFormat.Streams {
		for _, forceCodec := range forceCodecs {
			if c, ok := s.CodecNames.First(); ok && c == forceCodec {
				t.Errorf("test sanity check failed: codec already the prefered one")
				return
			}
		}
	}

	ctx, cancelFn := context.WithCancel(context.Background())

	dr, err := ydls.Download(ctx,
		DownloadOptions{
			URL:    youtubeTestVideoURL,
			Format: formatName,
			Codecs: forceCodecs,
		},
		nil)
	if err != nil {
		cancelFn()
		t.Errorf("%s: download failed: %s", youtubeTestVideoURL, err)
		return
	}

	pi, err := ffmpeg.Probe(ctx, ffmpeg.Reader{Reader: io.LimitReader(dr.Media, 10*1024*1024)}, nil, nil)
	dr.Media.Close()
	dr.Wait()
	cancelFn()
	if err != nil {
		t.Errorf("%s: probe failed: %s", youtubeTestVideoURL, err)
		return
	}

	if pi.FormatName() != "matroska" {
		t.Errorf("%s: force codec failed: found %s", youtubeTestVideoURL, pi)
		return
	}

	for i := 0; i < len(forceCodecs); i++ {
		if pi.Streams[i].CodecName != forceCodecs[i] {
			t.Errorf("%s: force codec failed: %s != %s", youtubeTestVideoURL, pi.Streams[i].CodecName, forceCodecs[i])
			return
		}
	}

}

func TestTimeRangeOption(t *testing.T) {
	if !testNetwork || !testFfmpeg || !testYoutubeldl {
		t.Skip("TEST_NETWORK, TEST_FFMPEG, TEST_YOUTUBEDL env not set")
	}

	ydls := ydlsFromEnv(t)

	defer leaktest.Check(t)()

	timeRange, timeRangeErr := timerange.NewFromString("10s-15s")
	if timeRangeErr != nil {
		t.Fatalf("failed to parse time range")
	}

	ctx, cancelFn := context.WithCancel(context.Background())

	dr, err := ydls.Download(ctx,
		DownloadOptions{
			URL:       youtubeTestVideoURL,
			Format:    "mkv",
			TimeRange: timeRange,
		},
		nil)
	if err != nil {
		cancelFn()
		t.Fatalf("%s: download failed: %s", youtubeTestVideoURL, err)
	}

	pi, err := ffmpeg.Probe(ctx, ffmpeg.Reader{Reader: io.LimitReader(dr.Media, 10*1024*1024)}, nil, nil)
	dr.Media.Close()
	dr.Wait()
	cancelFn()
	if err != nil {
		t.Errorf("%s: probe failed: %s", youtubeTestVideoURL, err)
		return
	}

	if pi.Duration() != timeRange.Duration() {
		t.Errorf("%s: probed duration not %v, got %v", youtubeTestVideoURL, timeRange.Duration(), pi.Duration())
		return
	}
}

func TestMissingMediaStream(t *testing.T) {
	if !testNetwork || !testFfmpeg || !testYoutubeldl {
		t.Skip("TEST_NETWORK, TEST_FFMPEG, TEST_YOUTUBEDL env not set")
	}

	ydls := ydlsFromEnv(t)

	defer leaktest.Check(t)()

	ctx, cancelFn := context.WithCancel(context.Background())

	_, err := ydls.Download(ctx,
		DownloadOptions{
			URL:    soundcloudTestAudioURL,
			Format: "mkv",
		},
		nil)
	cancelFn()
	if err == nil {
		t.Fatal("expected download to fail")
	}
}

func TestFindYDLFormat(t *testing.T) {
	ydlFormats := []youtubedl.Format{
		{FormatID: "1", Protocol: "http", NormACodec: "mp3", NormVCodec: "h264", NormBR: 1},
		{FormatID: "2", Protocol: "http", NormACodec: "", NormVCodec: "h264", NormBR: 2},
		{FormatID: "3", Protocol: "http", NormACodec: "aac", NormVCodec: "", NormBR: 3},
		{FormatID: "4", Protocol: "http", NormACodec: "vorbis", NormVCodec: "vp8", NormBR: 4},
		{FormatID: "5", Protocol: "http", NormACodec: "opus", NormVCodec: "vp9", NormBR: 5},
	}

	for i, c := range []struct {
		ydlFormats       []youtubedl.Format
		mediaType        MediaType
		codecs           stringprioset.Set
		expectedFormatID string
	}{
		{ydlFormats, MediaAudio, stringprioset.New([]string{"mp3"}), "1"},
		{ydlFormats, MediaAudio, stringprioset.New([]string{"aac"}), "3"},
		{ydlFormats, MediaVideo, stringprioset.New([]string{"h264"}), "2"},
		{ydlFormats, MediaVideo, stringprioset.New([]string{"h264"}), "2"},
		{ydlFormats, MediaAudio, stringprioset.New([]string{"vorbis"}), "4"},
		{ydlFormats, MediaVideo, stringprioset.New([]string{"vp8"}), "4"},
		{ydlFormats, MediaAudio, stringprioset.New([]string{"opus"}), "5"},
		{ydlFormats, MediaVideo, stringprioset.New([]string{"vp9"}), "5"},
	} {
		actualFormat, actaulFormatFound := findYDLFormat(c.ydlFormats, c.mediaType, c.codecs)
		if actaulFormatFound && actualFormat.FormatID != c.expectedFormatID {
			t.Errorf("%d: expected format %s, got %s", i, c.expectedFormatID, actualFormat)
		}
	}
}
