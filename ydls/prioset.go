package ydls

import (
	"encoding/json"
	"strings"
)

type prioStringSet []string

func (p prioStringSet) member(a string) bool {
	for _, c := range p {
		if c == a {
			return true
		}
	}
	return false
}

func (p prioStringSet) empty() bool {
	return len(p) == 0
}

func (p prioStringSet) first() string {
	if len(p) > 0 {
		return p[0]
	}
	return ""
}

func (p prioStringSet) String() string {
	return "[" + strings.Join(p, " ") + "]"
}

// need to be pointer type so value can be assigned
func (p *prioStringSet) UnmarshalJSON(b []byte) (err error) {
	var np []string
	err = json.Unmarshal(b, &np)
	*p = np
	return
}

type prioFormatCodecSet []FormatCodec

func (p prioFormatCodecSet) findByCodec(codec string) (FormatCodec, bool) {
	for _, fc := range p {
		if fc.Codec == codec {
			return fc, true
		}
	}
	return FormatCodec{}, false
}

func (p prioFormatCodecSet) hasCodec(codec string) bool {
	_, ok := p.findByCodec(codec)
	return ok
}

func (p prioFormatCodecSet) empty() bool {
	return len(p) == 0
}

func (p prioFormatCodecSet) first() (FormatCodec, bool) {
	if len(p) > 0 {
		return p[0], true
	}
	return FormatCodec{}, false
}

func (p prioFormatCodecSet) CodecNames() []string {
	var codecs []string
	for _, c := range p {
		codecs = append(codecs, c.Codec)
	}
	return codecs
}

func (p prioFormatCodecSet) PrioStringSet() prioStringSet {
	return prioStringSet(p.CodecNames())
}

func (p prioFormatCodecSet) String() string {
	return "[" + strings.Join(p.CodecNames(), " ") + "]"
}

// need to be pointer type so value can be assigned
func (p *prioFormatCodecSet) UnmarshalJSON(b []byte) (err error) {
	var formatCodecs []FormatCodec
	err = json.Unmarshal(b, &formatCodecs)
	*p = formatCodecs
	return
}
