package youtubedl

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"
)

// Error youtubedl specific error
type Error string

func (e Error) Error() string {
	return string(e)
}

// Info youtubedl json, thumbnail bytes and raw JSON
type Info struct {
	Artist   string `json:"artist"`
	Uploader string `json:"uploader"`
	Creator  string `json:"creator"`

	Title       string   `json:"title"`
	Description string   `json:"description"`
	Duration    float64  `json:"duration"`
	Thumbnail   string   `json:"thumbnail"`
	Formats     []Format `json:"formats"`

	// not unmarshalled, populated from image thumbnail file
	ThumbnailBytes []byte `json:"-"`

	// private, save raw json to be used later when downloading
	rawJSON []byte
}

// Format youtubedl downloadable format
type Format struct {
	FormatID string  `json:"format_id"`
	Protocol string  `json:"protocol"`
	Ext      string  `json:"ext"`
	ACodec   string  `json:"acodec"`
	VCodec   string  `json:"vcodec"`
	TBR      float64 `json:"tbr"`
	ABR      float64 `json:"abr"`
	VBR      float64 `json:"vbr"`

	NormBR     float64
	NormACodec string
	NormVCodec string
}

func (f Format) String() string {
	return fmt.Sprintf("%s:%s:%s a:%s:%f v:%s:%f %f",
		f.FormatID,
		f.Protocol,
		f.Ext,
		f.NormACodec,
		f.ABR,
		f.NormVCodec,
		f.VBR,
		f.NormBR,
	)
}

// guess codec from fuzzy codec name
func normalizeCodecName(c string) string {
	codecNameNormalizeMap := map[string]string{
		"none": "",
		"avc1": "h264",
		"mp4a": "aac",
		"mp4v": "h264",
		"h265": "hevc",
	}

	// "  NAME.something  " -> "name"
	c = strings.Trim(c, " ")
	c = strings.ToLower(c)
	p := strings.SplitN(c, ".", 2)
	c = p[0]

	if n, ok := codecNameNormalizeMap[c]; ok {
		return n
	}

	return c
}

// guess codecs based on ext
func codecFromExt(ext string) (acodec string, vcodec string) {
	switch strings.ToLower(ext) {
	case "mp3":
		return "mp3", ""
	case "mp4":
		return "aac", "h264"
	case "flv":
		return "aac", "h264"
	default:
		return "", ""
	}
}

func parseInfo(r io.Reader) (info Info, err error) {
	info = Info{}

	info.rawJSON, err = ioutil.ReadAll(r)
	if err != nil {
		return Info{}, err
	}

	if err := json.Unmarshal(info.rawJSON, &info); err != nil {
		return Info{}, err
	}

	for i := range info.Formats {
		f := &info.Formats[i]

		f.NormACodec = normalizeCodecName(f.ACodec)
		f.NormVCodec = normalizeCodecName(f.VCodec)

		extACodec, extVCodec := codecFromExt(f.Ext)
		if f.ACodec == "" {
			f.NormACodec = extACodec
		}
		if f.VCodec == "" {
			f.NormVCodec = extVCodec
		}

		if f.TBR != 0 {
			f.NormBR = f.TBR
		} else {
			f.NormBR = f.ABR + f.VBR
		}
	}

	return info, nil
}

// NewFromURL new Info downloaded from URL using context
func NewFromURL(ctx context.Context, url string, stdout io.Writer) (info Info, err error) {
	tempPath, _ := ioutil.TempDir("", "ydls-youtubedl")
	defer os.RemoveAll(tempPath)

	cmd := exec.CommandContext(
		ctx,
		"youtube-dl",
		"--no-call-home",
		"--no-cache-dir",
		"--skip-download",
		"--write-info-json",
		"--write-thumbnail",
		"--restrict-filenames",
		// don't base output filename on source info
		"--output", "source",
		// provide URL via stdin for security, youtube-dl has some run command args
		"--batch-file", "-",
	)
	cmd.Dir = tempPath
	cmd.Stdout = stdout
	cmdStderr, cmdStderrErr := cmd.StderrPipe()
	if cmdStderrErr != nil {
		return Info{}, cmdStderrErr
	}
	cmdStdin, cmdStdinErr := cmd.StdinPipe()
	if cmdStdinErr != nil {
		return Info{}, cmdStdinErr
	}

	if err := cmd.Start(); err != nil {
		return Info{}, err
	}
	defer cmd.Wait()

	fmt.Fprintln(cmdStdin, url)
	cmdStdin.Close()

	stderrLineScanner := bufio.NewScanner(cmdStderr)
	for stderrLineScanner.Scan() {
		const errorPrefix = "ERROR: "
		line := stderrLineScanner.Text()
		if strings.HasPrefix(line, errorPrefix) {
			return Info{}, Error(line[len(errorPrefix):])
		}
	}

	return NewFromPath(tempPath)
}

// NewFromPath new Info from path with JSON and optional image
func NewFromPath(infoPath string) (info Info, err error) {
	files, err := ioutil.ReadDir(infoPath)
	if err != nil {
		return Info{}, err
	}

	infoJSONPath := ""
	thumbnailPath := ""
	for _, f := range files {
		ext := path.Ext(f.Name())
		if ext == ".json" {
			infoJSONPath = path.Join(infoPath, f.Name())
		} else if ext == ".png" || ext == ".jpg" || ext == ".jpeg" {
			thumbnailPath = path.Join(infoPath, f.Name())
		}
	}

	if infoPath == "" {
		return Info{}, fmt.Errorf("no info json found")
	}

	file, err := os.Open(infoJSONPath)
	if err != nil {
		return Info{}, err
	}
	defer file.Close()
	info, err = parseInfo(file)
	if err != nil {
		return Info{}, err
	}

	if thumbnailPath != "" {
		info.ThumbnailBytes, err = ioutil.ReadFile(thumbnailPath)
		if err != nil {
			return Info{}, fmt.Errorf("failed to read thumbnail")
		}
	}

	return info, nil
}

// DownloadResult download result
type DownloadResult struct {
	Reader io.ReadCloser
	waitCh chan struct{}
}

// Wait for resource cleanup
func (dr *DownloadResult) Wait() {
	<-dr.waitCh
}

// Download format matched by filter
func (info Info) Download(ctx context.Context, filter string, stderr io.Writer) (*DownloadResult, error) {
	tempPath, tempErr := ioutil.TempDir("", "ydls-youtubedl")
	if tempErr != nil {
		return nil, tempErr
	}
	jsonTempPath := path.Join(tempPath, "info.json")
	if err := ioutil.WriteFile(jsonTempPath, info.rawJSON, 0644); err != nil {
		os.RemoveAll(tempPath)
		return nil, err
	}

	dr := &DownloadResult{
		waitCh: make(chan struct{}),
	}

	cmd := exec.CommandContext(
		ctx,
		"youtube-dl",
		"--no-call-home",
		"--no-cache-dir",
		"--restrict-filenames",
		"--load-info", jsonTempPath,
		"-f", filter,
		"-o", "-",
	)
	cmd.Dir = tempPath
	var w io.WriteCloser
	dr.Reader, w = io.Pipe()
	cmd.Stdout = w
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	go func() {
		cmd.Wait()
		w.Close()
		os.RemoveAll(tempPath)
		close(dr.waitCh)
	}()

	return dr, nil
}
