package ffmpeg

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"
)

// ProbeInfo ffprobe result
type ProbeInfo struct {
	Format  map[string]interface{}   `json:"format"`
	Streams []map[string]interface{} `json:"streams"`
}

func (pi *ProbeInfo) findStringFiledStream(findField, findValue, field string) string {
	for _, fs := range pi.Streams {
		if s, _ := fs[findField].(string); s == findValue {
			v, _ := fs[field].(string)
			return v
		}
	}
	return ""
}

// VCodec probed video codec
func (pi *ProbeInfo) VCodec() string {
	return pi.findStringFiledStream("codec_type", "video", "codec_name")
}

// ACodec probed audio codec
func (pi *ProbeInfo) ACodec() string {
	return pi.findStringFiledStream("codec_type", "audio", "codec_name")
}

// FormatName probed format
func (pi *ProbeInfo) FormatName() string {
	v, _ := pi.Format["format_name"].(string)
	if fl := strings.Split(v, ","); len(fl) > 0 {
		return fl[0]
	}
	return ""
}

func (pi *ProbeInfo) String() string {
	return fmt.Sprintf("%s:%s:%s", pi.FormatName(), pi.ACodec(), pi.VCodec())
}

func probeInfoParse(r io.Reader) (pi *ProbeInfo, err error) {
	pi = &ProbeInfo{}
	d := json.NewDecoder(r)
	if err := d.Decode(&pi); err != nil {
		return nil, err
	}

	return pi, nil
}

// Probe run ffprobe with context
func Probe(ctx context.Context, r io.Reader, debugLog *log.Logger, stderr io.Writer) (pi *ProbeInfo, err error) {
	log := log.New(ioutil.Discard, "", 0)
	if debugLog != nil {
		log = debugLog
	}

	cmd := exec.CommandContext(
		ctx,
		"ffprobe",
		"-hide_banner",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		"pipe:0",
	)
	cmd.Stdin = r
	cmd.Stderr = stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	log.Printf("cmd %v", cmd.Args)

	if cmdErr := cmd.Start(); cmdErr != nil {
		return nil, cmdErr
	}

	var piErr error
	if pi, piErr = probeInfoParse(stdout); err != nil {
		return nil, piErr
	}

	if cmdErr := cmd.Wait(); cmdErr != nil {
		return nil, cmdErr
	}

	return pi, nil
}

// StreamMap stream mapping
type StreamMap struct {
	Stream          io.Reader
	StreamSpecifier string // 0, a:0, v:0, etc
	Codec           string
	CodecKind       string // audio/video
	CodecFlags      []string
}

// Format output format
type Format struct {
	Name  string
	Flags []string
}

// FFmpeg instance
type FFmpeg struct {
	StreamMaps []StreamMap
	Format     Format
	Stderr     io.Writer
	Stdout     io.WriteCloser
	DebugLog   *log.Logger

	cmd         *exec.Cmd
	cmdErr      error
	cmdWaitCh   chan error
	startWaitCh chan struct{}
}

func (f *FFmpeg) startAux(ctx context.Context, stdout io.WriteCloser) error {
	log := log.New(ioutil.Discard, "", 0)
	if f.DebugLog != nil {
		log = f.DebugLog
	}

	// figure out unique readers and create pipes for each
	type inputFD struct {
		r           *os.File
		w           *os.File
		childFD     int // fd in child process
		inputFileID int // ffmpeg input file id
	}
	inputToFDs := []*inputFD{}
	inputToFDMap := map[io.Reader]*inputFD{}

	// from os.Cmd "entry i becomes file descriptor 3+i"
	childFD := 3
	inputFileID := 0
	for _, m := range f.StreamMaps {
		// skip if reader already created
		if _, ok := inputToFDMap[m.Stream]; ok {
			continue
		}

		var err error
		ifd := &inputFD{childFD: childFD, inputFileID: inputFileID}
		childFD++
		inputFileID++
		ifd.r, ifd.w, err = os.Pipe()
		if err != nil {
			return err
		}
		go func(r io.Reader) {
			defer ifd.w.Close()
			io.Copy(ifd.w, r)
		}(m.Stream)

		inputToFDs = append(inputToFDs, ifd)
		inputToFDMap[m.Stream] = ifd
	}

	ffmpegName := "ffmpeg"
	ffmpegArgs := []string{"-hide_banner"}

	var extraFiles []*os.File
	for _, ifd := range inputToFDs {
		extraFiles = append(extraFiles, ifd.r)
		ffmpegArgs = append(ffmpegArgs, "-i", fmt.Sprintf("pipe:%d", ifd.childFD))
	}

	for _, m := range f.StreamMaps {
		ifd := inputToFDMap[m.Stream]

		ffmpegArgs = append(ffmpegArgs, "-map", fmt.Sprintf("%d:%s", ifd.inputFileID, m.StreamSpecifier))
		if m.CodecKind == "audio" {
			ffmpegArgs = append(ffmpegArgs, "-acodec")
		} else if m.CodecKind == "video" {
			ffmpegArgs = append(ffmpegArgs, "-vcodec")
		} else {
			panic(fmt.Sprintf("kind can only be audio or video (was %s)", m.CodecKind))
		}
		ffmpegArgs = append(ffmpegArgs, m.Codec)
		ffmpegArgs = append(ffmpegArgs, m.CodecFlags...)
	}

	ffmpegArgs = append(ffmpegArgs, "-f", f.Format.Name)
	ffmpegArgs = append(ffmpegArgs, f.Format.Flags...)
	ffmpegArgs = append(ffmpegArgs, "pipe:1")

	f.cmd = exec.CommandContext(ctx, ffmpegName, ffmpegArgs...)
	f.cmd.ExtraFiles = extraFiles
	f.cmd.Stderr = f.Stderr
	f.cmd.Stdout = stdout

	log.Printf("cmd %v", f.cmd.Args)

	if err := f.cmd.Start(); err != nil {
		return err
	}

	go func() {
		f.cmdWaitCh <- f.cmd.Wait()
		stdout.Close()
	}()

	return nil
}

// Start start ffmpeg with context and return once there is data to be read
func (f *FFmpeg) Start(ctx context.Context) error {
	f.cmdWaitCh = make(chan error, 1)
	f.startWaitCh = make(chan struct{}, 1)

	probeR, probeW := io.Pipe()
	if err := f.startAux(ctx, probeW); err != nil {
		probeR.Close()
		return err
	}

	probeByte := make([]byte, 1)
	if _, readErr := io.ReadFull(probeR, probeByte); readErr != nil {
		probeR.Close()
		f.cmdErr = <-f.cmdWaitCh
		if f.cmdErr != nil {
			return f.cmdErr
		}
		return readErr
	}

	go func() {
		f.Stdout.Write(probeByte)
		io.Copy(f.Stdout, probeR)
		probeR.Close()
		f.Stdout.Close()
		f.cmdErr = <-f.cmdWaitCh
		close(f.startWaitCh)
	}()

	return nil
}

// Wait for ffmpeg to finish
func (f *FFmpeg) Wait() error {
	<-f.startWaitCh
	return f.cmdErr
}
