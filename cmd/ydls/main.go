package main

import (
	"context"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/wader/ydls/internal/ydls"
)

var gitCommit = "dev"

var versionFlag = flag.Bool("version", false, "Print version ("+gitCommit+")")

var debugFlag = flag.Bool("debug", false, "Debug output")
var configFlag = flag.String("config", "ydls.json", "Config file")
var infoFlag = flag.Bool("info", false, "Info output")

var serverFlag = flag.Bool("server", false, "Start server")
var listenFlag = flag.String("listen", ":8080", "Listen address")
var indexFlag = flag.String("index", "", "Path to index template")
var noProgressFlag = flag.Bool("noprogress", false, "Don't print download progress")

func fatalIfErrorf(err error, format string, a ...interface{}) {
	if err != nil {
		a = append(a, err)
		log.Fatalf(format+": %v", a...)
	}
}

func init() {
	log.SetFlags(0)
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [flags] URL [options]...\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if *versionFlag {
		fmt.Println(gitCommit)
		os.Exit(0)
	}
	if os.Getenv("DEBUG") != "" {
		*debugFlag = true
	}
}

func server(y ydls.YDLS) {
	yh := &ydls.Handler{YDLS: y}

	if *infoFlag {
		yh.InfoLog = log.New(os.Stdout, "INFO: ", log.Ltime)
	}
	if *debugFlag {
		yh.DebugLog = log.New(os.Stdout, "DEBUG: ", log.Ltime)
	}
	if *indexFlag != "" {
		indexTmpl, err := template.ParseFiles(*indexFlag)
		fatalIfErrorf(err, "failed to parse index template")
		yh.IndexTmpl = indexTmpl
	}

	log.Printf("Listening on %s", *listenFlag)
	if err := http.ListenAndServe(*listenFlag, yh); err != nil {
		log.Fatal(err)
	}
}

type progressWriter struct {
	fn    func(bytes uint64)
	bytes uint64
}

func (pw *progressWriter) Write(p []byte) (n int, err error) {
	pw.bytes += uint64(len(p))
	pw.fn(pw.bytes)
	return len(p), nil
}

func absRootPath(root string, path string) (string, error) {
	abs, err := filepath.Abs(filepath.Join(root, path))
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(abs, filepath.Clean(root+string(filepath.Separator))) {
		return "", fmt.Errorf("%s is outside root path %s", abs, root)
	}

	return abs, nil
}

func download(y ydls.YDLS) {
	var debugLog ydls.Printer
	if *debugFlag {
		debugLog = log.New(os.Stdout, "DEBUG: ", log.Ltime)
	}

	rawURL := flag.Arg(0)
	if rawURL == "" {
		flag.Usage()
		os.Exit(1)
	}

	requestOptions, requestOptionsErr := ydls.NewRequestOptionsFromOpts(flag.Args()[1:], y.Config.Formats)
	requestOptions.MediaRawURL = flag.Arg(0)
	fatalIfErrorf(requestOptionsErr, "format and options")

	ctx, cancelFn := context.WithCancel(context.Background())
	defer cancelFn()

	dr, err := y.Download(ctx, ydls.DownloadOptions{
		RequestOptions: requestOptions,
		DebugLog:       debugLog,
	})
	fatalIfErrorf(err, "download failed")
	defer dr.Media.Close()
	defer dr.Wait()

	wd, err := os.Getwd()
	fatalIfErrorf(err, "getwd")

	path, err := absRootPath(wd, dr.Filename)
	fatalIfErrorf(err, "write path")

	var mediaWriter io.Writer
	if dr.Filename != "" {
		mediaFile, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
		fatalIfErrorf(err, "failed to open file")
		defer mediaFile.Close()
		if *noProgressFlag {
			fmt.Println(dr.Filename)
			mediaWriter = mediaFile
		} else {
			pw := &progressWriter{fn: func(bytes uint64) {
				fmt.Printf("\r%s %.2fMB", dr.Filename, float64(bytes)/(1024*1024))
			}}
			mediaWriter = io.MultiWriter(mediaFile, pw)
		}
	} else {
		mediaWriter = os.Stdout
	}

	io.Copy(mediaWriter, dr.Media)
	dr.Media.Close()
	dr.Wait()
	fmt.Print("\n")
}

func main() {
	y, err := ydls.NewFromFile(*configFlag)
	fatalIfErrorf(err, "failed to read config")

	if *serverFlag {
		server(y)
	} else {
		download(y)
	}
}
