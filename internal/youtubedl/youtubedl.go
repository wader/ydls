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

	"github.com/wader/ydls/internal/writelogger"
)

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

// Info youtubedl info
type Info struct {
	ID         string `json:"id"`
	Type       string `json:"_type"`
	URL        string `json:"url"`
	WebpageURL string `json:"webpage_url"`
	Direct     bool   `json:"direct"`

	Artist        string  `json:"artist"`
	Uploader      string  `json:"uploader"`
	UploadDate    string  `json:"upload_date"`
	Creator       string  `json:"creator"`
	Title         string  `json:"title"`
	PlaylistTitle string  `json:"playlist_title"`
	Episode       string  `json:"episode"`
	Description   string  `json:"description"`
	Duration      float64 `json:"duration"`
	Thumbnail     string  `json:"thumbnail"`
	// not unmarshalled, populated from image thumbnail file
	ThumbnailBytes []byte   `json:"-"`
	Formats        []Format `json:"formats"`

	// Playlist entries if _type is playlist
	Entries []Info `json:"entries"`

	// Info can also be a mix of Info and one Format
	Format
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
}

func (f Format) String() string {
	return fmt.Sprintf("%s:%s:%s a:%s:%f v:%s:%f %f %f",
		f.FormatID,
		f.Protocol,
		f.Ext,
		f.NormalizedACodec(),
		f.ABR,
		f.NormalizedVCodec(),
		f.VBR,
		f.TBR,
		f.NormalizedBR(),
	)
}

func (f Format) NormalizedACodec() string {
	normCodec := normalizeCodecName(f.ACodec)
	if normCodec != "" {
		return normCodec
	}
	normCodec, _ = guessCodecFromExt(f.Ext)
	return normCodec
}

func (f Format) NormalizedVCodec() string {
	normCodec := normalizeCodecName(f.VCodec)
	if normCodec != "" {
		return normCodec
	}
	_, normCodec = guessCodecFromExt(f.Ext)
	return normCodec
}

func (f Format) NormalizedBR() float64 {
	if f.TBR != 0 {
		return f.TBR
	}
	return f.ABR + f.VBR
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
func guessCodecFromExt(ext string) (acodec string, vcodec string) {
	switch strings.ToLower(ext) {
	case "wav":
		return "wav", ""
	case "mp3":
		return "mp3", ""
	case "ogg":
		return "vorbis", ""
	case "m4a",
		"aac":
		return "aac", ""
	case "mp4",
		"m4v",
		"mov",
		"3gp":
		return "aac", "h264"
	case "webm":
		return "opus", "vp9"
	case "flv":
		return "aac", "h264"
	}
	return "", ""
}

type Options struct {
	YesPlaylist    bool // prefer playlist
	PlaylistStart  uint
	PlaylistEnd    uint
	SkipThumbnails bool
	DebugLog       Printer
	HTTPClient     *http.Client
}

// New downloads metadata for URL
func New(ctx context.Context, rawURL string, options Options) (result Result, err error) {
	if options.DebugLog == nil {
		options.DebugLog = nopPrinter{}
	}
	if options.HTTPClient == nil {
		options.HTTPClient = http.DefaultClient
	}

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

func infoFromURL(ctx context.Context, rawURL string, options Options) (info Info, rawJSON []byte, err error) {
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

	ydlStderr := writelogger.New(options.DebugLog, "ydl-info stderr> ")
	stderrWriter := io.MultiWriter(stderrBuf, ydlStderr)

	cmd.Dir = tempPath
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrWriter
	cmd.Stdin = bytes.NewBufferString(rawURL + "\n")

	options.DebugLog.Printf("cmd %v", cmd.Args)
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
		resp, respErr := options.HTTPClient.Get(info.Thumbnail)
		if respErr == nil {
			thumbnailBuf, _ := ioutil.ReadAll(resp.Body)
			resp.Body.Close()
			info.ThumbnailBytes = thumbnailBuf
		}
	}

	return info, stdoutBuf.Bytes(), nil
}

// Result metadata for a URL
type Result struct {
	Info    Info
	RawJSON []byte  // saved raw JSON. Used later when downloading
	Options Options // options passed to New
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

// Formats return all formats
// helper to take care of mixed info and format
func (result Result) Formats() []Format {
	if len(result.Info.Formats) > 0 {
		return result.Info.Formats
	}
	return []Format{result.Info.Format}
}

// Download format matched by filter
func (result Result) Download(ctx context.Context, filter string) (*DownloadResult, error) {
	debugLog := result.Options.DebugLog

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
		"-o", "-",
	)
	// don't need to specify if direct as there is only one
	// also seems to be issues when using filter with generic extractor
	if !result.Info.Direct {
		cmd.Args = append(cmd.Args, "-f", filter)
	}

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
