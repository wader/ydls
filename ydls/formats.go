package ydls

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type prioStringSet []string

func (ss *prioStringSet) member(a string) bool {
	for _, s := range *ss {
		if s == a {
			return true
		}
	}
	return false
}

func (ss *prioStringSet) empty() bool {
	return len(*ss) == 0
}

func (ss *prioStringSet) first() string {
	if len(*ss) > 0 {
		return (*ss)[0]
	}
	return ""
}

func (ss *prioStringSet) String() string {
	return "[" + strings.Join(*ss, " ") + "]"
}

func (ss *prioStringSet) UnmarshalJSON(b []byte) (err error) {
	var a []string
	err = json.Unmarshal(b, &a)
	*ss = a
	return
}

// Format media container format, possible codecs, ffmpeg args, extension and mime
type Format struct {
	Name        string
	Formats     prioStringSet
	FormatFlags []string
	ACodecs     prioStringSet
	ACodecFlags []string
	VCodecs     prioStringSet
	VCodecFlags []string
	Prepend     string
	Ext         string
	MIMEType    string
}

func (f *Format) String() string {
	return fmt.Sprintf("%s:%s:%s:%s:%s:%s:%s",
		f.Name,
		f.Formats,
		f.ACodecs,
		f.VCodecs,
		f.Prepend,
		f.Ext,
		f.MIMEType,
	)
}

// Formats ordered list of Formats
type Formats []Format

func parseFormats(r io.Reader) (Formats, error) {
	f := Formats{}

	d := json.NewDecoder(r)
	if err := d.Decode(&f); err != nil {
		return nil, err
	}

	return f, nil
}

// FindByName find format by name
func (fs Formats) FindByName(name string) *Format {
	for _, f := range fs {
		if f.Name == name {
			return &f
		}
	}

	return nil
}

// Find find format by format and codecs (* for wildcard)
func (fs Formats) Find(format string, acodec string, vcodec string) *Format {
	for _, f := range fs {
		if (format == "*" || f.Formats.member(format)) &&
			((f.ACodecs.empty() && acodec == "") || acodec == "*" || f.ACodecs.member(acodec)) &&
			((f.VCodecs.empty() && vcodec == "") || vcodec == "*" || f.VCodecs.member(vcodec)) {
			return &f
		}
	}

	return nil
}
