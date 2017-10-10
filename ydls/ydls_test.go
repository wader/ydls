package ydls

// TODO: test close reader prematurely

import (
	"context"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/wader/ydls/ffmpeg"
	"github.com/wader/ydls/leaktest"
	"github.com/wader/ydls/timerange"
	"github.com/wader/ydls/youtubedl"
)

const youtubeTestVideoURL = "https://www.youtube.com/watch?v=C0DPdy98e4c"

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

	mkvFormat := ydls.Config.Formats.FindByName("mkv")
	forceACodec := "opus"
	forceVCodec := "vp9"

	// make sure codecs are not the perfered ones
	if f, _ := mkvFormat.ACodecs.first(); f.Codec == forceACodec {
		t.Errorf("test sanity check failed: audio codec already the prefered one")
		return
	}
	if f, _ := mkvFormat.VCodecs.first(); f.Codec == forceVCodec {
		t.Errorf("test sanity check failed: video codec already the prefered one")
		return
	}

	ctx, cancelFn := context.WithCancel(context.Background())

	dr, err := ydls.Download(ctx,
		DownloadOptions{
			URL:    youtubeTestVideoURL,
			Format: mkvFormat.Name,
			ACodec: forceACodec,
			VCodec: forceVCodec,
		},
		nil)
	if err != nil {
		cancelFn()
		t.Errorf("%s: download failed: %s", youtubeTestVideoURL, err)
		return
	}

	pi, err := ffmpeg.Probe(ctx, io.LimitReader(dr.Media, 10*1024*1024), nil, nil)
	dr.Media.Close()
	dr.Wait()
	cancelFn()
	if err != nil {
		t.Errorf("%s: probe failed: %s", youtubeTestVideoURL, err)
		return
	}

	if pi.FormatName() != "matroska" || pi.ACodec() != forceACodec || pi.VCodec() != forceVCodec {
		t.Errorf("%s: force codec failed: found %s", youtubeTestVideoURL, pi)
		return
	}
}

func TestTimeRangeOption(t *testing.T) {
	if !testNetwork || !testFfmpeg || !testYoutubeldl {
		t.Skip("TEST_NETWORK, TEST_FFMPEG, TEST_YOUTUBEDL env not set")
	}

	ydls := ydlsFromEnv(t)

	defer leaktest.Check(t)()

	mkvFormat := ydls.Config.Formats.FindByName("mkv")
	timeRange, timeRangeErr := timerange.NewFromString("10s-15s")
	if timeRangeErr != nil {
		t.Fatalf("failed to parse time range")
	}

	ctx, cancelFn := context.WithCancel(context.Background())

	dr, err := ydls.Download(ctx,
		DownloadOptions{
			URL:       youtubeTestVideoURL,
			Format:    mkvFormat.Name,
			TimeRange: timeRange,
		},
		nil)
	if err != nil {
		cancelFn()
		t.Fatalf("%s: download failed: %s", youtubeTestVideoURL, err)
	}

	pi, err := ffmpeg.Probe(ctx, io.LimitReader(dr.Media, 10*1024*1024), nil, nil)
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

func testBestFormatCase(formats []*youtubedl.Format, aCodecs prioStringSet, vCodecs prioStringSet, aFormatID string, vFormatID string) error {
	aFormat, vFormat := findBestFormats(formats, aCodecs, vCodecs)

	if (aFormat == nil && aFormatID != "") ||
		(aFormat != nil && aFormat.FormatID != aFormatID) ||
		(vFormat == nil && vFormatID != "") ||
		(vFormat != nil && vFormat.FormatID != vFormatID) {
		gotAFormatID := ""
		if aFormat != nil {
			gotAFormatID = aFormat.FormatID
		}
		gotVFormatID := ""
		if vFormat != nil {
			gotVFormatID = vFormat.FormatID
		}
		return fmt.Errorf(
			"%v %v, expected aFormatID=%v vFormatID=%v, gotAFormatID=%v gotVFormatID=%v",
			aCodecs, vCodecs,
			aFormatID, vFormatID, gotAFormatID, gotVFormatID,
		)
	}

	return nil
}

func TestFindBestFormats1(t *testing.T) {
	ydlFormats := []*youtubedl.Format{
		{FormatID: "1", Protocol: "http", NormACodec: "mp3", NormVCodec: "h264", NormBR: 1},
		{FormatID: "2", Protocol: "http", NormACodec: "", NormVCodec: "h264", NormBR: 2},
		{FormatID: "3", Protocol: "http", NormACodec: "aac", NormVCodec: "", NormBR: 3},
		{FormatID: "4", Protocol: "http", NormACodec: "vorbis", NormVCodec: "", NormBR: 4},
	}

	for _, c := range []struct {
		ydlFormats []*youtubedl.Format
		aCodecs    prioStringSet
		vCodecs    prioStringSet
		aFormatID  string
		vFormatID  string
	}{
		{ydlFormats, prioStringSet([]string{"mp3"}), prioStringSet([]string{"h264"}), "1", "1"},
		{ydlFormats, prioStringSet([]string{"mp3"}), prioStringSet{}, "1", ""},
		{ydlFormats, prioStringSet([]string{"aac"}), prioStringSet{}, "3", ""},
		{ydlFormats, prioStringSet([]string{"aac"}), prioStringSet([]string{"h264"}), "3", "2"},
		{ydlFormats, prioStringSet([]string{"opus"}), prioStringSet{}, "4", ""},
		{ydlFormats, prioStringSet([]string{"opus"}), prioStringSet([]string{"vp9"}), "4", "2"},
	} {
		if err := testBestFormatCase(c.ydlFormats, c.aCodecs, c.vCodecs, c.aFormatID, c.vFormatID); err != nil {
			t.Error(err)
		}
	}
}

func TestFindBestFormats2(t *testing.T) {
	ydlFormats2 := []*youtubedl.Format{
		{FormatID: "1", Protocol: "http", NormACodec: "mp3", NormVCodec: "", NormBR: 0},
		{FormatID: "2", Protocol: "rtmp", NormACodec: "aac", NormVCodec: "h264", NormBR: 0},
		{FormatID: "3", Protocol: "https", NormACodec: "aac", NormVCodec: "h264", NormBR: 0},
	}

	for _, c := range []struct {
		ydlFormats []*youtubedl.Format
		aCodecs    prioStringSet
		vCodecs    prioStringSet
		aFormatID  string
		vFormatID  string
	}{
		{ydlFormats2, prioStringSet([]string{"mp3"}), prioStringSet{}, "1", ""},
	} {
		if err := testBestFormatCase(c.ydlFormats, c.aCodecs, c.vCodecs, c.aFormatID, c.vFormatID); err != nil {
			t.Error(err)
		}
	}
}
