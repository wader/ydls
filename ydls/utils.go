package ydls

import (
	"bytes"
	"io"
	"log"
	"unicode"
)

// io.Writer that uses log.Logger to log lines with optional prefix
type loggerWriter struct {
	Logger *log.Logger
	Prefix string
	buf    bytes.Buffer
}

func (lw *loggerWriter) Write(p []byte) (n int, err error) {
	lw.buf.Write(p)

	b := lw.buf.Bytes()
	pos := 0

	for {
		i := bytes.IndexByte(b[pos:], '\n')
		if i < 0 {
			break
		}

		// replace non-printable runes with whitespace otherwise fancy progress
		// bars etc might mess up output
		lineRunes := []rune(string(b[pos : pos+i]))
		for i, r := range lineRunes {
			if !unicode.IsPrint(r) {
				lineRunes[i] = ' '
			}
		}

		lw.Logger.Print(lw.Prefix + string(lineRunes))
		pos += i + 1

	}
	lw.buf.Truncate(lw.buf.Len() - pos)

	return len(p), nil
}

func firstNonEmpty(sl ...string) string {
	for _, s := range sl {
		if s != "" {
			return s
		}
	}
	return ""
}

func boolString(b bool, t string, f string) string {
	if b {
		return t
	}
	return f
}

// TODO: can this be done without being a io.Closer?
// reReadCloser buffers all reads until Restarted is set to true
// then they are replyed and after that continue reading again
type reReadCloser struct {
	io.ReadCloser
	Buffer    bytes.Buffer
	Restarted bool
}

func (rr *reReadCloser) Read(p []byte) (n int, err error) {
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
