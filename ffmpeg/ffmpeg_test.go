package ffmpeg

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/wader/ydls/leaktest"
)

var testFfmpeg = os.Getenv("TEST_FFMPEG") != ""

func dummyFile(t *testing.T, format string, acodec string, vcodec string) io.Reader {
	var err error

	// file with black screen and no sound
	dummyFileCmd := exec.Command(
		"ffmpeg",
		"-f", "lavfi", "-i", "color=s=cga:d=1",
		"-f", "lavfi", "-i", "anullsrc",
		"-map", "0:0", "-acodec", acodec,
		"-map", "1:0", "-vcodec", vcodec,
		"-t", "1",
		"-f", format,
		"-",
	)

	stdoutBuf := &bytes.Buffer{}
	stderrBuf := &bytes.Buffer{}
	dummyFileCmd.Stdout = stdoutBuf
	dummyFileCmd.Stderr = stderrBuf

	if err = dummyFileCmd.Run(); err != nil {
		t.Logf("cmd failed: %s", strings.Join(dummyFileCmd.Args, " "))
		t.Log(string(stderrBuf.Bytes()))
		t.Fatal(err)
	}

	return stdoutBuf
}

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

	pi, probeErr := Probe(context.Background(), dummyFile(t, "matroska", "mp3", "h264"), nil, nil)
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

	file := dummyFile(t, "matroska", "mp3", "h264")
	output := &closeBuffer{}

	ffmpegP := &FFmpeg{
		StreamMaps: []StreamMap{
			StreamMap{
				Reader:    file,
				Specifier: "a:0",
				Codec:     "acodec:vorbis",
			},
			StreamMap{
				Reader:    file,
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
