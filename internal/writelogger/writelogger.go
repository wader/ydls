package writelogger

import (
	"bytes"
	"log"
	"unicode"
)

// WriteLogger io.Writer that logs each lines with optional prefix
type WriteLogger struct {
	Logger *log.Logger
	Prefix string
	buf    bytes.Buffer
}

// New return a initialized WriteLogger
func New(logger *log.Logger, prefix string) *WriteLogger {
	return &WriteLogger{Logger: logger, Prefix: prefix}
}

// same as bytes.IndexByte but with set of bytes to look for
func indexByteSet(s []byte, cs []byte) int {
	ri := -1

	for _, c := range cs {
		i := bytes.IndexByte(s, c)
		if i != -1 && (ri == -1 || i < ri) {
			ri = i
		}
	}

	return ri
}

func (wl *WriteLogger) Write(p []byte) (n int, err error) {
	wl.buf.Write(p)

	b := wl.buf.Bytes()
	pos := 0

	for {
		i := indexByteSet(b[pos:], []byte{'\n', '\r'})
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

		wl.Logger.Print(wl.Prefix + string(lineRunes))
		pos += i + 1
	}
	wl.buf.Reset()
	wl.buf.Write(b[pos:])

	return len(p), nil
}
