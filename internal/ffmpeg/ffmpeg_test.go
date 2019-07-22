package ffmpeg

// TODO: test URLOutput, mixed
// TODO: ffmpeg pipeline

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"log"
	"os"
	"testing"
	"time"

	"github.com/fortytw2/leaktest"
	"github.com/wader/osleaktest"
)

var testExternal = os.Getenv("TEST_EXTERNAL") != ""

func leakChecks(t *testing.T) func() {
	leakFn := leaktest.Check(t)
	osLeakFn := osleaktest.Check(t)

	return func() {
		leakFn()
		osLeakFn()
	}
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

func mustDummy(t *testing.T, format string, acodec string, vcodec string) io.Reader {
	dummy, dummyErr := Dummy("matroska", "mp3", "h264")
	if dummyErr != nil {
		t.Fatal(dummyErr)
	}
	return dummy
}

func TestProbe(t *testing.T) {
	if !testExternal {
		t.Skip("TEST_EXTERNAL")
	}

	defer leakChecks(t)()

	dummy := mustDummy(t, "matroska", "mp3", "h264")

	pi, probeErr := Probe(context.Background(), Reader{Reader: dummy}, nil, nil)
	if probeErr != nil {
		t.Error(probeErr)
	}

	if pi.FormatName() != "matroska" {
		t.Errorf("FormatName should be matroska, is %s", pi.FormatName())
	}
	if pi.AudioCodec() != "mp3" {
		t.Errorf("AudioCodec should be mp3, is %s", pi.AudioCodec())
	}
	if pi.VideoCodec() != "h264" {
		t.Errorf("VideoCodec should be h264, is %s", pi.VideoCodec())
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

func TestReader(t *testing.T) {
	if !testExternal {
		t.Skip("TEST_EXTERNAL")
	}

	defer leakChecks(t)()

	dummy1 := mustDummy(t, "matroska", "mp3", "h264")
	dummy2 := mustDummy(t, "matroska", "mp3", "h264")
	output := &closeBuffer{}

	ffmpegP := &FFmpeg{
		Streams: []Stream{
			Stream{
				Maps: []Map{
					Map{
						Input:     Reader{Reader: dummy1},
						Specifier: "a:0",
						Codec:     AudioCodec("libvorbis"),
					},
					Map{
						Input:     Reader{Reader: dummy2},
						Specifier: "v:0",
						Codec:     VideoCodec("vp8"),
					},
				},
				Format: Format{Name: "matroska"},
				Output: Writer{Writer: output},
			},
		},
		// DebugLog: log.New(os.Stdout, "debug> ", 0),
		// Stderr:   printwriter.New(log.New(os.Stdout, "stderr> ", 0), ""),
	}

	if err := ffmpegP.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	ffmpegP.Wait()

	pi, piErr := Probe(context.Background(), Reader{Reader: bytes.NewBuffer(output.Bytes())}, nil, nil)
	if piErr != nil {
		t.Fatal(piErr)
	}

	if pi.FormatName() != "matroska" {
		t.Fatalf("FormatName should be matroska, is %s", pi.FormatName())
	}
	if pi.AudioCodec() != "vorbis" {
		t.Fatalf("AudioCodec should be vorbis, is %s", pi.AudioCodec())
	}
	if pi.VideoCodec() != "vp8" {
		t.Fatalf("VideoCodec should be vp8, is %s", pi.VideoCodec())
	}
}

func TestURLInput(t *testing.T) {
	if !testExternal {
		t.Skip("TEST_EXTERNAL")
	}

	defer leakChecks(t)()

	dummy1 := mustDummy(t, "matroska", "mp3", "h264")
	tempFile1, tempFile1Err := ioutil.TempFile("", "TestURLInput")
	if tempFile1Err != nil {
		log.Fatal(tempFile1)
	}
	defer os.Remove(tempFile1.Name())
	io.Copy(tempFile1, dummy1)
	tempFile1.Close()

	dummy2 := mustDummy(t, "matroska", "mp3", "h264")
	tempFile2, tempFile2Err := ioutil.TempFile("", "TestURLInput")
	if tempFile2Err != nil {
		log.Fatal(tempFile2Err)
	}
	defer os.Remove(tempFile2.Name())
	io.Copy(tempFile2, dummy2)
	tempFile2.Close()

	output := &closeBuffer{}

	ffmpegP := &FFmpeg{
		Streams: []Stream{
			Stream{
				Maps: []Map{
					Map{
						Input:     URL(tempFile1.Name()),
						Specifier: "a:0",
						Codec:     AudioCodec("libvorbis"),
					},
					Map{
						Input:     URL(tempFile2.Name()),
						Specifier: "v:0",
						Codec:     VideoCodec("vp8"),
					},
				},
				Format: Format{Name: "matroska"},
				Output: Writer{Writer: output},
			},
		},
		// DebugLog: log.New(os.Stdout, "debug> ", 0),
		// Stderr:   printwriter.New(log.New(os.Stdout, "stderr> ", 0), ""),
	}

	if err := ffmpegP.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	ffmpegP.Wait()

	pi, piErr := Probe(context.Background(), Reader{Reader: bytes.NewBuffer(output.Bytes())}, nil, nil)
	if piErr != nil {
		t.Fatal(piErr)
	}

	if pi.FormatName() != "matroska" {
		t.Fatalf("FormatName should be matroska, is %s", pi.FormatName())
	}
	if pi.AudioCodec() != "vorbis" {
		t.Fatalf("AudioCodec should be vorbis, is %s", pi.AudioCodec())
	}
	if pi.VideoCodec() != "vp8" {
		t.Fatalf("VideoCodec should be vp8, is %s", pi.VideoCodec())
	}
}

func TestWriterOutput(t *testing.T) {
	if !testExternal {
		t.Skip("TEST_EXTERNAL")
	}

	defer leakChecks(t)()

	dummy1 := mustDummy(t, "matroska", "mp3", "h264")
	outputAudio := &closeBuffer{}
	outputVideo := &closeBuffer{}

	ffmpegP := &FFmpeg{
		Streams: []Stream{
			Stream{
				Maps: []Map{
					Map{
						Input:     Reader{Reader: dummy1},
						Specifier: "a:0",
						Codec:     AudioCodec("copy"),
					},
				},
				Format: Format{Name: "matroska"},
				Output: Writer{Writer: outputAudio},
			},
			Stream{
				Maps: []Map{
					Map{
						Input:     Reader{Reader: dummy1},
						Specifier: "v:0",
						Codec:     VideoCodec("copy"),
					},
				},
				Format: Format{Name: "matroska"},
				Output: Writer{Writer: outputVideo},
			},
		},
		// DebugLog: log.New(os.Stdout, "debug> ", 0),
		// Stderr:   printwriter.New(log.New(os.Stdout, "stderr> ", 0), ""),
	}

	if err := ffmpegP.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	ffmpegP.Wait()

	piAudio, piErr := Probe(context.Background(), Reader{Reader: bytes.NewBuffer(outputAudio.Bytes())}, nil, nil)
	if piErr != nil {
		t.Fatal(piErr)
	}

	if piAudio.FormatName() != "matroska" {
		t.Fatalf("FormatName should be matroska, is %s", piAudio.FormatName())
	}
	if piAudio.AudioCodec() != "mp3" {
		t.Fatalf("AudioCodec should be mp3, is %s", piAudio.AudioCodec())
	}
	if piAudio.VideoCodec() != "" {
		t.Fatalf("VideoCodec should be none, is %s", piAudio.VideoCodec())
	}

	piVideo, piErr := Probe(context.Background(), Reader{Reader: bytes.NewBuffer(outputVideo.Bytes())}, nil, nil)
	if piErr != nil {
		t.Fatal(piErr)
	}

	if piVideo.FormatName() != "matroska" {
		t.Fatalf("FormatName should be matroska, is %s", piVideo.FormatName())
	}
	if piVideo.AudioCodec() != "" {
		t.Fatalf("AudioCodec should be none, is %s", piVideo.AudioCodec())
	}
	if piVideo.VideoCodec() != "h264" {
		t.Fatalf("VideoCodec should be h264, is %s", piVideo.VideoCodec())
	}
}

func TestMetadataMap(t *testing.T) {
	if v := (Metadata{Artist: "a"}).Map()["artist"]; v != "a" {
		t.Fatalf("Metadata artist should be a, is %s", v)
	}
}

func TestMetadataMerge(t *testing.T) {
	m := (Metadata{Artist: "a"}).Merge(Metadata{Artist: "b", Title: "b"})

	if v := m.Artist; v != "a" {
		t.Fatalf("Metadata artist should be a, is %s", v)
	}
	if v := m.Title; v != "b" {
		t.Fatalf("Metadata artist should be b, is %s", v)
	}
}
