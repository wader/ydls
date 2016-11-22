package writelogger

import (
	"bytes"
	"log"
	"unicode"
)

// WriteLogger io.Writer that uses a log.Logger to log lines with optional prefix
type WriteLogger struct {
	Logger *log.Logger
	Prefix string
	buf    bytes.Buffer
}

// New return a initialized WriteLogger
func New(logger *log.Logger, prefix string) *WriteLogger {
	return &WriteLogger{Logger: logger, Prefix: prefix}
}

func (wl *WriteLogger) Write(p []byte) (n int, err error) {
	wl.buf.Write(p)

	b := wl.buf.Bytes()
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

		wl.Logger.Print(wl.Prefix + string(lineRunes))
		pos += i + 1

	}
	wl.buf.Truncate(wl.buf.Len() - pos)

	return len(p), nil
}
