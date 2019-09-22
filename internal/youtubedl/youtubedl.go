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
	Print(v ...interface{})
	Printf(format string, v ...interface{})
}

type nopPrinter struct{}

func (nopPrinter) Print(v ...interface{})                 {}
func (nopPrinter) Printf(format string, v ...interface{}) {}

// Error youtubedl specific error
type Error string

func (e Error) Error() string {
	return string(e)
}

// Info youtubedl info
type Info struct {
	// Generated from youtube-dl README using:
	// sed -e 's/ - `\(.*\)` (\(.*\)): \(.*\)/\1 \2 `json:"\1"` \/\/ \3/' | sed -e 's/numeric/float64/' | sed -e 's/boolean/bool/' | sed -e 's/_id/ID/'  | sed -e 's/_count/Count/'| sed -e 's/_uploader/Uploader/' | sed -e 's/_key/Key/' | sed -e 's/_year/Year/' | sed -e 's/_title/Title/' | sed -e 's/_rating/Rating/'  | sed -e 's/_number/Number/'  | awk '{print toupper(substr($0, 0, 1))  substr($0, 2)}'
	ID                 string  `json:"id"`                   // Video identifier
	Title              string  `json:"title"`                // Video title
	URL                string  `json:"url"`                  // Video URL
	AltTitle           string  `json:"alt_title"`            // A secondary title of the video
	DisplayID          string  `json:"display_id"`           // An alternative identifier for the video
	Uploader           string  `json:"uploader"`             // Full name of the video uploader
	License            string  `json:"license"`              // License name the video is licensed under
	Creator            string  `json:"creator"`              // The creator of the video
	ReleaseDate        string  `json:"release_date"`         // The date (YYYYMMDD) when the video was released
	Timestamp          float64 `json:"timestamp"`            // UNIX timestamp of the moment the video became available
	UploadDate         string  `json:"upload_date"`          // Video upload date (YYYYMMDD)
	UploaderID         string  `json:"uploader_id"`          // Nickname or id of the video uploader
	Channel            string  `json:"channel"`              // Full name of the channel the video is uploaded on
	ChannelID          string  `json:"channel_id"`           // Id of the channel
	Location           string  `json:"location"`             // Physical location where the video was filmed
	Duration           float64 `json:"duration"`             // Length of the video in seconds
	ViewCount          float64 `json:"view_count"`           // How many users have watched the video on the platform
	LikeCount          float64 `json:"like_count"`           // Number of positive ratings of the video
	DislikeCount       float64 `json:"dislike_count"`        // Number of negative ratings of the video
	RepostCount        float64 `json:"repost_count"`         // Number of reposts of the video
	AverageRating      float64 `json:"average_rating"`       // Average rating give by users, the scale used depends on the webpage
	CommentCount       float64 `json:"comment_count"`        // Number of comments on the video
	AgeLimit           float64 `json:"age_limit"`            // Age restriction for the video (years)
	IsLive             bool    `json:"is_live"`              // Whether this video is a live stream or a fixed-length video
	StartTime          float64 `json:"start_time"`           // Time in seconds where the reproduction should start, as specified in the URL
	EndTime            float64 `json:"end_time"`             // Time in seconds where the reproduction should end, as specified in the URL
	Extractor          string  `json:"extractor"`            // Name of the extractor
	ExtractorKey       string  `json:"extractor_key"`        // Key name of the extractor
	Epoch              float64 `json:"epoch"`                // Unix epoch when creating the file
	Autonumber         float64 `json:"autonumber"`           // Five-digit number that will be increased with each download, starting at zero
	Playlist           string  `json:"playlist"`             // Name or id of the playlist that contains the video
	PlaylistIndex      float64 `json:"playlist_index"`       // Index of the video in the playlist padded with leading zeros according to the total length of the playlist
	PlaylistID         string  `json:"playlist_id"`          // Playlist identifier
	PlaylistTitle      string  `json:"playlist_title"`       // Playlist title
	PlaylistUploader   string  `json:"playlist_uploader"`    // Full name of the playlist uploader
	PlaylistUploaderID string  `json:"playlist_uploader_id"` // Nickname or id of the playlist uploader

	// Available for the video that belongs to some logical chapter or section:
	Chapter       string  `json:"chapter"`        // Name or title of the chapter the video belongs to
	ChapterNumber float64 `json:"chapter_number"` // Number of the chapter the video belongs to
	ChapterID     string  `json:"chapter_id"`     // Id of the chapter the video belongs to

	// Available for the video that is an episode of some series or programme:
	Series        string  `json:"series"`         // Title of the series or programme the video episode belongs to
	Season        string  `json:"season"`         // Title of the season the video episode belongs to
	SeasonNumber  float64 `json:"season_number"`  // Number of the season the video episode belongs to
	SeasonID      string  `json:"season_id"`      // Id of the season the video episode belongs to
	Episode       string  `json:"episode"`        // Title of the video episode
	EpisodeNumber float64 `json:"episode_number"` // Number of the video episode within a season
	EpisodeID     string  `json:"episode_id"`     // Id of the video episode

	// Available for the media that is a track or a part of a music album:
	Track       string  `json:"track"`        // Title of the track
	TrackNumber float64 `json:"track_number"` // Number of the track within an album or a disc
	TrackID     string  `json:"track_id"`     // Id of the track
	Artist      string  `json:"artist"`       // Artist(s) of the track
	Genre       string  `json:"genre"`        // Genre(s) of the track
	Album       string  `json:"album"`        // Title of the album the track belongs to
	AlbumType   string  `json:"album_type"`   // Type of the album
	AlbumArtist string  `json:"album_artist"` // List of all artists appeared on the album
	DiscNumber  float64 `json:"disc_number"`  // Number of the disc or other physical medium the track belongs to
	ReleaseYear float64 `json:"release_year"` // Year (YYYY) when the album was released

	Type        string `json:"_type"`
	Direct      bool   `json:"direct"`
	WebpageURL  string `json:"webpage_url"`
	Description string `json:"description"`
	Thumbnail   string `json:"thumbnail"`
	// not unmarshalled, populated from image thumbnail file
	ThumbnailBytes []byte `json:"-"`

	Formats   []Format              `json:"formats"`
	Subtitles map[string][]Subtitle `json:"subtitles"`

	// Playlist entries if _type is playlist
	Entries []Info `json:"entries"`

	// Info can also be a mix of Info and one Format
	Format
}

// Format youtubedl downloadable format
type Format struct {
	Ext            string  `json:"ext"`             // Video filename extension
	Format         string  `json:"format"`          // A human-readable description of the format
	FormatID       string  `json:"format_id"`       // Format code specified by `--format`
	FormatNote     string  `json:"format_note"`     // Additional info about the format
	Width          float64 `json:"width"`           // Width of the video
	Height         float64 `json:"height"`          // Height of the video
	Resolution     string  `json:"resolution"`      // Textual description of width and height
	TBR            float64 `json:"tbr"`             // Average bitrate of audio and video in KBit/s
	ABR            float64 `json:"abr"`             // Average audio bitrate in KBit/s
	ACodec         string  `json:"acodec"`          // Name of the audio codec in use
	ASR            float64 `json:"asr"`             // Audio sampling rate in Hertz
	VBR            float64 `json:"vbr"`             // Average video bitrate in KBit/s
	FPS            float64 `json:"fps"`             // Frame rate
	VCodec         string  `json:"vcodec"`          // Name of the video codec in use
	Container      string  `json:"container"`       // Name of the container format
	Filesize       float64 `json:"filesize"`        // The number of bytes, if known in advance
	FilesizeApprox float64 `json:"filesize_approx"` // An estimate for the number of bytes
	Protocol       string  `json:"protocol"`        // The protocol that will be used for the actual download
}

type Subtitle struct {
	URL      string `json:"url"`
	Ext      string `json:"ext"`
	Language string `json:"-"`
	// not unmarshalled, populated from subtitle file
	Bytes []byte `json:"-"`
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
	normCodec, normCodecFound := normalizeCodecName(f.ACodec)
	if normCodecFound {
		return normCodec
	}
	normCodec, _ = guessCodecFromExt(f.Ext)
	return normCodec
}

func (f Format) NormalizedVCodec() string {
	normCodec, normCodecFound := normalizeCodecName(f.VCodec)
	if normCodecFound {
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
func normalizeCodecName(c string) (string, bool) {
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
		return n, true
	}

	return c, false
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
	YesPlaylist       bool // prefer playlist
	PlaylistStart     uint
	PlaylistEnd       uint
	DownloadThumbnail bool
	DownloadSubtitles bool
	DebugLog          Printer
	HTTPClient        *http.Client
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
		"--ignore-errors",
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
		if options.DownloadSubtitles {
			cmd.Args = append(cmd.Args,
				"--all-subs",
			)
		}
		cmd.Args = append(cmd.Args,
			"--no-playlist",
		)
	}

	tempPath, _ := ioutil.TempDir("", "ydls-youtubedl")
	defer os.RemoveAll(tempPath)

	stdoutBuf := &bytes.Buffer{}
	stderrBuf := &bytes.Buffer{}

	stderrWL := writelogger.New(options.DebugLog, "ydl-info stderr> ")
	stderrWriter := io.MultiWriter(stderrBuf, stderrWL)

	cmd.Dir = tempPath
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrWriter
	cmd.Stdin = bytes.NewBufferString(rawURL + "\n")

	options.DebugLog.Printf("cmd %v", cmd.Args)
	cmdErr := cmd.Run()
	stderrWL.Close()

	stderrLineScanner := bufio.NewScanner(stderrBuf)
	errMessage := ""
	for stderrLineScanner.Scan() {
		const errorPrefix = "ERROR: "
		line := stderrLineScanner.Text()
		if strings.HasPrefix(line, errorPrefix) {
			errMessage = line[len(errorPrefix):]
		}
	}

	// HACK: --ignore-errors still return error message and exit code != 0
	// so workaround is to assume things went ok if we get some json on stdout
	if len(stdoutBuf.Bytes()) == 0 {
		if errMessage != "" {
			return Info{}, nil, Error(errMessage)
		} else if cmdErr != nil {
			return Info{}, nil, cmdErr
		}
	}

	if infoErr := json.Unmarshal(stdoutBuf.Bytes(), &info); infoErr != nil {
		return Info{}, nil, infoErr
	}

	if options.YesPlaylist && (info.Type != "playlist" || info.Type == "multi_video") {
		return Info{}, nil, fmt.Errorf("not a playlist")
	}

	if options.DownloadThumbnail && info.Thumbnail != "" {
		resp, respErr := options.HTTPClient.Get(info.Thumbnail)
		if respErr == nil {
			buf, _ := ioutil.ReadAll(resp.Body)
			resp.Body.Close()
			info.ThumbnailBytes = buf
		}
	}

	for language, subtitles := range info.Subtitles {
		for i := range subtitles {
			subtitles[i].Language = language
		}
	}

	if options.DownloadSubtitles {
		for _, subtitles := range info.Subtitles {
			for i, subtitle := range subtitles {
				resp, respErr := options.HTTPClient.Get(subtitle.URL)
				if respErr == nil {
					buf, _ := ioutil.ReadAll(resp.Body)
					resp.Body.Close()
					subtitles[i].Bytes = buf
				}
			}
		}
	}

	// as we ignore errors some entries might show up as null
	if options.YesPlaylist {
		var filteredEntrise []Info
		for _, e := range info.Entries {
			if e.ID == "" {
				continue
			}
			filteredEntrise = append(filteredEntrise, e)
		}
		info.Entries = filteredEntrise
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
	if err := ioutil.WriteFile(jsonTempPath, result.RawJSON, 0600); err != nil {
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
		"--ignore-errors",
		"--newline",
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
	stderrWL := writelogger.New(debugLog, fmt.Sprintf("ydl-dl %s stderr> ", filter))
	cmd.Stdout = w
	cmd.Stderr = stderrWL

	debugLog.Printf("cmd %v", cmd.Args)
	if err := cmd.Start(); err != nil {
		os.RemoveAll(tempPath)
		return nil, err
	}

	go func() {
		cmd.Wait()
		stderrWL.Close()
		w.Close()
		os.RemoveAll(tempPath)
		close(dr.waitCh)
	}()

	return dr, nil
}
