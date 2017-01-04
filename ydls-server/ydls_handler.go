package main

import (
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/wader/ydls/ydls"
)

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
		s := parts[0] + "/" + parts[1]
		if URL.RawQuery != "" {
			s += "?" + URL.RawQuery
		}
		return format, s

	}

	s := parts[0]
	if URL.RawQuery != "" {
		s += "?" + URL.RawQuery
	}

	return format, s
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
	ydls      *ydls.YDLS
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

	dr, err := yh.ydls.Download(r.Context(), downloadURL.String(), formatName, debugLog)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer dr.Media.Close()

	w.Header().Set("Content-Security-Policy", "default-src 'none'; reflected-xss block")
	w.Header().Set("Content-Type", dr.MIMEType)
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename*=UTF-8''%s; filename=\"%s\"",
			urlEncode(dr.Filename), contentDispositionFilename(dr.Filename)),
	)

	io.Copy(w, dr.Media)
}
