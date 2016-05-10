package ydls

import (
	"bytes"
	"encoding/binary"
	"io"
)

const id3v2TextEncodingUTF8 = 3
const id3v2PictureTypeOther = 0

func synchsafeUint32(i uint32) uint32 {
	return (0 |
		((i & (0x7f << 0)) << 0) |
		((i & (0x7f << 7)) << 1) |
		((i & (0x7f << 14)) << 2) |
		((i & (0x7f << 21)) << 3))
}

func binaryWriteMany(w io.Writer, fields []interface{}) {
	for _, f := range fields {
		binary.Write(w, binary.BigEndian, f)
	}
}

type id3v2Frame interface {
	id3v2FrameID() string
	id3v2FrameBytes() []byte // TODO: io.Writer instead?
}

type textFrame struct {
	ID   string // 4 bytes
	Text string
}

func (tf *textFrame) id3v2FrameID() string {
	return tf.ID
}
func (tf *textFrame) id3v2FrameBytes() []byte {
	b := &bytes.Buffer{}
	binaryWriteMany(b, []interface{}{
		uint8(id3v2TextEncodingUTF8),
		[]byte(tf.Text),
		uint8(0),
	})
	return b.Bytes()
}

type commFrame struct {
	Language    string // 3 bytes
	Description string
	Text        string
}

func (cf *commFrame) id3v2FrameID() string {
	return "COMM"
}
func (cf *commFrame) id3v2FrameBytes() []byte {
	b := &bytes.Buffer{}
	binaryWriteMany(b, []interface{}{
		uint8(id3v2TextEncodingUTF8),
		[]byte(cf.Language),
		[]byte(cf.Description),
		uint8(0),
		[]byte(cf.Text),
	})
	return b.Bytes()
}

type apicFrame struct {
	MIMEType    string
	PictureType uint8
	Description string
	Data        []byte
}

func (af *apicFrame) id3v2FrameID() string {
	return "APIC"
}
func (af *apicFrame) id3v2FrameBytes() []byte {
	b := &bytes.Buffer{}
	binaryWriteMany(b, []interface{}{
		uint8(id3v2TextEncodingUTF8),
		[]byte(af.MIMEType),
		uint8(0),
		uint8(af.PictureType),
		[]byte(af.Description),
		uint8(0),
		af.Data,
	})
	return b.Bytes()
}

func id3v2WriteHeader(w io.Writer, frames []id3v2Frame) {
	bufFrames := &bytes.Buffer{}

	for _, f := range frames {
		b := f.id3v2FrameBytes()
		binaryWriteMany(bufFrames, []interface{}{
			[]byte(f.id3v2FrameID()), // frame id
			uint32(len(b)),           // len
			uint16(0),                // no flags
			b,                        // frame data
		})
	}

	// ffmpeg pads 10 bytes to fix some broken readers
	pad := make([]byte, 10)
	binary.Write(bufFrames, binary.BigEndian, pad)

	b := bufFrames.Bytes()
	binaryWriteMany(w, []interface{}{
		[]byte("ID3"),  // id3v2 header
		uint16(0x0300), // version 3
		uint8(0),       // no flags
		synchsafeUint32(uint32(len(b))),
		b,
	})
}
