// TODO: count child processes
// TODO: ulimit proc and fds?
// TODO: test close reader prematurely

package ydls

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/wader/ydls/ffmpeg"
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
		{"https://www.youtube.com/watch?v=uVYWQJ5BB_w", false, "A Radiolab Producer on the Making of a Podcast"},
	} {
		for _, f := range *ydls.Formats {
			if c.audioOnly && len(f.VCodecs) > 0 {
				t.Logf("%s: %s: skip, audio only\n", c.url, f.Name)
				continue
			}

			ctx, cancelFn := context.WithCancel(context.Background())

			dr, err := ydls.Download(ctx, c.url, f.Name, nil)
			if err != nil {
				cancelFn()
				t.Errorf("%s: %s: download failed: %s", c.url, f.Name, err)
				continue
			}

			pi, err := ffmpeg.Probe(ctx, io.LimitReader(dr.Media, 10*1024*1024), nil, nil)
			dr.Media.Close()
			cancelFn()
			if err != nil {
				t.Errorf("%s: %s: probe failed: %s", c.url, f.Name, err)
				continue
			}

			if !strings.HasPrefix(dr.Filename, c.expectedFilename) {
				t.Errorf("%s: %s: expected filename '%s' found '%s'", c.url, f.Name, c.expectedFilename, dr.Filename)
				continue
			}
			if f.MIMEType != dr.MIMEType {
				t.Errorf("%s: %s: expected MIME type '%s' found '%s'", c.url, f.Name, f.MIMEType, dr.MIMEType)
				continue
			}
			if !stringsContains([]string(f.Formats), pi.FormatName()) {
				t.Errorf("%s: %s: expected format %s found %s", c.url, f.Name, f.Formats, pi.FormatName())
				continue
			}
			if len(f.ACodecs.CodecNames()) != 0 && !stringsContains(f.ACodecs.CodecNames(), pi.ACodec()) {
				t.Errorf("%s: %s: expected acodec %s found %s", c.url, f.Name, f.ACodecs.CodecNames(), pi.ACodec())
				continue
			}
			if len(f.VCodecs.CodecNames()) != 0 && !stringsContains(f.VCodecs.CodecNames(), pi.VCodec()) {
				t.Errorf("%s: %s: expected vcodec %s found %s", c.url, f.Name, f.VCodecs.CodecNames(), pi.VCodec())
				continue
			}

			t.Logf("%s: %s: OK (probed %s)\n", c.url, f.Name, pi)
		}

		// test raw format
		// TODO: what to check more?

		ctx, cancelFn := context.WithCancel(context.Background())

		dr, err := ydls.Download(ctx, c.url, "", nil)
		if err != nil {
			cancelFn()
			t.Errorf("%s: %s: download failed: %s", c.url, "raw", err)
			continue
		}

		pi, err := ffmpeg.Probe(ctx, io.LimitReader(dr.Media, 10*1024*1024), nil, nil)
		dr.Media.Close()
		cancelFn()
		if err != nil {
			t.Errorf("%s: %s: probe failed: %s", c.url, "raw", err)
			continue
		}

		t.Logf("%s: %s: OK (probed %s)\n", c.url, "raw", pi)
	}
}

func TestFail(t *testing.T) {
	if !testNetwork || !testFfmpeg || !testYoutubeldl {
		t.Skip("TEST_NETWORK, TEST_FFMPEG, TEST_YOUTUBEDL env not set")
	}

	ydls := ydlsFromFormatsEnv(t)

	geoBlockedURL := "https://www.youtube.com/watch?v=aaaaaaaaaaa"
	_, ydlsErr := ydls.Download(context.Background(), geoBlockedURL, "", nil)
	if ydlsErr == nil {
		t.Errorf("%s: should fail", geoBlockedURL)
	}

	expectedError := "aaaaaaaaaaa: YouTube said: This video does not exist."
	if ydlsErr.Error() != expectedError {
		t.Errorf("%s: expected '%s' got '%s'", geoBlockedURL, expectedError, ydlsErr.Error())
	}
}

func codecsToFormatCodecs(s string) prioFormatCodecSet {
	if s == "" {
		return prioFormatCodecSet{}
	}

	formatCodecs := []FormatCodec{}
	for _, c := range strings.Split(s, ",") {
		formatCodecs = append(formatCodecs, FormatCodec{Codec: c})
	}
	return prioFormatCodecSet(formatCodecs)
}

func testBestFormatCase(Formats []*youtubedl.Format, acodecs string, vcodecs string, aFormatID string, vFormatID string) error {
	aFormat, vFormat := findBestFormats(
		Formats,
		&Format{
			ACodecs: codecsToFormatCodecs(acodecs),
			VCodecs: codecsToFormatCodecs(vcodecs),
		},
	)

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
			acodecs, vcodecs,
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
		aCodecs    string
		vCodecs    string
		aFormatID  string
		vFormatID  string
	}{
		{ydlFormats, "mp3", "h264", "1", "1"},
		{ydlFormats, "mp3", "", "1", ""},
		{ydlFormats, "aac", "", "3", ""},
		{ydlFormats, "aac", "h264", "3", "2"},
		{ydlFormats, "opus", "", "4", ""},
		{ydlFormats, "opus", "v9", "4", "2"},
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
		aCodecs    string
		vCodecs    string
		aFormatID  string
		vFormatID  string
	}{
		{ydlFormats2, "mp3", "", "1", ""},
	} {
		if err := testBestFormatCase(c.ydlFormats, c.aCodecs, c.vCodecs, c.aFormatID, c.vFormatID); err != nil {
			t.Error(err)
		}
	}
}
