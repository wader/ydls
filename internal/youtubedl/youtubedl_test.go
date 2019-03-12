package youtubedl

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/wader/ydls/internal/leaktest"
)

var testExternal = os.Getenv("TEST_EXTERNAL") != ""

func TestParseInfo(t *testing.T) {
	if !testExternal {
		t.Skip("TEST_EXTERNAL")
	}

	for _, c := range []struct {
		url           string
		expectedTitle string
	}{
		{"https://soundcloud.com/avalonemerson/avalon-emerson-live-at-printworks-london-march-2017", "Avalon Emerson Live at Printworks London"},
		{"https://vimeo.com/129701495", "Ben Nagy Fuzzing OSX At Scale"},
		{"https://www.infoq.com/presentations/Simple-Made-Easy", "Simple Made Easy"},
		{"https://www.youtube.com/watch?v=uVYWQJ5BB_w", "A Radiolab Producer on the Making of a Podcast"},
	} {
		func() {
			defer leaktest.Check(t)()

			ctx, cancelFn := context.WithCancel(context.Background())
			ydlResult, err := New(ctx, c.url, Options{
				DownloadThumbnail: true,
			})
			if err != nil {
				cancelFn()
				t.Errorf("failed to parse %s: %v", c.url, err)
				return
			}
			cancelFn()

			yi := ydlResult.Info
			results := ydlResult.Formats()

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

			if len(results) == 0 {
				t.Errorf("%s: expected formats", c.url)
			}

			for _, f := range results {
				if f.FormatID == "" {
					t.Errorf("%s: %s expected FormatID not empty", c.url, f.FormatID)
				}
				if f.ACodec != "" && f.ACodec != "none" && f.Ext != "" && f.NormalizedACodec() == "" {
					t.Errorf("%s: %s expected NormalizedACodec not empty for %s", c.url, f.FormatID, f.ACodec)
				}
				if f.VCodec != "" && f.VCodec != "none" && f.Ext != "" && f.NormalizedVCodec() == "" {
					t.Errorf("%s: %s expected NormalizedVCodec not empty for %s", c.url, f.FormatID, f.VCodec)
				}
				if f.ABR+f.VBR+f.TBR != 0 && f.NormalizedBR() == 0 {
					t.Errorf("%s: %s expected NormalizedBR not zero", c.url, f.FormatID)
				}
			}

			t.Logf("%s: OK\n", c.url)
		}()
	}
}

func TestPlaylist(t *testing.T) {
	if !testExternal {
		t.Skip("TEST_EXTERNAL")
	}

	defer leaktest.Check(t)()

	playlistRawURL := "https://soundcloud.com/mattheis/sets/kindred-phenomena"
	ydlResult, ydlResultErr := New(context.Background(), playlistRawURL, Options{
		YesPlaylist:       true,
		DownloadThumbnail: false,
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
	if !testExternal {
		t.Skip("TEST_EXTERNAL")
	}

	defer leaktest.Check(t)()

	playlistRawURL := "https://soundcloud.com/avalonemerson/avalon-emerson-live-at-printworks-london-march-2017"
	_, ydlResultErr := New(context.Background(), playlistRawURL, Options{
		YesPlaylist:       true,
		DownloadThumbnail: false,
	})

	if ydlResultErr == nil {
		t.Error("expected error")
	}
}

func TestSubtitles(t *testing.T) {
	if !testExternal {
		t.Skip("TEST_EXTERNAL")
	}

	subtitlesTestVideoURL := "https://www.youtube.com/watch?v=QRS8MkLhQmM"

	ydlResult, ydlResultErr := New(
		context.Background(),
		subtitlesTestVideoURL,
		Options{
			DownloadSubtitles: true,
		})

	if ydlResultErr != nil {
		t.Errorf("failed to download: %s", ydlResultErr)
	}

	for _, subtitles := range ydlResult.Info.Subtitles {
		for _, subtitle := range subtitles {
			if subtitle.Ext == "" {
				t.Errorf("%s: %s: expected extension", ydlResult.Info.URL, subtitle.Language)
			}
			if subtitle.Language == "" {
				t.Errorf("%s: %s: expected language", ydlResult.Info.URL, subtitle.Language)
			}
			if subtitle.URL == "" {
				t.Errorf("%s: %s: expected url", ydlResult.Info.URL, subtitle.Language)
			}
			if len(subtitle.Bytes) == 0 {
				t.Errorf("%s: %s: expected bytes", ydlResult.Info.URL, subtitle.Language)
			}
		}
	}
}
