package ydls

// TODO: test close reader prematurely

import (
	"context"
	"encoding/xml"
	"io"
	"io/ioutil"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wader/goutubedl"

	"github.com/wader/ydls/internal/ffmpeg"
	"github.com/wader/ydls/internal/rss"
	"github.com/wader/ydls/internal/stringprioset"
	"github.com/wader/ydls/internal/timerange"
)

func TestSafeFilename(t *testing.T) {
	for _, c := range []struct {
		s      string
		expect string
	}{
		{"aba", "aba"},
		{"a/a", "a_a"},
		{"a\\a", "a_a"},
	} {
		t.Run(c.s, func(t *testing.T) {
			actual := safeFilename(c.s)
			if actual != c.expect {
				t.Errorf("got %v expected %v", actual, c.expect)
			}
		})
	}
}

func TestForceCodec(t *testing.T) {
	if !testExternal {
		t.Skip("TEST_EXTERNAL")
	}

	defer leakChecks(t)()

	ydls := ydlsFromEnv(t)
	const formatName = "mkv"
	mkvFormat, _ := ydls.Config.Formats.FindByName(formatName)
	forceCodecs := []string{"opus", "vp9"}

	// make sure codecs are not the perferred ones
	for _, s := range mkvFormat.Streams {
		for _, forceCodec := range forceCodecs {
			if c, ok := s.CodecNames.First(); ok && c == forceCodec {
				t.Errorf("test sanity check failed: codec already the preferred one")
				return
			}
		}
	}

	ctx, cancelFn := context.WithCancel(context.Background())

	dr, err := ydls.Download(ctx,
		DownloadOptions{
			RequestOptions: RequestOptions{
				MediaRawURL: youtubeTestVideoURL,
				Format:      &mkvFormat,
				Codecs:      forceCodecs,
			},
		},
	)
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
	if !testExternal {
		t.Skip("TEST_EXTERNAL")
	}

	defer leakChecks(t)()

	ydls := ydlsFromEnv(t)
	const formatName = "mkv"
	mkvFormat, _ := ydls.Config.Formats.FindByName(formatName)

	timeRange, timeRangeErr := timerange.NewTimeRangeFromString("10s-15s")
	if timeRangeErr != nil {
		t.Fatalf("failed to parse time range")
	}

	ctx, cancelFn := context.WithCancel(context.Background())

	dr, err := ydls.Download(ctx,
		DownloadOptions{
			RequestOptions: RequestOptions{
				MediaRawURL: youtubeTestVideoURL,
				Format:      &mkvFormat,
				TimeRange:   timeRange,
			},
		},
	)
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
	if !testExternal {
		t.Skip("TEST_EXTERNAL")
	}

	defer leakChecks(t)()

	ydls := ydlsFromEnv(t)
	const formatName = "mkv"
	mkvFormat, _ := ydls.Config.Formats.FindByName(formatName)

	ctx, cancelFn := context.WithCancel(context.Background())

	_, err := ydls.Download(ctx,
		DownloadOptions{
			RequestOptions: RequestOptions{
				MediaRawURL: soundcloudTestAudioURL,
				Format:      &mkvFormat,
			},
		},
	)
	cancelFn()
	if err == nil {
		t.Fatal("expected download to fail")
	}
}

func TestSortYDLFormats(t *testing.T) {
	ydlFormats := []goutubedl.Format{
		{FormatID: "1", Protocol: "http", ACodec: "mp3", VCodec: "h264", TBR: 1},
		{FormatID: "2", Protocol: "http", ACodec: "", VCodec: "h264", TBR: 2},
		{FormatID: "3", Protocol: "http", ACodec: "aac", VCodec: "", TBR: 3},
		{FormatID: "4", Protocol: "http", ACodec: "vorbis", VCodec: "vp8", TBR: 4},
		{FormatID: "5", Protocol: "http", ACodec: "opus", VCodec: "vp9", TBR: 5},
	}

	for i, c := range []struct {
		ydlFormats       []goutubedl.Format
		mediaType        mediaType
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
		actualFormats := sortYDLFormats(c.ydlFormats, c.mediaType, c.codecs)
		if len(actualFormats) > 0 && actualFormats[0].FormatID != c.expectedFormatID {
			t.Errorf("%d: expected format %s, got %s", i, c.expectedFormatID, actualFormats)
		}
	}
}

func TestContextCloseProbe(t *testing.T) {
	if !testExternal {
		t.Skip("TEST_EXTERNAL")
	}

	defer leakChecks(t)()

	ydls := ydlsFromEnv(t)
	const formatName = "mkv"
	mkvFormat, _ := ydls.Config.Formats.FindByName(formatName)

	ctx, cancelFn := context.WithCancel(context.Background())

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		time.Sleep(2 * time.Second)
		cancelFn()
		wg.Done()
	}()
	_, err := ydls.Download(ctx,
		DownloadOptions{
			RequestOptions: RequestOptions{
				MediaRawURL: youtubeLongTestVideoURL,
				Format:      &mkvFormat,
			},
		},
	)
	if err == nil {
		t.Error("expected error while probe")
	}
	cancelFn()
	wg.Wait()
}

func TestContextCloseDownload(t *testing.T) {
	if !testExternal {
		t.Skip("TEST_EXTERNAL")
	}

	defer leakChecks(t)()

	ydls := ydlsFromEnv(t)
	const formatName = "mkv"
	mkvFormat, _ := ydls.Config.Formats.FindByName(formatName)

	ctx, cancelFn := context.WithCancel(context.Background())

	dr, err := ydls.Download(ctx,
		DownloadOptions{
			RequestOptions: RequestOptions{
				MediaRawURL: youtubeLongTestVideoURL,
				Format:      &mkvFormat,
			},
		},
	)
	if err != nil {
		t.Error("expected no error while download")
	}
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		time.Sleep(1 * time.Second)
		cancelFn()
		wg.Done()
	}()
	io.Copy(ioutil.Discard, dr.Media)
	cancelFn()
	wg.Wait()
}

func TestRSS(t *testing.T) {
	if !testExternal {
		t.Skip("TEST_EXTERNAL")
	}

	defer leakChecks(t)()

	ydls := ydlsFromEnv(t)
	const formatName = "rss"
	rssFormat, _ := ydls.Config.Formats.FindByName(formatName)

	ctx, cancelFn := context.WithCancel(context.Background())

	dr, err := ydls.Download(ctx,
		DownloadOptions{
			RequestOptions: RequestOptions{
				MediaRawURL: soundcloudTestPlaylistURL,
				Format:      &rssFormat,
				Items:       2,
			},
			BaseURL: &url.URL{Scheme: "http", Host: "dummy"},
		},
	)
	if err != nil {
		cancelFn()
		t.Fatalf("%s: download failed: %s", soundcloudTestPlaylistURL, err)
	}
	defer cancelFn()

	if dr.Filename != "" {
		t.Errorf("expected no filename, got %s", dr.Filename)
	}
	expectedMIMEType := "text/xml"
	if dr.MIMEType != expectedMIMEType {
		t.Errorf("expected mimetype %s, got %s", expectedMIMEType, dr.MIMEType)
	}

	rssRoot := rss.RSS{}
	decoder := xml.NewDecoder(dr.Media)
	decodeErr := decoder.Decode(&rssRoot)
	if decodeErr != nil {
		t.Errorf("failed to parse rss: %s", decodeErr)
		return
	}
	dr.Media.Close()
	dr.Wait()

	expectedTitle := "Kindred Phenomena"
	if rssRoot.Channel.Title != expectedTitle {
		t.Errorf("expected title \"%s\" got \"%s\"", expectedTitle, rssRoot.Channel.Title)
	}

	// TODO: description, not returned by youtube-dl?

	expectedItemsCount := 2
	if len(rssRoot.Channel.Items) != expectedItemsCount {
		t.Errorf("expected %d items got %d", expectedItemsCount, len(rssRoot.Channel.Items))
	}

	itemOne := rssRoot.Channel.Items[0]

	expectedItemTitle := "A1 Mattheis - Herds"
	if rssRoot.Channel.Items[0].Title != expectedItemTitle {
		t.Errorf("expected title \"%s\" got \"%s\"", expectedItemTitle, itemOne.Title)
	}

	expectedItemDescriptionPrefix := "Releasing my debut"
	if !strings.HasPrefix(rssRoot.Channel.Items[0].Description, expectedItemDescriptionPrefix) {
		t.Errorf("expected description prefix \"%s\" got \"%s\"", expectedItemDescriptionPrefix, itemOne.Description)
	}

	expectedItemGUID := "http://dummy/mp3/https://soundcloud.com/mattheis/sets/kindred-phenomena#293285002"
	if rssRoot.Channel.Items[0].GUID != expectedItemGUID {
		t.Errorf("expected guid \"%s\" got \"%s\"", expectedItemGUID, itemOne.GUID)
	}

	expectedItemURL := "http://dummy/media.mp3?format=mp3&url=https%3A%2F%2Fsoundcloud.com%2Fmattheis%2Fa1-mattheis-herds"
	if itemOne.Enclosure.URL != expectedItemURL {
		t.Errorf("expected enclousure url \"%s\" got \"%s\"", expectedItemURL, itemOne.Enclosure.URL)
	}

	expectedItemType := "audio/mpeg"
	if itemOne.Enclosure.Type != expectedItemType {
		t.Errorf("expected enclousure type \"%s\" got \"%s\"", expectedItemType, itemOne.Enclosure.Type)
	}
}

func TestRSSStructure(t *testing.T) {
	if !testExternal {
		t.Skip("TEST_EXTERNAL")
	}

	rawXML := `
<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd">
  <channel>
    <item>
    </item>
  </channel>
</rss>
`
	rssRoot := rss.RSS{}
	decodeErr := xml.Unmarshal([]byte(rawXML), &rssRoot)
	if decodeErr != nil {
		t.Errorf("failed to parse rss: %s", decodeErr)
		return
	}

	expectedItemsCount := 1
	if len(rssRoot.Channel.Items) != expectedItemsCount {
		t.Errorf("expected %d items got %d", expectedItemsCount, len(rssRoot.Channel.Items))
	}
}

func TestSubtitles(t *testing.T) {
	if !testExternal {
		t.Skip("TEST_EXTERNAL")
	}

	subtitlesTestVideoURL := "https://www.youtube.com/watch?v=QRS8MkLhQmM"
	ydls := ydlsFromEnv(t)

	for _, f := range ydls.Config.Formats {
		if f.SubtitleCodecs.Empty() {
			continue
		}

		dr, drErr := ydls.Download(context.Background(),
			DownloadOptions{
				RequestOptions: RequestOptions{
					MediaRawURL: subtitlesTestVideoURL,
					Format:      &f,
					TimeRange:   timerange.TimeRange{Stop: timerange.Duration(time.Second * 2)},
				},
			},
		)
		if drErr != nil {
			t.Fatalf("%s: download failed: %s", subtitlesTestVideoURL, drErr)
		}

		pi, piErr := ffmpeg.Probe(context.Background(), ffmpeg.Reader{Reader: dr.Media}, nil, nil)
		dr.Media.Close()
		dr.Wait()
		if piErr != nil {
			t.Errorf("%s: %s: probe failed: %s", subtitlesTestVideoURL, f.Name, piErr)
			return
		}

		subtitlesStreamCount := 0
		expectedSubtitlesStreamCount := 13
		for _, s := range pi.Streams {
			if s.CodecType == "subtitle" {
				subtitlesStreamCount++
			}
		}

		if subtitlesStreamCount != expectedSubtitlesStreamCount {
			t.Errorf("%s: %s: expected %d got %d", subtitlesTestVideoURL, f.Name, expectedSubtitlesStreamCount, subtitlesStreamCount)
		}
	}
}

func TestDownloadFormatFallback(t *testing.T) {
	if !testExternal {
		t.Skip("TEST_EXTERNAL")
	}

	defer leakChecks(t)()

	ydls := ydlsFromEnv(t)
	const formatName = "mp3"
	const formatFallbackTestURL = "https://www.infoq.com/presentations/rust-thread-safety"
	format, _ := ydls.Config.Formats.FindByName(formatName)

	ctx, cancelFn := context.WithCancel(context.Background())

	dr, err := ydls.Download(ctx,
		DownloadOptions{
			RequestOptions: RequestOptions{
				MediaRawURL: formatFallbackTestURL,
				Format:      &format,
				TimeRange:   timerange.TimeRange{Stop: timerange.Duration(time.Second * 2)},
			},
		},
	)
	if err != nil {
		t.Error("expected no error while download")
	}
	io.Copy(ioutil.Discard, dr.Media)
	cancelFn()
}
