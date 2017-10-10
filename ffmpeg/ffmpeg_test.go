package ffmpeg

import (
	"bytes"
	"context"
	"log"
	"os"
	"testing"
	"time"

	"github.com/wader/ydls/leaktest"
)

var testFfmpeg = os.Getenv("TEST_FFMPEG") != ""

func TestDurationToPosition(t *testing.T) {
	for _, tc := range []struct {
		duration time.Duration
		expected string
	}{
		{time.Duration(1) * time.Second, "0:00:01"},
		{time.Duration(59) * time.Second, "0:00:59"},
		{time.Duration(60) * time.Second, "0:01:00"},
		{time.Duration(61) * time.Second, "0:01:01"},
		{time.Duration(3599) * time.Second, "0:59:59"},
		{time.Duration(3600) * time.Second, "1:00:00"},
		{time.Duration(3601) * time.Second, "1:00:01"},
		{time.Duration(100) * time.Hour, "100:00:00"},
	} {
		if v := DurationToPosition(tc.duration); v != tc.expected {
			t.Errorf("Expected %v to be %s, got %s", tc.duration, tc.expected, v)
		}
	}
}

func TestProbe(t *testing.T) {
	if !testFfmpeg {
		t.Skip("TEST_FFMPEG env not set")
	}

	defer leaktest.Check(t)()

	dummy, dummyErr := Dummy("matroska", "mp3", "h264")
	if dummyErr != nil {
		log.Fatal(dummyErr)
	}

	pi, probeErr := Probe(context.Background(), dummy, nil, nil)
	if probeErr != nil {
		t.Error(probeErr)
	}

	if pi.FormatName() != "matroska" {
		t.Errorf("FormatName should be matroska, is %s", pi.FormatName())
	}
	if pi.ACodec() != "mp3" {
		t.Errorf("ACodec should be mp3, is %s", pi.ACodec())
	}
	if pi.VCodec() != "h264" {
		t.Errorf("VCodec should be h264, is %s", pi.VCodec())
	}
	if v := pi.Duration().Seconds(); v != 1 {
		t.Errorf("Duration should be %v, is %v", 1, v)
	}
}

type closeBuffer struct {
	bytes.Buffer
}

func (closeBuffer) Close() error {
	return nil
}

func TestStart(t *testing.T) {
	if !testFfmpeg {
		t.Skip("TEST_FFMPEG env not set")
	}

	defer leaktest.Check(t)()

	dummy, dummyErr := Dummy("matroska", "mp3", "h264")
	if dummyErr != nil {
		log.Fatal(dummyErr)
	}
	output := &closeBuffer{}

	ffmpegP := &FFmpeg{
		StreamMaps: []StreamMap{
			StreamMap{
				Reader:    dummy,
				Specifier: "a:0",
				Codec:     "acodec:vorbis",
			},
			StreamMap{
				Reader:    dummy,
				Specifier: "v:0",
				Codec:     "vcodec:vp8",
			},
		},
		Format:   Format{Name: "matroska"},
		DebugLog: nil, // log.New(os.Stdout, "debug> ", 0),
		Stderr:   nil, // writelogger.New(log.New(os.Stdout, "stderr> ", 0), ""),
		Stdout:   output,
	}

	if err := ffmpegP.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	ffmpegP.Wait()

	pi, piErr := Probe(context.Background(), bytes.NewBuffer(output.Bytes()), nil, nil)
	if piErr != nil {
		t.Fatal(piErr)
	}

	if pi.FormatName() != "matroska" {
		t.Fatalf("FormatName should be matroska, is %s", pi.FormatName())
	}
	if pi.ACodec() != "vorbis" {
		t.Fatalf("ACodec should be vorbis, is %s", pi.ACodec())
	}
	if pi.VCodec() != "vp8" {
		t.Fatalf("VCodec should be vp8, is %s", pi.VCodec())
	}
}
