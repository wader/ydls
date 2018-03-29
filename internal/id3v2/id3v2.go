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

func binaryWriteBE(w io.Writer, v interface{}) (int, error) {
	return binary.Size(v), binary.Write(w, binary.BigEndian, v)
}

func binaryWriteMany(w io.Writer, fields []interface{}) (int, error) {
	tn := 0

	for _, f := range fields {
		n, err := binaryWriteBE(w, f)
		if err != nil {
			return tn, err
		}
		tn += n
	}

	return tn, nil
}

// Write write ID3v2 tag
func Write(w io.Writer, frames []Frame) (int, error) {
	var err error
	framesBuf := &bytes.Buffer{}

	for _, f := range frames {
		frameBuf := &bytes.Buffer{}

		_, err = f.ID3v2FrameWriteTo(frameBuf)
		if err != nil {
			return 0, err
		}

		_, err = binaryWriteMany(framesBuf, []interface{}{
			[]byte(f.ID3v2FrameID()), // frame id
			uint32(frameBuf.Len()),   // len
			uint16(0),                // no flags
			frameBuf.Bytes(),         // frame data
		})
		if err != nil {
			return 0, err
		}
	}

	// ffmpeg pads 10 bytes to fix some broken readers
	pad := make([]byte, 10)
	_, err = binaryWriteBE(framesBuf, pad)
	if err != nil {
		return 0, err
	}

	return binaryWriteMany(w, []interface{}{
		[]byte("ID3"),  // ID3v2 header
		uint16(0x0300), // version 3
		uint8(0),       // no flags
		synchsafeUint32(uint32(framesBuf.Len())),
		framesBuf.Bytes(),
	})
}
