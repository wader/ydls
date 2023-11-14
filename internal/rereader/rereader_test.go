package rereader

import (
	"bytes"
	"io"
	"reflect"
	"testing"
)

func testShort(t *testing.T, r io.Reader, w io.Writer, restart func()) {
	b1 := make([]byte, 1)
	b2 := make([]byte, 2)

	if _, err := w.Write([]byte{0, 1, 2, 3}); err != nil {
		t.Fatal(err)
	}

	if n, err := io.ReadFull(r, b2); err != nil || n != 2 || !reflect.DeepEqual(b2[:n], []byte{0, 1}) {
		t.Errorf("read %#v %#v %#v", err, n, b2)
	}
	restart()
	if n, err := io.ReadFull(r, b1); err != nil || n != 1 || !reflect.DeepEqual(b1[:n], []byte{0}) {
		t.Errorf("read %#v %#v %#v", err, n, b1)
	}
	if n, err := io.ReadFull(r, b2); err != nil || n != 2 || !reflect.DeepEqual(b2[:n], []byte{1, 2}) {
		t.Errorf("read %#v %#v %#v", err, n, b2)
	}
	if n, err := io.ReadFull(r, b2); err == nil || n != 1 || !reflect.DeepEqual(b2[:n], []byte{3}) {
		t.Errorf("read %#v %#v %#v", err, n, b2)
	}
}

func testLarger(t *testing.T, r io.Reader, w io.Writer, restart func()) {
	b4 := make([]byte, 4)

	if _, err := w.Write([]byte{0, 1}); err != nil {
		t.Fatal(err)
	}

	// read buffer larger than reread buffer
	if n, err := io.ReadFull(r, b4); err == nil || n != 2 || !reflect.DeepEqual(b4[:n], []byte{0, 1}) {
		t.Errorf("read %#v %#v %#v", err, n, b4)
	}
	restart()
	if n, err := io.ReadFull(r, b4); err == nil || n != 2 || !reflect.DeepEqual(b4[:n], []byte{0, 1}) {
		t.Errorf("read %#v %#v %#v", err, n, b4)
	}
}

func TestReReaderShort(t *testing.T) {
	b := &bytes.Buffer{}
	rr := NewReReader(b)
	testShort(t, rr, b, func() { rr.Restarted = true })
}

func TestReReaderLarge(t *testing.T) {
	b := &bytes.Buffer{}
	rr := NewReReader(b)
	testLarger(t, rr, b, func() { rr.Restarted = true })
}

type bufferCloser struct {
	bytes.Buffer
	closeCalled bool
}

func (bc *bufferCloser) Close() error {
	bc.closeCalled = true
	return nil
}

func TestReReadCloserShort(t *testing.T) {
	b := &bufferCloser{}
	rr := NewReReadCloser(b)
	testShort(t, rr, b, func() { rr.Restarted = true })
	rr.Close()
	if !b.closeCalled {
		t.Error("close not called")
	}
}

func TestReReadCloserLarge(t *testing.T) {
	b := &bufferCloser{}
	rr := NewReReadCloser(b)
	testLarger(t, rr, b, func() { rr.Restarted = true })
	rr.Close()
	if !b.closeCalled {
		t.Error("close not called")
	}
}
