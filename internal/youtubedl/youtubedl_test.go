package youtubedl

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/wader/ydls/internal/leaktest"
)

var testNetwork = os.Getenv("TEST_NETWORK") != ""
var testYoutubeldl = os.Getenv("TEST_YOUTUBEDL") != ""

func TestParseInfo(t *testing.T) {
	if !testNetwork || !testYoutubeldl {
		t.Skip("TEST_NETWORK, TEST_YOUTUBEDL env not set")
	}

	for _, c := range []struct {
		url           string
		expectedTitle string
	}{
		{"https://soundcloud.com/timsweeney/thedrifter", "BIS Radio Show #793 with The Drifter"},
		{"https://vimeo.com/129701495", "Ben Nagy Fuzzing OSX At Scale"},
		{"https://www.infoq.com/presentations/Simple-Made-Easy", "Simple Made Easy"},
		{"https://www.youtube.com/watch?v=uVYWQJ5BB_w", "A Radiolab Producer on the Making of a Podcast"},
	} {
		func() {
			defer leaktest.Check(t)()

			ctx, cancelFn := context.WithCancel(context.Background())
			ydlResult, err := NewFromURL(ctx, c.url, Options{})
			if err != nil {
				cancelFn()
				t.Errorf("failed to parse %s: %v", c.url, err)
				return
			}
			cancelFn()

			yi := ydlResult.Info

			if yi.Title != c.expectedTitle {
				t.Errorf("%s: expected title '%s' got '%s'", c.url, c.expectedTitle, yi.Title)
			}

			if yi.Thumbnail != "" && len(yi.ThumbnailBytes) == 0 {
				t.Errorf("%s: expected thumbnail bytes", c.url)
			}

			var dummy map[string]interface{}
			if err := json.Unmarshal(ydlResult.RawJSON, &dummy); err != nil {
				t.Errorf("%s: failed to parse RawJSON", c.url)
			}

			if len(yi.Formats) == 0 {
				t.Errorf("%s: expected formats", c.url)
			}

			for _, f := range yi.Formats {
				if f.FormatID == "" {
					t.Errorf("%s: %s expected FormatID not empty", c.url, f.FormatID)
				}
				if f.ACodec != "" && f.ACodec != "none" && f.Ext != "" && f.NormACodec == "" {
					t.Errorf("%s: %s expected NormACodec not empty for %s", c.url, f.FormatID, f.ACodec)
				}
				if f.VCodec != "" && f.VCodec != "none" && f.Ext != "" && f.NormVCodec == "" {
					t.Errorf("%s: %s expected NormVCodec not empty for %s", c.url, f.FormatID, f.VCodec)
				}
				if f.ABR+f.VBR+f.TBR != 0 && f.NormBR == 0 {
					t.Errorf("%s: %s expected NormBR not zero", c.url, f.FormatID)
				}
			}

			t.Logf("%s: OK\n", c.url)
		}()
	}
}

func TestFail(t *testing.T) {
	if !testNetwork || !testYoutubeldl {
		t.Skip("TEST_NETWORK, TEST_YOUTUBEDL env not set")
	}

	defer leaktest.Check(t)()

	geoBlockedURL := "https://www.youtube.com/watch?v=aaaaaaaaaaa"
	_, err := NewFromURL(context.Background(), geoBlockedURL, Options{})

	if err == nil {
		t.Errorf("%s: should fail", geoBlockedURL)
	}

	expectedError := "aaaaaaaaaaa: YouTube said: This video is unavailable."
	if err.Error() != expectedError {
		t.Errorf("%s: expected '%s' got '%s'", geoBlockedURL, expectedError, err.Error())
	}
}

func TestPlaylist(t *testing.T) {
	if !testNetwork || !testYoutubeldl {
		t.Skip("TEST_NETWORK, TEST_YOUTUBEDL env not set")
	}

	defer leaktest.Check(t)()

	playlistRawURL := "https://soundcloud.com/mattheis/sets/kindred-phenomena"
	ydlResult, ydlResultErr := NewFromURL(context.Background(), playlistRawURL, Options{
		YesPlaylist:    true,
		SkipThumbnails: true,
	})

	if ydlResultErr != nil {
		t.Errorf("failed to download: %s", ydlResultErr)
	}

	expectedTitle := "Kindred Phenomena"
	if ydlResult.Info.Title != expectedTitle {
		t.Errorf("expected title \"%s\" got \"%s\"", expectedTitle, ydlResult.Info.Title)
	}

	expectedEntries := 8
	if len(ydlResult.Info.Entries) != expectedEntries {
		t.Errorf("expected %d entries got %d", expectedEntries, len(ydlResult.Info.Entries))
	}

	expectedTitleOne := "A1 Mattheis - Herds"
	if ydlResult.Info.Entries[0].Title != expectedTitleOne {
		t.Errorf("expected title \"%s\" got \"%s\"", expectedTitleOne, ydlResult.Info.Entries[0].Title)
	}
}

func TestPlaylistBadURL(t *testing.T) {
	if !testNetwork || !testYoutubeldl {
		t.Skip("TEST_NETWORK, TEST_YOUTUBEDL env not set")
	}

	defer leaktest.Check(t)()

	playlistRawURL := "https://soundcloud.com/timsweeney/thedrifter"
	_, ydlResultErr := NewFromURL(context.Background(), playlistRawURL, Options{
		YesPlaylist:    true,
		SkipThumbnails: true,
	})

	if ydlResultErr == nil {
		t.Error("expected error")
	}
}
