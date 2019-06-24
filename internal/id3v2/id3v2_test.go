package id3v2

import (
	"bytes"
	"reflect"
	"testing"
)

func TestWrite(t *testing.T) {
	frames := []Frame{
		&TextFrame{ID: "TIT2", Text: "title"},
		&COMMFrame{Language: "lan", Description: "", Text: "description"},
		&APICFrame{
			MIMEType:    "image/test",
			PictureType: PictureTypeOther,
			Description: "description",
			Data:        []byte{1, 2, 3},
		},
	}

	actual := &bytes.Buffer{}
	Write(actual, frames)

	expected := []byte(
		"ID3\x03\x00\x00\x00\x00\x00[" +
			"TIT2\x00\x00\x00\a\x00\x00\x03title\x00" +
			"COMM\x00\x00\x00\x10\x00\x00\x03lan\x00description" +
			"APIC\x00\x00\x00\x1c\x00\x00\x03image/test\x00\x00description\x00\x01\x02\x03" +
			"\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00",
	)

	if !reflect.DeepEqual(actual.Bytes(), expected) {
		t.Errorf("expected '%#v' actual '%#v'", string(expected), actual.String())
	}
}
