package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/wader/ydls/ydls"
)

var debugFlag = flag.Bool("debug", false, "debug output")
var formatsFlag = flag.String("formats", "formats.json", "formats config file")
var debugLog = log.New(ioutil.Discard, "DEBUG: ", log.Ltime)

type progressWriter struct {
	fn    func(bytes uint64)
	bytes uint64
}

func (pw *progressWriter) Write(p []byte) (n int, err error) {
	pw.bytes += uint64(len(p))
	pw.fn(pw.bytes)
	return len(p), nil
}

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s [options] URL [format]:\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if os.Getenv("DEBUG") != "" {
		*debugFlag = true
	}
	if *debugFlag {
		debugLog.SetOutput(os.Stdout)
	}
}

func absRootPath(root string, path string) (string, error) {
	abs, err := filepath.Abs(filepath.Join(root, path))
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(abs, root+string(filepath.Separator)) {
		return "", fmt.Errorf("%s is outside root path %s", path, root)
	}

	return abs, nil
}

func fatalIfErrorf(err error, format string, a ...interface{}) {
	if err != nil {
		a = append(a, err)
		log.Fatalf(format+": %v", a...)
	}
}

func main() {
	ydls, err := ydls.NewFromFile(*formatsFlag)
	if err != nil {
		log.Fatalf("failed to read formats: %s", err)
	}

	url := flag.Arg(0)
	if url == "" {
		log.Fatalf("no URL specified")
	}
	formatName := flag.Arg(1)

	ctx, cancelFn := context.WithCancel(context.Background())
	defer cancelFn()

	dr, err := ydls.Download(ctx, url, formatName, debugLog)
	fatalIfErrorf(err, "download failed")
	defer dr.Media.Close()
	wd, err := os.Getwd()
	fatalIfErrorf(err, "getwd")
	path, err := absRootPath(wd, dr.Filename)
	fatalIfErrorf(err, "write path")

	mediaFile, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	fatalIfErrorf(err, "failed to open file")
	defer mediaFile.Close()

	pw := &progressWriter{fn: func(bytes uint64) {
		fmt.Printf("\r%s %.2fMB", dr.Filename, float64(bytes)/(1024*1024))
	}}
	mw := io.MultiWriter(mediaFile, pw)

	io.Copy(mw, dr.Media)
	fmt.Print("\n")
}
