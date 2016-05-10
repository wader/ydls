package ydls

import (
	"bytes"
	"io"
	"io/ioutil"
	"log"
	"reflect"
	"testing"
)

func TestLogWriter(t *testing.T) {
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
		lw := &loggerWriter{Logger: log, Prefix: ">"}

		for _, w := range c.writes {
			lw.Write(w)
		}

		if !reflect.DeepEqual(actualBuf.Bytes(), c.expected) {
			t.Errorf("writes %#v, expected %#v, actual %#v", c.writes, c.expected, actualBuf.Bytes())
		}
	}
}

func TestReReadCloser(t *testing.T) {
	b1 := make([]byte, 1)
	b2 := make([]byte, 2)
	b4 := make([]byte, 4)

	rr1 := &reReadCloser{ReadCloser: ioutil.NopCloser(bytes.NewReader([]byte{0, 1, 2, 3}))}

	if n, err := io.ReadFull(rr1, b2); err != nil || n != 2 || !reflect.DeepEqual(b2[:n], []byte{0, 1}) {
		t.Errorf("read %#v %#v %#v", err, n, b2)
	}
	rr1.Restarted = true
	if n, err := io.ReadFull(rr1, b1); err != nil || n != 1 || !reflect.DeepEqual(b1[:n], []byte{0}) {
		t.Errorf("read %#v %#v %#v", err, n, b1)
	}
	if n, err := io.ReadFull(rr1, b2); err != nil || n != 2 || !reflect.DeepEqual(b2[:n], []byte{1, 2}) {
		t.Errorf("read %#v %#v %#v", err, n, b2)
	}
	if n, err := io.ReadFull(rr1, b2); err == nil || n != 1 || !reflect.DeepEqual(b2[:n], []byte{3}) {
		t.Errorf("read %#v %#v %#v", err, n, b2)
	}

	// read buffer larger than reread buffer
	rr2 := &reReadCloser{ReadCloser: ioutil.NopCloser(bytes.NewReader([]byte{0, 1}))}
	if n, err := io.ReadFull(rr2, b4); err == nil || n != 2 || !reflect.DeepEqual(b4[:n], []byte{0, 1}) {
		t.Errorf("read %#v %#v %#v", err, n, b4)
	}
	rr2.Restarted = true
	if n, err := io.ReadFull(rr2, b4); err == nil || n != 2 || !reflect.DeepEqual(b4[:n], []byte{0, 1}) {
		t.Errorf("read %#v %#v %#v", err, n, b4)
	}
}
