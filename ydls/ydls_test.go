package ydls

// TODO: test close reader prematurely

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/wader/ydls/ffmpeg"
	"github.com/wader/ydls/leaktest"
	"github.com/wader/ydls/youtubedl"
)

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

func ydlsFromFormatsEnv(t *testing.T) *YDLS {
	ydls, err := NewFromFile(os.Getenv("FORMATS"))
	if err != nil {
		t.Fatalf("failed to read formats: %s", err)
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

const youtbeuTestVideoURL = "https://www.youtube.com/watch?v=C0DPdy98e4c"

func TestFormats(t *testing.T) {
	if !testNetwork || !testFfmpeg || !testYoutubeldl {
		t.Skip("TEST_NETWORK, TEST_FFMPEG, TEST_YOUTUBEDL env not set")
	}

	ydls := ydlsFromFormatsEnv(t)

	for _, c := range []struct {
		url              string
		audioOnly        bool
		expectedFilename string
	}{
		{"https://soundcloud.com/timsweeney/thedrifter", true, "BIS Radio Show #793 with The Drifter"},
		{youtbeuTestVideoURL, false, "TEST VIDEO"},
	} {
		for _, f := range *ydls.Formats {
			func() {
				defer leaktest.Check(t)()

				if c.audioOnly && len(f.VCodecs) > 0 {
					t.Logf("%s: %s: skip, audio only\n", c.url, f.Name)
					return
				}

				ctx, cancelFn := context.WithCancel(context.Background())

				dr, err := ydls.Download(ctx, c.url, f.Name, DownloadOptions{})
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

	ydls := ydlsFromFormatsEnv(t)

	defer leaktest.Check(t)()

	ctx, cancelFn := context.WithCancel(context.Background())

	dr, err := ydls.Download(ctx, youtbeuTestVideoURL, "", DownloadOptions{})
	if err != nil {
		cancelFn()
		t.Errorf("%s: %s: download failed: %s", youtbeuTestVideoURL, "raw", err)
		return
	}

	pi, err := ffmpeg.Probe(ctx, io.LimitReader(dr.Media, 10*1024*1024), nil, nil)
	dr.Media.Close()
	dr.Wait()
	cancelFn()
	if err != nil {
		t.Errorf("%s: %s: probe failed: %s", youtbeuTestVideoURL, "raw", err)
		return
	}

	t.Logf("%s: %s: OK (probed %s)\n", youtbeuTestVideoURL, "raw", pi)
}

func TestForceCodec(t *testing.T) {
	if !testNetwork || !testFfmpeg || !testYoutubeldl {
		t.Skip("TEST_NETWORK, TEST_FFMPEG, TEST_YOUTUBEDL env not set")
	}

	ydls := ydlsFromFormatsEnv(t)

	defer leaktest.Check(t)()

	ctx, cancelFn := context.WithCancel(context.Background())

	mkvFormat := ydls.Formats.FindByName("mkv")
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

	dr, err := ydls.Download(ctx, youtbeuTestVideoURL, mkvFormat.Name, DownloadOptions{ForceACodec: forceACodec, ForceVCodec: forceVCodec})
	if err != nil {
		cancelFn()
		t.Errorf("%s: %s: download failed: %s", youtbeuTestVideoURL, "raw", err)
		return
	}

	pi, err := ffmpeg.Probe(ctx, io.LimitReader(dr.Media, 10*1024*1024), nil, nil)
	dr.Media.Close()
	dr.Wait()
	cancelFn()
	if err != nil {
		t.Errorf("%s: probe failed: %s", youtbeuTestVideoURL, err)
		return
	}

	if pi.FormatName() != "matroska" || pi.ACodec() != forceACodec || pi.VCodec() != forceVCodec {
		t.Errorf("%s: force codec failed: found %s", youtbeuTestVideoURL, pi)
		return
	}

	t.Logf("%s: OK (probed %s)\n", youtbeuTestVideoURL, pi)
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
