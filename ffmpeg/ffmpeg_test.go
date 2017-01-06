package ffmpeg

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
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
		"-shortest",
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

func TestProbe(t *testing.T) {
	if !testFfmpeg {
		t.SkipNow()
	}

	pi, probeErr := Probe(context.Background(), dummyFile(t, "matroska", "mp3", "h264"), nil, nil)
	if probeErr != nil {
		t.Error(probeErr)
	}

	if pi.FormatName() != "matroska" {
		t.Fatalf("FormatName should be matroska, is %s", pi.FormatName())
	}
	if pi.ACodec() != "mp3" {
		t.Fatalf("ACodec should be mp3, is %s", pi.ACodec())
	}
	if pi.VCodec() != "h264" {
		t.Fatalf("VCodec should be h264, is %s", pi.VCodec())
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
		t.SkipNow()
	}

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
