package ydls

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/wader/ydls/internal/timerange"
)

// DownloadOptions download options
type DownloadOptions struct {
	MediaRawURL string
	Format      *Format
	Codecs      []string            // force codecs
	Retranscode bool                // force retranscode even if same input codec
	TimeRange   timerange.TimeRange // time range limit
	Items       uint                // feed item limit
	BaseURL     *url.URL            // base URL to use in rss feed etc
}

// NewDownloadOptionsFromQuery /?url=...&format=...
func NewDownloadOptionsFromQuery(v url.Values, formats Formats) (DownloadOptions, error) {
	mediaRawURL := v.Get("url")
	if mediaRawURL == "" {
		return DownloadOptions{}, fmt.Errorf("no url")
	}
	var timeRange timerange.TimeRange
	var timeRangeErr error
	if time := v.Get("time"); time != "" {
		timeRange, timeRangeErr = timerange.NewTimeRangeFromString(v.Get("time"))
		if timeRangeErr != nil {
			return DownloadOptions{}, timeRangeErr
		}
	}

	var codecs []string
	var format *Format

	formatName := v.Get("format")
	if formatName != "" {
		qFormat, qFormatFound := formats.FindByName(formatName)
		if !qFormatFound {
			return DownloadOptions{}, fmt.Errorf("unknown format \"%s\"", formatName)
		}
		format = &qFormat

		codecNames := map[string]bool{}
		for _, s := range format.Streams {
			for _, c := range s.Codecs {
				codecNames[c.Name] = true
			}
		}

		for _, codec := range v["codec"] {
			if _, ok := codecNames[codec]; !ok {
				return DownloadOptions{}, fmt.Errorf("unknown codec \"%s\"", codec)
			}
			codecs = append(codecs, codec)
		}
	}

	items := uint(0)
	itemsStr := v.Get("items")
	if itemsStr != "" {
		itemsN, itemsNErr := strconv.Atoi(itemsStr)
		if itemsNErr != nil {
			return DownloadOptions{}, fmt.Errorf("invalid items count")
		}
		items = uint(itemsN)
	}

	return DownloadOptions{
		MediaRawURL: mediaRawURL,
		Format:      format,
		Codecs:      codecs,
		Retranscode: v.Get("retranscode") != "",
		TimeRange:   timeRange,
		Items:       items,
	}, nil
}

// NewDownloadOptionsFromPath
// /format+opt+opt.../schema://host.domin/path?query
// /format+opt+opt.../host.domain/path?query
// /schema://host.domain/path?query
// /host.domain/path?query
func NewDownloadOptionsFromPath(url *url.URL, formats Formats) (DownloadOptions, error) {
	formatAndOpts := ""
	mediaRawURL := ""

	// /format+opt/url -> ["/", "format", "url"]
	// parts[0] always empty, path always starts with /
	parts := strings.SplitN(url.Path, "/", 3)
	parts = parts[1:]

	// format? part does not contains ":" or "."
	if !strings.Contains(parts[0], ":") && !strings.Contains(parts[0], ".") {
		formatAndOpts = parts[0]
		parts = parts[1:]
	}

	if len(parts) == 0 {
		return DownloadOptions{}, fmt.Errorf("no url")
	}

	if len(parts) == 2 {
		// had schema:// but split has removed one /
		mediaRawURL = parts[0] + "/" + parts[1]
	} else {
		mediaRawURL = parts[0]
	}
	if url.RawQuery != "" {
		mediaRawURL += "?" + url.RawQuery
	}

	opts := []string{}
	if formatAndOpts != "" {
		opts = strings.Split(formatAndOpts, "+")
	}

	d, dErr := NewDownloadOptionsFromOpts(opts, formats)
	if dErr != nil {
		return DownloadOptions{}, dErr
	}

	d.MediaRawURL = mediaRawURL

	return d, nil
}

func NewDownloadOptionsFromOpts(opts []string, formats Formats) (DownloadOptions, error) {
	var format Format
	var formatFound bool
	formatIndex := -1
	for i, opt := range opts {
		format, formatFound = formats.FindByName(opt)
		if formatFound {
			formatIndex = i
			break
		}
	}

	codecNames := map[string]bool{}
	if formatFound {
		for _, s := range format.Streams {
			for _, c := range s.Codecs {
				codecNames[c.Name] = true
			}
		}
	}

	d := DownloadOptions{}

	if formatFound {
		d.Format = &format
	}

	for i, opt := range opts {
		const itemsSuffix = "items"

		if i == formatIndex {
			// nop, skip format opt
		} else if opt == "retranscode" {
			d.Retranscode = true
		} else if strings.HasSuffix(opt, itemsSuffix) {
			itemsN, itemsNErr := strconv.Atoi(opt[0 : len(opt)-len(itemsSuffix)])
			if itemsNErr != nil {
				return DownloadOptions{}, fmt.Errorf("invalid items count")
			}
			d.Items = uint(itemsN)
			strconv.ParseUint("", 10, 32)
		} else if _, ok := codecNames[opt]; ok {
			d.Codecs = append(d.Codecs, opt)
		} else if tr, trErr := timerange.NewTimeRangeFromString(opt); trErr == nil {
			d.TimeRange = tr
		} else {
			return DownloadOptions{}, fmt.Errorf("unknown opt %s", opt)
		}
	}

	return d, nil
}

func (d DownloadOptions) QueryValues() url.Values {
	v := url.Values{}
	if d.MediaRawURL != "" {
		v.Set("url", d.MediaRawURL)
	}
	if d.Format != nil {
		v.Set("format", d.Format.Name)
	}
	for _, codec := range d.Codecs {
		v.Add("codec", codec)
	}
	if d.Retranscode {
		v.Set("retranscode", "1")
	}
	if !d.TimeRange.IsZero() {
		v.Set("time", d.TimeRange.String())
	}
	if d.Items > 0 {
		v.Set("items", strconv.Itoa(int(d.Items)))
	}
	return v
}

func (d DownloadOptions) URL() *url.URL {
	u := &url.URL{
		RawQuery: d.QueryValues().Encode(),
	}

	if d.BaseURL == nil {
		return u
	}

	return d.BaseURL.ResolveReference(u)
}
