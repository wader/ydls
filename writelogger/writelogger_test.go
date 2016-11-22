package writelogger

import (
	"bytes"
	"log"
	"reflect"
	"testing"
)

func TestWriteLogger(t *testing.T) {
	for _, c := range []struct {
		writes   [][]byte
		expected []byte
	}{
		{
			[][]byte{
				[]byte("aaa\n"),
				[]byte("bbb\n"),
			},
			[]byte(">aaa\n>bbb\n"),
		},
		{
			[][]byte{
				[]byte("a"),
				[]byte("b\n"),
			},
			[]byte(">ab\n"),
		},
		{
			[][]byte{
				[]byte("a"),
				[]byte("\n"),
			},
			[]byte(">a\n"),
		},
		{
			[][]byte{
				[]byte("a\nb\nc\n"),
			},
			[]byte(">a\n>b\n>c\n"),
		},
		{
			[][]byte{
				[]byte("a"),
				[]byte("\r\n"),
			},
			[]byte(">a \n"),
		},
		{
			[][]byte{
				[]byte("🐹"),
				[]byte("\n"),
			},
			[]byte(">🐹\n"),
		},
	} {

		actualBuf := &bytes.Buffer{}
		log := log.New(actualBuf, "", 0)
		lw := New(log, ">")

		for _, w := range c.writes {
			lw.Write(w)
		}

		if !reflect.DeepEqual(actualBuf.Bytes(), c.expected) {
			t.Errorf("writes %#v, expected %#v, actual %#v", c.writes, c.expected, actualBuf.Bytes())
		}
	}
}
