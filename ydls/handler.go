package ydls

import (
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
)

// URL encode with space encoded as "%20"
func urlEncode(s string) string {
	return strings.Replace(url.QueryEscape(s), "+", "%20", -1)
}

// make string safe to use in non-encoded content disposition filename
func safeContentDispositionFilename(s string) string {
	rs := []rune(s)
	for i, r := range rs {
		if r < 0x20 || r > 0x7e || r == '"' || r == '/' || r == '\\' {
			rs[i] = '_'
		}
	}

	return string(rs)
}

func splitRequestURL(URL *url.URL) (format string, urlStr string, forceACodec string, forceVCodec string) {
	if URL.Query().Get("url") != "" {
		// ?url=url&format=format&acodec=&vcodec=
		return URL.Query().Get("format"), URL.Query().Get("url"), URL.Query().Get("acodec"), URL.Query().Get("vcodec")
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
		return "", "", "", ""
	}

	if len(parts) == 2 {
		// had schema:// but split has removed one /
		s := parts[0] + "/" + parts[1]
		if URL.RawQuery != "" {
			s += "?" + URL.RawQuery
		}
		return format, s, "", ""

	}

	s := parts[0]
	if URL.RawQuery != "" {
		s += "?" + URL.RawQuery
	}

	return format, s, "", ""
}

func parseFormatDownloadURL(URL *url.URL) (format string, downloadURL *url.URL, forceACodec string, forceVCodec string) {
	var urlStr string
	format, urlStr, forceACodec, forceVCodec = splitRequestURL(URL)

	if !strings.HasPrefix(urlStr, "http://") && !strings.HasPrefix(urlStr, "https://") {
		urlStr = "http://" + urlStr
	}

	downloadURL, err := url.Parse(urlStr)
	if err != nil {
		return "", nil, "", ""
	}

	if downloadURL.Host == "" ||
		(downloadURL.Scheme != "http" && downloadURL.Scheme != "https") {
		return "", nil, "", ""
	}

	return format, downloadURL, forceACodec, forceVCodec
}

// Handler is a http.Handler using ydls
type Handler struct {
	YDLS      *YDLS
	IndexTmpl *template.Template
	InfoLog   *log.Logger
	DebugLog  *log.Logger
}

func (yh *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	infoLog := logOrDiscard(yh.InfoLog)
	debugLog := logOrDiscard(yh.DebugLog)

	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-XSS-Protection", "1; mode=block")

	debugLog.Printf("%s Request %s %s", r.RemoteAddr, r.Method, r.URL.String())

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if r.URL.Path == "/" && r.URL.RawQuery == "" {
		if yh.IndexTmpl != nil {
			w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src 'self'; style-src 'unsafe-inline'; form-action 'self'")
			yh.IndexTmpl.Execute(w, yh.YDLS.Formats)
		} else {
			http.Error(w, "Not found", http.StatusNotFound)
		}
		return
	} else if r.URL.Path == "/favicon.ico" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	formatName, downloadURL, forceACodec, forceVCodec := parseFormatDownloadURL(r.URL)
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

	dr, err := yh.YDLS.Download(r.Context(), downloadURL.String(), formatName,
		DownloadOptions{
			DebugLog:    debugLog,
			ForceACodec: forceACodec,
			ForceVCodec: forceVCodec,
		},
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Security-Policy", "default-src 'none'; reflected-xss block")
	w.Header().Set("Content-Type", dr.MIMEType)
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename*=UTF-8''%s; filename=\"%s\"",
			urlEncode(dr.Filename), safeContentDispositionFilename(dr.Filename)),
	)

	io.Copy(w, dr.Media)
	dr.Media.Close()
	dr.Wait()
}
