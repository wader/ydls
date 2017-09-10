package ydls

import (
	"encoding/json"
	"fmt"
	"io"
)

// Format media container format, possible codecs, extension and mime
type Format struct {
	Name        string
	Formats     prioStringSet
	FormatFlags []string
	ACodecs     prioFormatCodecSet
	VCodecs     prioFormatCodecSet
	Ext         string
	Prepend     string
	MIMEType    string
}

// FormatCodec codec name and ffmpeg args
type FormatCodec struct {
	Codec       string
	CodecFlags  []string
	FormatFlags []string
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

func parseFormats(r io.Reader) (*Formats, error) {
	f := &Formats{}

	d := json.NewDecoder(r)
	if err := d.Decode(f); err != nil {
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
			((f.ACodecs.empty() && acodec == "") || acodec == "*" || f.ACodecs.hasCodec(acodec)) &&
			((f.VCodecs.empty() && vcodec == "") || vcodec == "*" || f.VCodecs.hasCodec(vcodec)) {
			return &f
		}
	}

	return nil
}
