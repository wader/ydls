package ffmpeg

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"time"
)

type Printer interface {
	Printf(format string, v ...interface{})
}

type nopPrinter struct{}

func (nopPrinter) Printf(format string, v ...interface{}) {}

// ProbeInfo ffprobe result
type ProbeInfo struct {
	Format  ProbeFormat            `json:"format"`
	Streams []ProbeStream          `json:"streams"`
	Raw     map[string]interface{} `json:"-"`
}

type ProbeStream struct {
	Index          uint   `json:"index"`
	CodecName      string `json:"codec_name"`
	CodecLongName  string `json:"codec_long_name"`
	CodecType      string `json:"codec_type"`
	CodecTimeBase  string `json:"codec_time_base"`
	CodecTagString string `json:"codec_tag_string"`
	CodecTag       string `json:"codec_tag"`
	SampleFmt      string `json:"sample_fmt"`
	SampleRate     string `json:"sample_rate"`
	Channels       uint   `json:"channels"`
	ChannelLayout  string `json:"channel_layout"`
	BitsPerSample  uint   `json:"bits_per_sample"`
	RFrameRate     string `json:"r_frame_rate"`
	AvgFrameRate   string `json:"avg_frame_rate"`
	TimeBase       string `json:"time_base"`
	StartPts       int64  `json:"start_pts"`
	StartTime      string `json:"start_time"`
	DurationTs     uint64 `json:"duration_ts"`
	Duration       string `json:"duration"`
	BitRate        string `json:"bit_rate"`
}

type ProbeFormat struct {
	Filename       string   `json:"filename"`
	FormatName     string   `json:"format_name"`
	FormatLongName string   `json:"format_long_name"`
	StartTime      string   `json:"start_time"`
	Duration       string   `json:"duration"`
	Size           string   `json:"size"`
	BitRate        string   `json:"bit_rate"`
	ProbeScore     uint     `json:"probe_score"`
	Tags           Metadata `json:"tags"`
}

// from libavformat/avformat.h
// json tag is used for metadata key name also
type Metadata struct {
	Album string `json:"album"` // name of the set this work belongs to
	// main creator of the set/album, if different from artist.
	// e.g. "Various Artists" for compilation albums.
	AlbumArtist  string `json:"album_artist"`
	Artist       string `json:"artist"`        // main creator of the work
	Comment      string `json:"comment"`       // any additional description of the file.
	Composer     string `json:"composer"`      // who composed the work, if different from artist.
	Copyright    string `json:"copyright"`     // name of copyright holder.
	CreationTime string `json:"creation_time"` // date when the file was created, preferably in ISO 8601.
	Date         string `json:"date"`          // date when the work was created, preferably in ISO 8601.
	Disc         string `json:"disc"`          // number of a subset, e.g. disc in a multi-disc collection.
	Encoder      string `json:"encoder"`       // name/settings of the software/hardware that produced the file.
	EncodedBy    string `json:"encoded_by"`    // person/group who created the file.
	Filename     string `json:"filename"`      // original name of the file.
	Genre        string `json:"genre"`         // <self-evident>.
	// main language in which the work is performed, preferably
	// in ISO 639-2 format. Multiple languages can be specified by
	// separating them with commas.
	Language string `json:"language"`
	// artist who performed the work, if different from artist.
	// E.g for "Also sprach Zarathustra", artist would be "Richard
	// Strauss" and performer "London Philharmonic Orchestra".
	Performer       string `json:"performer"`
	Publisher       string `json:"publisher"`        // name of the label/publisher.
	ServiceName     string `json:"service_name"`     // name of the service in broadcasting (channel name).
	ServiceProvider string `json:"service_provider"` // name of the service provider in broadcasting.
	Title           string `json:"title"`            // name of the work.
	Track           string `json:"track"`            // number of this work in the set, can be in form current/total.
	VariantBitrate  string `json:"variant_bitrate"`  // the total bitrate of the bitrate variant that the current stream is part of
}

type Codec interface {
	codecArgs() []string
}

type VideoCodec string

func (c VideoCodec) codecArgs() []string {
	return []string{"-codec:v", string(c)}
}

type AudioCodec string

func (c AudioCodec) codecArgs() []string {
	return []string{"-codec:a", string(c)}
}

type SubtitleCodec string

func (c SubtitleCodec) codecArgs() []string {
	return []string{"-codec:s", string(c)}
}

type Input interface {
	input()
}

type Output interface {
	output()
}

// Reader read from a io.Reader
// is a struct as a interface can't be a receiver
type Reader struct {
	Reader io.Reader
}

func (Reader) input() {}

// Writer write to a io.WriteCloser
type Writer struct {
	Writer io.WriteCloser
}

func (Writer) output() {}

// URL read or write to URL (local file path, pipe:, https:// etc)
type URL string

func (URL) input()  {}
func (URL) output() {}

// Format output format
type Format struct {
	Name  string
	Flags []string
}

// Map input stream to output stream
type Map struct {
	Input      Input  // many streams can use same the input
	Specifier  string // 0, a:0, v:0, etc
	Codec      Codec
	CodecFlags []string
}

type Stream struct {
	InputFlags  []string
	OutputFlags []string
	Maps        []Map
	Format      Format
	Metadata    Metadata
	Output      Output
}

// FFmpeg instance
type FFmpeg struct {
	Streams  []Stream
	Stderr   io.Writer
	DebugLog Printer

	cmd       *exec.Cmd
	cmdWaitCh chan error
	// design borrowed from go src/exec/exec.go
	copyErrCh chan error
	copyFns   []func() error
}

// DurationToPosition time.Duration to ffmpeg position format
func DurationToPosition(d time.Duration) string {
	n := uint64(d.Seconds())

	s := n % 60
	n /= 60
	m := n % 60
	n /= 60
	h := n

	return fmt.Sprintf("%d:%.2d:%.2d", h, m, s)
}

func (pi *ProbeInfo) UnmarshalJSON(text []byte) error {
	type probeInfo ProbeInfo
	var piDummy probeInfo
	err := json.Unmarshal(text, &piDummy)
	json.Unmarshal(text, &piDummy.Raw)
	*pi = ProbeInfo(piDummy)
	return err
}

func (pi ProbeInfo) FindStreamType(codecType string) (ProbeStream, bool) {
	for _, s := range pi.Streams {
		if s.CodecType == codecType {
			return s, true
		}
	}
	return ProbeStream{}, false
}

// VideoCodec probed video codec
func (pi ProbeInfo) VideoCodec() string {
	if s, ok := pi.FindStreamType("video"); ok {
		return s.CodecName
	}
	return ""
}

// AudioCodec probed audio codec
func (pi ProbeInfo) AudioCodec() string {
	if s, ok := pi.FindStreamType("audio"); ok {
		return s.CodecName
	}
	return ""
}

// SubtitleCodec probed audio codec
func (pi ProbeInfo) SubtitleCodec() string {
	if s, ok := pi.FindStreamType("subtitle"); ok {
		return s.CodecName
	}
	return ""
}

// FormatName probed format
func (pi ProbeInfo) FormatName() string {
	if fl := strings.Split(pi.Format.FormatName, ","); len(fl) > 0 {
		return fl[0]
	}
	return ""
}

// Duration probed duration
func (pi ProbeInfo) Duration() time.Duration {
	v, _ := strconv.ParseFloat(pi.Format.Duration, 64)
	return time.Second * time.Duration(v)
}

func (pi ProbeInfo) String() string {
	var codecs []string
	for _, s := range pi.Streams {
		codecs = append(codecs, s.CodecName)
	}

	return fmt.Sprintf("%s:%s", pi.FormatName(), strings.Join(codecs, ":"))
}

// Probe run ffprobe with context
func Probe(ctx context.Context, i Input, debugLog Printer, stderr io.Writer) (pi ProbeInfo, err error) {
	if debugLog == nil {
		debugLog = nopPrinter{}
	}

	ffprobeName := "ffprobe"
	ffprobeArgs := []string{
		"-hide_banner",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
	}
	cmd := exec.CommandContext(ctx, ffprobeName, ffprobeArgs...)
	switch i := i.(type) {
	case Reader:
		cmd.Stdin = i.Reader
		cmd.Args = append(cmd.Args, "pipe:0")
	case URL:
		cmd.Args = append(cmd.Args, string(i))
	default:
		panic(fmt.Sprintf("unknown input type %v", i))
	}
	cmd.Stderr = stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return ProbeInfo{}, err
	}

	debugLog.Printf("cmd %v", cmd.Args)

	if err := cmd.Start(); err != nil {
		return ProbeInfo{}, err
	}

	pi = ProbeInfo{}

	d := json.NewDecoder(stdout)
	jsonErr := d.Decode(&pi)

	waitErr := cmd.Wait()
	if exitErr, ok := waitErr.(*exec.ExitError); ok && !exitErr.Success() {
		return ProbeInfo{}, exitErr
	}

	if jsonErr != nil {
		return ProbeInfo{}, jsonErr
	}

	return pi, nil
}

func (m Metadata) Map() map[string]string {
	kv := map[string]string{}
	t := reflect.TypeOf(m)
	v := reflect.ValueOf(m)

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		value := v.Field(i).String()
		if value == "" {
			continue
		}

		// json tag is used for metadata key name
		key := field.Tag.Get("json")
		kv[key] = value
	}

	return kv
}

func (a Metadata) Merge(b Metadata) Metadata {
	at := reflect.TypeOf(a)
	av := reflect.ValueOf(&a)
	bv := reflect.ValueOf(b)

	for i := 0; i < at.NumField(); i++ {
		af := av.Elem().Field(i)
		if af.String() == "" {
			bf := bv.Field(i)
			af.SetString(bf.String())
		}
	}

	return a
}

func (f *FFmpeg) Start(ctx context.Context) error {
	log := f.DebugLog
	if log == nil {
		log = nopPrinter{}
	}

	f.cmdWaitCh = make(chan error)

	// figure out unique readers and create pipes for io.Readers
	type ffmpegInput struct {
		flags []string
		arg   string // ffmpeg -i argument (pipe:, url)
		index int    // ffmpeg input index
	}
	inputs := []*ffmpegInput{}
	inputsMap := map[Input]*ffmpegInput{}

	type ffmpegOutput struct {
		arg string // ffmpeg output argument (pipe:, url)
	}
	outputs := []*ffmpegOutput{}
	outputsMap := map[Output]*ffmpegOutput{}

	closeAfterStartFns := []func(){}
	closeAfterStart := func() {
		for _, fn := range closeAfterStartFns {
			fn()
		}
	}

	// from os.Cmd "entry i becomes file descriptor 3+i"
	childFD := 3
	var extraFiles []*os.File
	inputFileIndex := 0

	for _, stream := range f.Streams {
		for _, m := range stream.Maps {
			// skip if input already created
			if fi, ok := inputsMap[m.Input]; ok {
				fi.flags = append(fi.flags, stream.InputFlags...)
				continue
			}

			switch i := m.Input.(type) {
			case Reader:
				fi := &ffmpegInput{
					arg:   fmt.Sprintf("pipe:%d", childFD),
					index: inputFileIndex,
				}
				fi.flags = make([]string, len(stream.InputFlags))
				copy(fi.flags, stream.InputFlags)
				childFD++
				inputFileIndex++

				pr, pw, pErr := os.Pipe()
				if pErr != nil {
					return pErr
				}
				extraFiles = append(extraFiles, pr)
				f.copyFns = append(f.copyFns, func() error {
					_, err := io.Copy(pw, i.Reader)
					pw.Close()
					return err
				})
				closeAfterStartFns = append(closeAfterStartFns, func() {
					pr.Close()
				})

				inputs = append(inputs, fi)
				inputsMap[i] = fi
			case URL:
				fi := &ffmpegInput{
					arg:   string(i),
					index: inputFileIndex,
					flags: []string{},
				}
				fi.flags = make([]string, len(stream.InputFlags))
				copy(fi.flags, stream.InputFlags)
				inputFileIndex++

				inputs = append(inputs, fi)
				inputsMap[i] = fi
			default:
				panic(fmt.Sprintf("unknown input type %v", i))
			}
		}

		switch o := stream.Output.(type) {
		case Writer:
			fo := &ffmpegOutput{
				arg: fmt.Sprintf("pipe:%d", childFD),
			}
			childFD++

			pr, pw, pErr := os.Pipe()
			if pErr != nil {
				return pErr
			}
			extraFiles = append(extraFiles, pw)
			f.copyFns = append(f.copyFns, func() error {
				_, err := io.Copy(o.Writer, pr)
				o.Writer.Close()
				pr.Close()
				return err
			})
			closeAfterStartFns = append(closeAfterStartFns, func() {
				pw.Close()
			})

			outputs = append(outputs, fo)
			outputsMap[o] = fo
		case URL:
			fo := &ffmpegOutput{
				arg: string(o),
			}

			outputs = append(outputs, fo)
			outputsMap[o] = fo
		default:
			panic(fmt.Sprintf("unknown output type %v", o))
		}
	}

	ffmpegName := "ffmpeg"
	ffmpegArgs := []string{"-nostdin", "-hide_banner", "-y"}

	for _, fi := range inputs {
		ffmpegArgs = append(ffmpegArgs, fi.flags...)
		ffmpegArgs = append(ffmpegArgs, "-i", fi.arg)
	}

	for _, stream := range f.Streams {
		fo := outputsMap[stream.Output]

		for _, m := range stream.Maps {
			fi := inputsMap[m.Input]
			ffmpegArgs = append(ffmpegArgs, "-map", fmt.Sprintf("%d:%s", fi.index, m.Specifier))
			ffmpegArgs = append(ffmpegArgs, m.Codec.codecArgs()...)
			ffmpegArgs = append(ffmpegArgs, m.CodecFlags...)
		}

		ffmpegArgs = append(ffmpegArgs, "-f", stream.Format.Name)
		ffmpegArgs = append(ffmpegArgs, stream.Format.Flags...)
		for k, v := range stream.Metadata.Map() {
			ffmpegArgs = append(ffmpegArgs, "-metadata", k+"="+v)
		}
		ffmpegArgs = append(ffmpegArgs, stream.OutputFlags...)
		ffmpegArgs = append(ffmpegArgs, fo.arg)
	}

	f.cmd = exec.CommandContext(ctx, ffmpegName, ffmpegArgs...)
	f.cmd.ExtraFiles = extraFiles
	f.cmd.Stderr = f.Stderr

	log.Printf("cmd %v", f.cmd.Args)

	if err := f.cmd.Start(); err != nil {
		closeAfterStart()
		return err
	}

	// after fork we can close read on input and write on output pipes
	closeAfterStart()

	f.copyErrCh = make(chan error, len(f.copyFns))
	for _, fn := range f.copyFns {
		go func(fn func() error) {
			f.copyErrCh <- fn()
		}(fn)
	}

	go func() {
		f.cmdWaitCh <- f.cmd.Wait()
	}()

	return nil
}

// Wait for ffmpeg to finish
func (f *FFmpeg) Wait() error {
	var copyErr error
	for range f.copyFns {
		if err := <-f.copyErrCh; err != nil && copyErr == nil {
			copyErr = err
		}
	}

	cmdErr := <-f.cmdWaitCh
	if cmdErr != nil {
		return cmdErr
	}

	return copyErr
}
