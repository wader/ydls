package id3v2

import (
	"bytes"
	"encoding/binary"
	"io"
)

// TextEncodingUTF8 constant for UTF-8 encoding in ID3v2
const TextEncodingUTF8 = 3

// PictureTypeOther APIC picture type other
const PictureTypeOther = 0

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

// Write write ID3v2 tag
func Write(w io.Writer, frames []Frame) {
	bufFrames := &bytes.Buffer{}

	for _, f := range frames {
		b := f.ID3v2FrameBytes()
		binaryWriteMany(bufFrames, []interface{}{
			[]byte(f.ID3v2FrameID()), // frame id
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
		[]byte("ID3"),  // ID3v2 header
		uint16(0x0300), // version 3
		uint8(0),       // no flags
		synchsafeUint32(uint32(len(b))),
		b,
	})
}
