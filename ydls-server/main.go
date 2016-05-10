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

func parseDownloadFormatURL(URL *url.URL) (format string, downloadURL *url.URL) {
	parts := strings.SplitN(URL.Path, "/", 3)
	if len(parts) != 3 {
		// ?url=url&format=format
		u, err := url.Parse(URL.Query().Get("url"))
		if err != nil {
			return "", nil
		}
		format = URL.Query().Get("format")

		return format, u
	}

	// format/schema://host.tld/path
	// format/host.tld/path
	// schema://host.tld/path
	// host.tld/path

	// note: query is in URL.RawQuery

	hostPath := ""
	if !(strings.Contains(parts[1], ":") || strings.Contains(parts[1], ".")) {
		format = parts[1]
		hostPath = parts[2]
	} else {
		format = ""
		hostPath = parts[1] + "/" + parts[2]
	}
	if !(strings.HasPrefix(hostPath, "http://") || strings.HasPrefix(hostPath, "https://")) {
		hostPath = "http://" + hostPath
	}

	u, err := url.Parse(hostPath)
	if err != nil {
		return "", nil
	}
	u.RawQuery = URL.RawQuery
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", nil
	}

	return format, u
}

type ydlsHandler struct {
	ydls      *ydls.YDLs
	indexTmpl *template.Template
}

func (yh *ydlsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")

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
	}

	formatName, downloadURL := parseDownloadFormatURL(r.URL)

	if downloadURL == nil {
		infoLog.Printf("%s Invalid request %s %s", r.RemoteAddr, r.Method, r.URL.Path)
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}

	infoLog.Printf("%s Request %s %s %s", r.RemoteAddr, r.Method, formatName, downloadURL)

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
