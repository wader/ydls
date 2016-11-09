package ydls

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type prioStringSet []string

func (p *prioStringSet) member(a string) bool {
	for _, c := range *p {
		if c == a {
			return true
		}
	}
	return false
}

func (p *prioStringSet) empty() bool {
	return len(*p) == 0
}

func (p *prioStringSet) first() string {
	if len(*p) > 0 {
		return (*p)[0]
	}
	return ""
}

func (p *prioStringSet) String() string {
	return "[" + strings.Join(*p, " ") + "]"
}

func (p *prioStringSet) UnmarshalJSON(b []byte) (err error) {
	var np []string
	err = json.Unmarshal(b, &np)
	*p = np
	return
}

type prioFormatCodecSet []FormatCodec

func (p *prioFormatCodecSet) findByCodecName(codec string) *FormatCodec {
	for _, fc := range *p {
		if fc.Codec == codec {
			return &fc
		}
	}
	return nil
}

func (p *prioFormatCodecSet) empty() bool {
	return len(*p) == 0
}

func (p *prioFormatCodecSet) first() *FormatCodec {
	if len(*p) > 0 {
		return &(*p)[0]
	}
	return nil
}

func (p *prioFormatCodecSet) CodecNames() []string {
	var codecs []string
	for _, c := range *p {
		codecs = append(codecs, c.Codec)
	}
	return codecs
}

func (p *prioFormatCodecSet) PrioStringSet() *prioStringSet {
	ps := prioStringSet(p.CodecNames())
	return &ps
}

func (p *prioFormatCodecSet) String() string {
	return "[" + strings.Join(p.CodecNames(), " ") + "]"
}

func (p *prioFormatCodecSet) UnmarshalJSON(b []byte) (err error) {
	var formatCodecs []FormatCodec
	err = json.Unmarshal(b, &formatCodecs)
	*p = formatCodecs
	return
}

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
			((f.ACodecs.empty() && acodec == "") || acodec == "*" || f.ACodecs.findByCodecName(acodec) != nil) &&
			((f.VCodecs.empty() && vcodec == "") || vcodec == "*" || f.VCodecs.findByCodecName(vcodec) != nil) {
			return &f
		}
	}

	return nil
}
