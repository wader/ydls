package youtubedl

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/wader/ydls/internal/writelogger"
)

var httpClient = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		// TODO: reuse messes up leaktests
		DisableKeepAlives: true,
	},
}

type Printer interface {
	Printf(format string, v ...interface{})
}

type nopPrinter struct{}

func (nopPrinter) Printf(format string, v ...interface{}) {}

// Error youtubedl specific error
type Error string

func (e Error) Error() string {
	return string(e)
}

// Info youtubedl json, thumbnail bytes and raw JSON
type Info struct {
	ID         string `json:"id"`
	Type       string `json:"_type"`
	URL        string `json:"url"`
	WebpageURL string `json:"webpage_url"`

	Artist      string   `json:"artist"`
	Uploader    string   `json:"uploader"`
	UploadDate  string   `json:"upload_date"`
	Creator     string   `json:"creator"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Duration    float64  `json:"duration"`
	Thumbnail   string   `json:"thumbnail"`
	Formats     []Format `json:"formats"`

	// not unmarshalled, populated from image thumbnail file
	ThumbnailBytes []byte `json:"-"`

	// Playlist entries if _type is playlist
	Entries []Info `json:"entries"`
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
	return fmt.Sprintf("%s:%s:%s a:%s:%f v:%s:%f %f %f",
		f.FormatID,
		f.Protocol,
		f.Ext,
		f.NormACodec,
		f.ABR,
		f.NormVCodec,
		f.VBR,
		f.TBR,
		f.NormBR,
	)
}

func (f *Format) UnmarshalJSON(b []byte) (err error) {
	type FormatRaw Format
	var fr FormatRaw
	if err := json.Unmarshal(b, &fr); err != nil {
		return err
	}
	*f = Format(fr)

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

	return nil
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

type Options struct {
	YesPlaylist    bool // prefer playlist
	PlaylistStart  uint
	PlaylistEnd    uint
	SkipThumbnails bool
	DebugLog       Printer
}

// NewFromURL new Info downloaded from URL using context
func NewFromURL(ctx context.Context, rawURL string, options Options) (result Result, err error) {
	info, rawJSON, err := infoFromURL(ctx, rawURL, options)
	if err != nil {
		return Result{}, err
	}

	rawJSONCopy := make([]byte, len(rawJSON))
	copy(rawJSONCopy, rawJSON)

	return Result{
		Info:    info,
		RawJSON: rawJSONCopy,
		Options: options,
	}, nil
}

// NewFromURL new Info downloaded from URL using context
func infoFromURL(ctx context.Context, rawURL string, options Options) (info Info, rawJSON []byte, err error) {
	debugLog := options.DebugLog
	if debugLog == nil {
		debugLog = nopPrinter{}
	}

	cmd := exec.CommandContext(
		ctx,
		"youtube-dl",
		"--no-call-home",
		"--no-cache-dir",
		"--skip-download",
		"--restrict-filenames",
		// provide URL via stdin for security, youtube-dl has some run command args
		"--batch-file", "-",
		"-J",
	)
	if options.YesPlaylist {
		cmd.Args = append(cmd.Args, "--yes-playlist")

		if options.PlaylistStart > 0 {
			cmd.Args = append(cmd.Args,
				"--playlist-start", strconv.Itoa(int(options.PlaylistStart)),
			)
		}
		if options.PlaylistEnd > 0 {
			cmd.Args = append(cmd.Args,
				"--playlist-end", strconv.Itoa(int(options.PlaylistEnd)),
			)
		}
	} else {
		cmd.Args = append(cmd.Args, "--no-playlist")
	}

	tempPath, _ := ioutil.TempDir("", "ydls-youtubedl")
	defer os.RemoveAll(tempPath)

	stdoutBuf := &bytes.Buffer{}
	stderrBuf := &bytes.Buffer{}

	ydlStderr := writelogger.New(debugLog, "ydl-info stderr> ")
	stderrWriter := io.MultiWriter(stderrBuf, ydlStderr)

	cmd.Dir = tempPath
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrWriter
	cmd.Stdin = bytes.NewBufferString(rawURL + "\n")

	debugLog.Printf("cmd %v", cmd.Args)
	cmdErr := cmd.Run()

	stderrLineScanner := bufio.NewScanner(stderrBuf)
	errMessage := ""
	for stderrLineScanner.Scan() {
		const errorPrefix = "ERROR: "
		line := stderrLineScanner.Text()
		if strings.HasPrefix(line, errorPrefix) {
			errMessage = line[len(errorPrefix):]
		}
	}

	if errMessage != "" {
		return Info{}, nil, Error(errMessage)
	} else if cmdErr != nil {
		return Info{}, nil, cmdErr
	}

	if infoErr := json.Unmarshal(stdoutBuf.Bytes(), &info); infoErr != nil {
		return Info{}, nil, infoErr
	}

	if options.YesPlaylist && (info.Type != "playlist" || info.Type == "mutli_video") {
		return Info{}, nil, fmt.Errorf("not a playlist")
	}

	if !options.SkipThumbnails && info.Thumbnail != "" {
		resp, respErr := httpClient.Get(info.Thumbnail)
		if respErr == nil {
			thumbnailBuf, _ := ioutil.ReadAll(resp.Body)
			resp.Body.Close()
			info.ThumbnailBytes = thumbnailBuf
		}
	}

	return info, stdoutBuf.Bytes(), nil
}

type Result struct {
	Info    Info
	RawJSON []byte  // saved raw JSON. Used later when downloading
	Options Options // options to NewFromURL
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
func (result Result) Download(ctx context.Context, filter string) (*DownloadResult, error) {
	debugLog := result.Options.DebugLog
	if debugLog == nil {
		debugLog = nopPrinter{}
	}

	if result.Info.Type == "playlist" || result.Info.Type == "multi_video" {
		return nil, fmt.Errorf("is a playlist")
	}

	tempPath, tempErr := ioutil.TempDir("", "ydls-youtubedl")
	if tempErr != nil {
		return nil, tempErr
	}
	jsonTempPath := path.Join(tempPath, "info.json")
	if err := ioutil.WriteFile(jsonTempPath, result.RawJSON, 0644); err != nil {
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
	cmd.Stderr = writelogger.New(debugLog, "ydl-dl stderr> ")

	debugLog.Printf("cmd %v", cmd.Args)
	if err := cmd.Start(); err != nil {
		os.RemoveAll(tempPath)
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
