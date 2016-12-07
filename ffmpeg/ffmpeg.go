package ffmpeg

import (
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

// FFprobe run ffprobe
func FFprobe(r io.Reader, debugLog *log.Logger, stderr io.Writer) (pi *ProbeInfo, err error) {
	log := log.New(ioutil.Discard, "", 0)
	if debugLog != nil {
		log = debugLog
	}

	cmd := exec.Command(
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

// Map stream mapping
type Map struct {
	Input           io.Reader
	Kind            string // audio/video
	StreamSpecifier string // 0, a:0, v:0
	Codec           string
	Flags           []string
}

// Format output format
type Format struct {
	Name  string
	Flags []string
}

// FFmpeg instance
type FFmpeg struct {
	Maps     []Map
	Format   Format
	Stderr   io.Writer
	Stdout   io.WriteCloser
	DebugLog *log.Logger

	cmd       *exec.Cmd
	cmdWaitCh chan error
}

func (f *FFmpeg) startAux(stdout io.WriteCloser) error {
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
	for _, m := range f.Maps {
		// skip if reader already created
		if _, ok := inputToFDMap[m.Input]; ok {
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
		}(m.Input)

		inputToFDs = append(inputToFDs, ifd)
		inputToFDMap[m.Input] = ifd
	}

	var extraFiles []*os.File
	args := []string{"-hide_banner"}
	for _, ifd := range inputToFDs {
		extraFiles = append(extraFiles, ifd.r)
		args = append(args, "-i", fmt.Sprintf("pipe:%d", ifd.childFD))
	}

	for _, m := range f.Maps {
		ifd := inputToFDMap[m.Input]

		args = append(args, "-map", fmt.Sprintf("%d:%s", ifd.inputFileID, m.StreamSpecifier))
		if m.Kind == "audio" {
			args = append(args, "-acodec")
		} else if m.Kind == "video" {
			args = append(args, "-vcodec")
		} else {
			panic(fmt.Sprintf("kind can only be audio or video (was %s)", m.Kind))
		}
		args = append(args, m.Codec)
		args = append(args, m.Flags...)
	}

	args = append(args, "-f", f.Format.Name)
	args = append(args, f.Format.Flags...)
	args = append(args, "pipe:1")

	f.cmd = exec.Command("ffmpeg", args...)
	f.cmd.ExtraFiles = extraFiles
	f.cmd.Stderr = f.Stderr
	f.cmd.Stdout = stdout

	log.Printf("cmd %v", f.cmd.Args)

	if err := f.cmd.Start(); err != nil {
		return err
	}

	f.cmdWaitCh = make(chan error, 1)

	go func() {
		f.cmdWaitCh <- f.cmd.Wait()
		f.Stdout.Close()
		close(f.cmdWaitCh)
	}()

	return nil
}

// Start ffmpeg instance
func (f *FFmpeg) Start() error {
	return f.startAux(f.Stdout)
}

// StartWaitForData start ffmpeg instance and return once there is data to be read
func (f *FFmpeg) StartWaitForData() error {
	probeR, probeW := io.Pipe()
	if err := f.startAux(probeW); err != nil {
		probeR.Close()
		return err
	}

	probeByte := make([]byte, 1)
	if _, readErr := io.ReadFull(probeR, probeByte); readErr != nil {
		probeR.Close()
		if cmdErr := f.cmd.Wait(); cmdErr != nil {
			return cmdErr
		}
		return readErr
	}

	go func() {
		f.Stdout.Write(probeByte)
		io.Copy(f.Stdout, probeR)
		probeR.Close()
	}()

	return nil
}

// Wait for ffmpeg to exit
func (f *FFmpeg) Wait() error {
	return <-f.cmdWaitCh
}
