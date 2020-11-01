package ydls

import (
	"os"
	"testing"

	"github.com/fortytw2/leaktest"
	"github.com/wader/osleaktest"
)

var youtubeTestVideoURL = "https://www.youtube.com/watch?v=C0DPdy98e4c"
var youtubeLongTestVideoURL = "https://www.youtube.com/watch?v=aqz-KE-bpKQ"
var soundcloudTestAudioURL = "https://soundcloud.com/avalonemerson/avalon-emerson-live-at-printworks-london-march-2017"
var soundcloudTestPlaylistURL = "https://soundcloud.com/mattheis/sets/kindred-phenomena"

var testExternal = os.Getenv("TEST_EXTERNAL") != ""

var ydlsLRetries = 3

func leakChecks(t *testing.T) func() {
	leakFn := leaktest.Check(t)
	osLeakFn := osleaktest.Check(t)

	return func() {
		leakFn()
		osLeakFn()
	}
}

func ydlsFromEnv(t *testing.T) YDLS {
	ydls, err := NewFromFile(os.Getenv("CONFIG"))
	if err != nil {
		t.Fatalf("failed to read config: %s", err)
	}

	ydls.Config.DownloadRetries = ydlsLRetries

	return ydls
}
