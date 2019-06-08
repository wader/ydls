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
				[]byte("download\n"),
				[]byte("\rprogress 1"),
				[]byte("\rprogress 2"),
				[]byte("\rprogress 3"),
				[]byte("\n"),
			},
			[]byte(">download\n>\n>progress 1\n>progress 2\n>progress 3\n"),
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
				[]byte("\b\n"),
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
		{
			[][]byte{
				[]byte("a\n"),
				[]byte("b"),
			},
			[]byte(">a\n>b\n"),
		},
	} {

		actualBuf := &bytes.Buffer{}
		log := log.New(actualBuf, "", 0)
		wl := New(log, ">")

		for _, w := range c.writes {
			wl.Write(w)
		}
		wl.Close()

		if !reflect.DeepEqual(actualBuf.Bytes(), c.expected) {
			t.Errorf("writes %#v, expected %#v, actual %#v", c.writes, string(c.expected), string(actualBuf.Bytes()))
		}
	}
}
