package ydls

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/wader/ydls/internal/timerange"
)

// RequestOptions request options
type RequestOptions struct {
	MediaRawURL string              // youtubedl media URL
	Format      *Format             // output format
	Codecs      []string            // force codecs
	Retranscode bool                // force retranscode even if same input codec
	TimeRange   timerange.TimeRange // time range limit
	Items       uint                // feed item limit
}

// NewRequestOptionsFromQuery /?url=...&format=...
func NewRequestOptionsFromQuery(v url.Values, formats Formats) (RequestOptions, error) {
	mediaRawURL := v.Get("url")
	if mediaRawURL == "" {
		return RequestOptions{}, fmt.Errorf("no url")
	}
	var timeRange timerange.TimeRange
	var timeRangeErr error
	if time := v.Get("time"); time != "" {
		timeRange, timeRangeErr = timerange.NewTimeRangeFromString(v.Get("time"))
		if timeRangeErr != nil {
			return RequestOptions{}, timeRangeErr
		}
	}

	var codecs []string
	var format *Format

	formatName := v.Get("format")
	if formatName != "" {
		qFormat, qFormatFound := formats.FindByName(formatName)
		if !qFormatFound {
			return RequestOptions{}, fmt.Errorf("unknown format \"%s\"", formatName)
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
				return RequestOptions{}, fmt.Errorf("unknown codec \"%s\"", codec)
			}
			codecs = append(codecs, codec)
		}
	}

	items := uint(0)
	itemsStr := v.Get("items")
	if itemsStr != "" {
		itemsN, itemsNErr := strconv.Atoi(itemsStr)
		if itemsNErr != nil {
			return RequestOptions{}, fmt.Errorf("invalid items count")
		}
		items = uint(itemsN)
	}

	return RequestOptions{
		MediaRawURL: mediaRawURL,
		Format:      format,
		Codecs:      codecs,
		Retranscode: v.Get("retranscode") != "",
		TimeRange:   timeRange,
		Items:       items,
	}, nil
}

// NewRequestOptionsFromPath
// /format+opt+opt.../schema://host.domin/path?query
// /format+opt+opt.../host.domain/path?query
// /schema://host.domain/path?query
// /host.domain/path?query
func NewRequestOptionsFromPath(url *url.URL, formats Formats) (RequestOptions, error) {
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
		return RequestOptions{}, fmt.Errorf("no url")
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

	r, dErr := NewRequestOptionsFromOpts(opts, formats)
	if dErr != nil {
		return RequestOptions{}, dErr
	}

	r.MediaRawURL = mediaRawURL

	return r, nil
}

func NewRequestOptionsFromOpts(opts []string, formats Formats) (RequestOptions, error) {
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

	r := RequestOptions{}

	if formatFound {
		r.Format = &format
	}

	for i, opt := range opts {
		const itemsSuffix = "items"

		if i == formatIndex {
			// nop, skip format opt
		} else if opt == "retranscode" {
			r.Retranscode = true
		} else if strings.HasSuffix(opt, itemsSuffix) {
			itemsN, itemsNErr := strconv.Atoi(opt[0 : len(opt)-len(itemsSuffix)])
			if itemsNErr != nil {
				return RequestOptions{}, fmt.Errorf("invalid items count")
			}
			r.Items = uint(itemsN)
			strconv.ParseUint("", 10, 32)
		} else if _, ok := codecNames[opt]; ok {
			r.Codecs = append(r.Codecs, opt)
		} else if tr, trErr := timerange.NewTimeRangeFromString(opt); trErr == nil {
			r.TimeRange = tr
		} else {
			return RequestOptions{}, fmt.Errorf("unknown opt %s", opt)
		}
	}

	return r, nil
}

func (r RequestOptions) QueryValues() url.Values {
	v := url.Values{}
	if r.MediaRawURL != "" {
		v.Set("url", r.MediaRawURL)
	}
	if r.Format != nil {
		v.Set("format", r.Format.Name)
	}
	for _, codec := range r.Codecs {
		v.Add("codec", codec)
	}
	if r.Retranscode {
		v.Set("retranscode", "1")
	}
	if !r.TimeRange.IsZero() {
		v.Set("time", r.TimeRange.String())
	}
	if r.Items > 0 {
		v.Set("items", strconv.Itoa(int(r.Items)))
	}
	return v
}
