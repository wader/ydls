package rereadcloser

import (
	"bytes"
	"io"
	"io/ioutil"
	"reflect"
	"testing"
)

func TestReReadCloser(t *testing.T) {
	b1 := make([]byte, 1)
	b2 := make([]byte, 2)
	b4 := make([]byte, 4)

	rr1 := New(ioutil.NopCloser(bytes.NewReader([]byte{0, 1, 2, 3})))

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
	rr2 := New(ioutil.NopCloser(bytes.NewReader([]byte{0, 1})))
	if n, err := io.ReadFull(rr2, b4); err == nil || n != 2 || !reflect.DeepEqual(b4[:n], []byte{0, 1}) {
		t.Errorf("read %#v %#v %#v", err, n, b4)
	}
	rr2.Restarted = true
	if n, err := io.ReadFull(rr2, b4); err == nil || n != 2 || !reflect.DeepEqual(b4[:n], []byte{0, 1}) {
		t.Errorf("read %#v %#v %#v", err, n, b4)
	}
}
