package writelogger

import (
	"bytes"
	"unicode"
)

type Printer interface {
	Printf(format string, v ...interface{})
}

// WriteLogger io.Writer that logs each lines with optional prefix
type WriteLogger struct {
	Printer Printer
	Prefix  string
	buf     bytes.Buffer
}

// New return a initialized WriteLogger
func New(printer Printer, prefix string) *WriteLogger {
	return &WriteLogger{Printer: printer, Prefix: prefix}
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

		wl.Printer.Printf("%s%s", wl.Prefix, string(lineRunes))
		pos += i + 1
	}
	wl.buf.Reset()
	wl.buf.Write(b[pos:])

	return len(p), nil
}

func (wl *WriteLogger) Close() error {
	if wl.buf.Len() > 0 {
		wl.Write([]byte{'\n'})
	}
	return nil
}
