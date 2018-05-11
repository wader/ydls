package ydls

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/wader/ydls/internal/stringprioset"
)

// YDLS config
type Config struct {
	InputFlags []string
	CodecMap   map[string]string
	Formats    Formats
}

// Format media container format, possible codecs, extension and mime
type Format struct {
	Name        string
	Formats     stringprioset.Set
	FormatFlags []string
	Streams     []Stream
	Ext         string
	Prepend     string
	MIMEType    string

	// used by rss feeds etc
	EnclosureFormat         string
	EnclosureFormatOptions  []string
	EnclosureRequestOptions RequestOptions `json:"-"`
}

func (f *Format) UnmarshalJSON(b []byte) (err error) {
	type FormatRaw Format
	var fr FormatRaw
	if err := json.Unmarshal(b, &fr); err != nil {
		return err
	}
	*f = Format(fr)

	if f.Formats.Empty() {
		return fmt.Errorf("Formats can't be empty")
	}

	if format, _ := f.Formats.First(); format == "rss" {
		if f.EnclosureFormat == "" {
			return fmt.Errorf("EnclosureFormat can't be empty for")
		}
		return nil
	}

	if f.Ext == "" {
		return fmt.Errorf("Format ext can't be empty")
	}
	if f.MIMEType == "" {
		return fmt.Errorf("Format mimetype can't be empty")
	}

	return nil
}

type Stream struct {
	Specifier string
	Codecs    []Codec

	Media      mediaType         `json:"-"`
	CodecNames stringprioset.Set `json:"-"`
}

func (s *Stream) UnmarshalJSON(b []byte) (err error) {
	type StreamRaw Stream
	var sr StreamRaw
	if err := json.Unmarshal(b, &sr); err != nil {
		return err
	}
	*s = Stream(sr)

	if strings.HasPrefix(s.Specifier, "a:") {
		s.Media = MediaAudio
	} else if strings.HasPrefix(s.Specifier, "v:") {
		s.Media = MediaVideo
	} else {
		return fmt.Errorf("stream specifier must be a: or v: is %s", s.Specifier)
	}

	var codecNames []string
	for _, c := range s.Codecs {
		codecNames = append(codecNames, c.Name)
	}
	s.CodecNames = stringprioset.New(codecNames)

	return nil
}

// Codec codec name and ffmpeg args
type Codec struct {
	Name        string
	Flags       []string
	FormatFlags []string
}

func (c *Codec) UnmarshalJSON(b []byte) (err error) {
	var codecString string
	type CodecRaw Codec
	var codecRaw CodecRaw

	if err := json.Unmarshal(b, &codecString); err == nil {
		*c = Codec{Name: codecString}
	} else if err := json.Unmarshal(b, &codecRaw); err == nil {
		*c = Codec(codecRaw)
	} else {
		return err
	}

	if c.Name == "" {
		return fmt.Errorf("codec name can't be empty")
	}

	return nil
}

func (f Format) String() string {
	return fmt.Sprintf("%v:%v:%s:%s:%s",
		f.Formats,
		f.Streams,
		f.Ext,
		f.Prepend,
		f.MIMEType,
	)
}

// Formats map name to Format
type Formats map[string]Format

func (f *Formats) UnmarshalJSON(b []byte) (err error) {
	type FormatsRaw Formats
	var fr FormatsRaw
	if err := json.Unmarshal(b, &fr); err != nil {
		return err
	}

	for formatName, format := range fr {
		format.Name = formatName
		fr[formatName] = format
	}

	for formatName, format := range fr {
		if format.EnclosureFormat != "" {
			enclosureFormat, enclosureFormatOk := fr[format.EnclosureFormat]
			if !enclosureFormatOk {
				return fmt.Errorf("EnclosureFormat %s not found", format.EnclosureFormat)
			}

			// add format name as first option to make codec check etc work
			options := append([]string{enclosureFormat.Name}, format.EnclosureFormatOptions...)
			requestOptions, requestOptionsErr := NewRequestOptionsFromOpts(
				options, Formats(fr),
			)
			if requestOptionsErr != nil {
				return fmt.Errorf("EnclosureFormatOptions %s: %s", format.EnclosureFormat, requestOptionsErr)

				return requestOptionsErr
			}
			format.EnclosureRequestOptions = requestOptions
		}

		fr[formatName] = format
	}

	*f = Formats(fr)

	return nil
}

// FindByName find format by name
func (fs Formats) FindByName(name string) (Format, bool) {
	for formatName, format := range fs {
		if formatName == name {
			return format, true
		}
	}

	return Format{}, false
}

// FindByFormatCodecs find format by format and codecs
// prioritize formats with less codecs (more specific)
// return format and format name, format name is empty if not found
func (fs Formats) FindByFormatCodecs(format string, codecs []string) (Format, string) {
	var bestFormat Format
	var bestFormatName string
	var bestFormatStreamCodecs uint

	codecsSet := stringprioset.New(codecs)

	for name, f := range fs {
		if v, ok := f.Formats.First(); !ok || v != format {
			continue
		}

		codecsFound := 0
		streamCodecs := uint(0)
		for _, s := range f.Streams {
			streamCodecs += uint(len(s.Codecs))

			if !codecsSet.Intersect(s.CodecNames).Empty() {
				codecsFound++
			}
		}

		if codecsFound != len(f.Streams) ||
			codecsFound != len(codecs) ||
			bestFormatStreamCodecs >= streamCodecs {
			continue
		}

		bestFormat = f
		bestFormatName = name
		bestFormatStreamCodecs = streamCodecs
	}

	return bestFormat, bestFormatName
}

func parseConfig(r io.Reader) (Config, error) {
	c := Config{}

	d := json.NewDecoder(r)
	if err := d.Decode(&c); err != nil {
		return Config{}, err
	}

	return c, nil
}
