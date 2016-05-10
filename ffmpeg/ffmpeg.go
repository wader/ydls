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

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	if pi, err = probeInfoParse(stdout); err != nil {
		return nil, err
	}

	if err := cmd.Wait(); err != nil {
		return nil, err
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

// Args ffmpeg arguments
type Args struct {
	Maps     []Map
	Format   Format
	Stderr   io.Writer
	DebugLog *log.Logger
}

// FFmpeg run ffmpeg
func FFmpeg(a *Args) (io.ReadCloser, error) {
	log := log.New(ioutil.Discard, "", 0)
	if a.DebugLog != nil {
		log = a.DebugLog
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
	for _, m := range a.Maps {
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
			return nil, err
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

	for _, m := range a.Maps {
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

	args = append(args, "-f", a.Format.Name)
	args = append(args, a.Format.Flags...)

	args = append(args, "pipe:1")

	cmd := exec.Command("ffmpeg", args...)
	cmd.ExtraFiles = extraFiles
	cmd.Stderr = a.Stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	log.Printf("cmd %v", cmd.Args)

	go func() {
		if err := cmd.Start(); err != nil {
			log.Printf("Start err=%v", err)
			return
		}
		if err := cmd.Wait(); err != nil {
			log.Printf("Wait err=%v", err)
			return
		}
	}()

	return stdout, nil
}
