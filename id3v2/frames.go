package id3v2

import "bytes"

// Frame is an interface implemented by types to serialize to a ID3v2 frame
type Frame interface {
	ID3v2FrameID() string
	ID3v2FrameBytes() []byte // TODO: io.Writer instead?
}

// TextFrame ID3v2 text frame
type TextFrame struct {
	ID   string // 4 bytes
	Text string
}

func (tf *TextFrame) ID3v2FrameID() string {
	return tf.ID
}

func (tf *TextFrame) ID3v2FrameBytes() []byte {
	b := &bytes.Buffer{}
	binaryWriteMany(b, []interface{}{
		uint8(TextEncodingUTF8),
		[]byte(tf.Text),
		uint8(0),
	})
	return b.Bytes()
}

// COMMFrame ID3v2 COMM frame
type COMMFrame struct {
	Language    string // 3 bytes
	Description string
	Text        string
}

func (cf *COMMFrame) ID3v2FrameID() string {
	return "COMM"
}

func (cf *COMMFrame) ID3v2FrameBytes() []byte {
	b := &bytes.Buffer{}
	binaryWriteMany(b, []interface{}{
		uint8(TextEncodingUTF8),
		[]byte(cf.Language),
		[]byte(cf.Description),
		uint8(0),
		[]byte(cf.Text),
	})
	return b.Bytes()
}

// APICFrame ID3v2 APIC frame
type APICFrame struct {
	MIMEType    string
	PictureType uint8
	Description string
	Data        []byte
}

func (af *APICFrame) ID3v2FrameID() string {
	return "APIC"
}

func (af *APICFrame) ID3v2FrameBytes() []byte {
	b := &bytes.Buffer{}
	binaryWriteMany(b, []interface{}{
		uint8(TextEncodingUTF8),
		[]byte(af.MIMEType),
		uint8(0),
		uint8(af.PictureType),
		[]byte(af.Description),
		uint8(0),
		af.Data,
	})
	return b.Bytes()
}
