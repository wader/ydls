package main

import (
	"flag"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/wader/ydls/ydls"
)

var infoFlag = flag.Bool("info", false, "info output")
var debugFlag = flag.Bool("debug", false, "debug output")
var formatsFlag = flag.String("formats", "formats.json", "formats config file")
var indexFlag = flag.String("index", "", "index html template")
var listenFlag = flag.String("listen", ":8080", "listen address")

var infoLog = log.New(ioutil.Discard, "INFO: ", log.Ltime)
var debugLog = log.New(ioutil.Discard, "DEBUG: ", log.Ltime)

func init() {
	flag.Parse()

	if os.Getenv("DEBUG") != "" {
		*debugFlag = true
	}
	if *infoFlag {
		infoLog.SetOutput(os.Stdout)
	}
	if *debugFlag {
		debugLog.SetOutput(os.Stdout)
	}
}

// URL encode with space encoded as "%20"
func urlEncode(s string) string {
	return strings.Replace(url.QueryEscape(s), "+", "%20", -1)
}

// make string safe to use in non-encoded content disposition filename
func contentDispositionFilename(s string) string {
	rs := []rune(s)
	for i, r := range rs {
		if r < 0x20 || r > 0x7e || r == '"' || r == '/' || r == '\\' {
			rs[i] = '_'
		}
	}

	return string(rs)
}

func splitRequestURL(URL *url.URL) (format string, urlStr string) {
	if URL.Query().Get("url") != "" {
		// ?url=url&format=format
		return URL.Query().Get("format"), URL.Query().Get("url")
	}

	// /format/schema://host.domin/path?query
	// /format/host.domain/path?query
	// /schema://host.domain/path?query
	// /host.domain/path?query

	parts := strings.SplitN(URL.Path, "/", 3)
	// parts[0] always empty, path always starts with /
	parts = parts[1:]

	// format? part does not contains ":" or "."
	if !strings.Contains(parts[0], ":") && !strings.Contains(parts[0], ".") {
		format = parts[0]
		parts = parts[1:]
	}

	if len(parts) == 0 {
		return "", ""
	}

	if len(parts) == 2 {
		// had schema:// but split has removed one /
		return format, parts[0] + "/" + parts[1] + "?" + URL.RawQuery

	}

	return format, parts[0] + "?" + URL.RawQuery
}

func parseFormatDownloadURL(URL *url.URL) (format string, downloadURL *url.URL) {
	var urlStr string
	format, urlStr = splitRequestURL(URL)

	if !strings.HasPrefix(urlStr, "http://") && !strings.HasPrefix(urlStr, "https://") {
		urlStr = "http://" + urlStr
	}

	downloadURL, err := url.Parse(urlStr)
	if err != nil {
		return "", nil
	}

	if downloadURL.Host == "" ||
		(downloadURL.Scheme != "http" && downloadURL.Scheme != "https") {
		return "", nil
	}

	return format, downloadURL
}

type ydlsHandler struct {
	ydls      *ydls.YDLs
	indexTmpl *template.Template
}

func (yh *ydlsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")

	debugLog.Printf("%s Request %s %s", r.RemoteAddr, r.Method, r.URL.String())

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if r.URL.Path == "/" && r.URL.RawQuery == "" {
		if yh.indexTmpl != nil {
			w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'unsafe-inline'; form-action 'self'; reflected-xss block")
			yh.indexTmpl.Execute(w, yh.ydls.Formats)
		} else {
			http.Error(w, "Not found", http.StatusNotFound)
		}
		return
	} else if r.URL.Path == "/favicon.ico" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	formatName, downloadURL := parseFormatDownloadURL(r.URL)
	if downloadURL == nil {
		infoLog.Printf("%s Invalid request %s %s", r.RemoteAddr, r.Method, r.URL.Path)
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}

	fancyFormatName := "best"
	if formatName != "" {
		fancyFormatName = formatName
	}
	infoLog.Printf("%s Downloading (%s) %s", r.RemoteAddr, fancyFormatName, downloadURL)

	mediaReader, filename, mimeType, err := yh.ydls.Download(downloadURL.String(), formatName, debugLog)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer mediaReader.Close()

	w.Header().Set("Content-Security-Policy", "default-src 'none'; reflected-xss block")
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename*=UTF-8''%s; filename=\"%s\"",
			urlEncode(filename), contentDispositionFilename(filename)),
	)

	io.Copy(w, mediaReader)
}

func main() {
	var yh ydlsHandler
	var err error

	yh.ydls, err = ydls.NewFromFile(*formatsFlag)
	if err != nil {
		log.Fatalf("failed to read formats: %s", err)
	}

	if *indexFlag != "" {
		yh.indexTmpl, err = template.ParseFiles(*indexFlag)
		if err != nil {
			log.Fatalf("failed to parse index template: %s", err)
		}
	}

	log.Printf("service listen on %s", *listenFlag)
	log.Fatal(http.ListenAndServe(*listenFlag, &yh))
}
