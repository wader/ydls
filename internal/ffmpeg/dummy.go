package ffmpeg

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// Dummy create a reader that is a dummy media file with requested format and codecs
func Dummy(format string, acodec string, vcodec string) (io.Reader, error) {
	var err error

	// black screen and no sound
	dummyFileCmd := exec.Command(
		"ffmpeg",
		"-f", "lavfi", "-i", "color=s=cga:d=1",
		"-f", "lavfi", "-i", "anullsrc",
		"-map", "0:0", "-acodec", acodec,
		"-map", "1:0", "-vcodec", vcodec,
		"-t", "1",
		"-f", format,
		"pipe:1",
	)

	stdoutBuf := &bytes.Buffer{}
	stderrBuf := &bytes.Buffer{}
	dummyFileCmd.Stdout = stdoutBuf
	dummyFileCmd.Stderr = stderrBuf

	if err = dummyFileCmd.Run(); err != nil {
		return nil, fmt.Errorf(
			"cmd failed: %s: %s",
			strings.Join(dummyFileCmd.Args, " "),
			string(stderrBuf.Bytes()),
		)
	}

	return stdoutBuf, nil
}
