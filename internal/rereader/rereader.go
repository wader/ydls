package rereader

import (
	"bytes"
	"io"
)

type restartBuffer struct {
	Buffer    bytes.Buffer
	Restarted bool
}

func (rb *restartBuffer) Read(r io.Reader, p []byte) (n int, err error) {
	if rb.Restarted {
		if rb.Buffer.Len() > 0 {
			return rb.Buffer.Read(p)
		}
		n, err = r.Read(p)
		return n, err
	}

	n, err = r.Read(p)
	rb.Buffer.Write(p[:n])

	return n, err
}

// ReReader transparently buffers all reads from a reader until Restarted
// is set to true. When restarted buffered data will be replayed on read and
// after that normal reading from the reader continues.
type ReReader struct {
	io.Reader
	restartBuffer
}

// NewReReader return a initialized ReReader
func NewReReader(r io.Reader) *ReReader {
	return &ReReader{Reader: r}
}

func (rr *ReReader) Read(p []byte) (n int, err error) {
	return rr.restartBuffer.Read(rr.Reader, p)
}

// ReReadCloser is same as ReReader but also forwards Close calls
type ReReadCloser struct {
	io.ReadCloser
	restartBuffer
}

// NewReReadCloser return a initialized ReReadCloser
func NewReReadCloser(rc io.ReadCloser) *ReReadCloser {
	return &ReReadCloser{ReadCloser: rc}
}

func (rc *ReReadCloser) Read(p []byte) (n int, err error) {
	return rc.restartBuffer.Read(rc.ReadCloser, p)
}
