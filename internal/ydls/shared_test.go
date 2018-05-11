package ydls

import (
	"os"
	"testing"
)

var youtubeTestVideoURL = "https://www.youtube.com/watch?v=C0DPdy98e4c"
var youtubeLongTestVideoURL = "https://www.youtube.com/watch?v=z7VYVjR_nwE"
var soundcloudTestAudioURL = "https://soundcloud.com/timsweeney/thedrifter"
var soundcloudTestPlaylistURL = "https://soundcloud.com/mattheis/sets/kindred-phenomena"

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
