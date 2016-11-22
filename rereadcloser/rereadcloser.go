package rereadcloser

import (
	"bytes"
	"io"
)

// ReReadCloser transparently buffers all reads from a reader until Restarted
// is set to true. When restarted buffered data will be replayed on read and
// after that normal reading from the reader continues.
// TODO: can this be done without being a io.Closer?
type ReReadCloser struct {
	io.ReadCloser
	Buffer    bytes.Buffer
	Restarted bool
}

// New return a initialized ReReadCloser
func New(rc io.ReadCloser) *ReReadCloser {
	return &ReReadCloser{ReadCloser: rc}
}

func (rr *ReReadCloser) Read(p []byte) (n int, err error) {
	if rr.Restarted {
		if rr.Buffer.Len() > 0 {
			return rr.Buffer.Read(p)
		}
		n, err = rr.ReadCloser.Read(p)
		return n, err
	}

	n, err = rr.ReadCloser.Read(p)
	rr.Buffer.Write(p[:n])

	return n, err
}
