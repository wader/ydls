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

func splitRequestURL(URL *url.URL) (formatAndOpts string, urlStr string) {
	// /format/schema://host.domin/path?query
	// /format/host.domain/path?query
	// /schema://host.domain/path?query
	// /host.domain/path?query

	parts := strings.SplitN(URL.Path, "/", 3)
	// parts[0] always empty, path always starts with /
	parts = parts[1:]

	// format? part does not contains ":" or "."
	if !strings.Contains(parts[0], ":") && !strings.Contains(parts[0], ".") {
		formatAndOpts = parts[0]
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
		return formatAndOpts, s

	}

	s := parts[0]
	if URL.RawQuery != "" {
		s += "?" + URL.RawQuery
	}

	return formatAndOpts, s
}

// Handler is a http.Handler using ydls
type Handler struct {
	YDLS      YDLS
	IndexTmpl *template.Template
	InfoLog   *log.Logger
	DebugLog  *log.Logger
}

func (yh *Handler) parseFormatDownloadURL(URL *url.URL) (DownloadOptions, error) {
	var urlStr string
	var optStrings []string

	if URL.Query().Get("url") != "" {
		// ?url=url&format=format&acodec=&vcodec=...

		urlStr = URL.Query().Get("url")

		optStrings = append(optStrings, URL.Query().Get("format"))

		if v := URL.Query().Get("acodec"); v != "" {
			optStrings = append(optStrings, v)
		}
		if v := URL.Query().Get("vcodec"); v != "" {
			optStrings = append(optStrings, v)
		}
		if v := URL.Query().Get("retranscode"); v != "" {
			optStrings = append(optStrings, "retranscode")
		}
	} else {
		// /format+opts.../url

		var formatAndOpts string
		formatAndOpts, urlStr = splitRequestURL(URL)
		optStrings = strings.Split(formatAndOpts, "+")
	}

	if len(optStrings) == 0 {
		return DownloadOptions{URL: urlStr}, nil
	}

	return yh.YDLS.ParseDownloadOptions(urlStr, optStrings[0], optStrings[1:])
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
			yh.IndexTmpl.Execute(w, yh.YDLS.Config.Formats)
		} else {
			http.Error(w, "Not found", http.StatusNotFound)
		}
		return
	} else if r.URL.Path == "/favicon.ico" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	downloadOptions, err := yh.parseFormatDownloadURL(r.URL)
	if err != nil {
		infoLog.Printf("%s Invalid request %s %s (%s)", r.RemoteAddr, r.Method, r.URL.Path, err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if url, urlErr := url.Parse(downloadOptions.URL); err != nil {
		infoLog.Printf("%s Invalid download URL %s %s (%s)", r.RemoteAddr, r.Method, r.URL.Path, urlErr.Error())
		http.Error(w, urlErr.Error(), http.StatusBadRequest)
		return
	} else if url.Scheme != "http" && url.Scheme != "https" {
		infoLog.Printf("%s Invalid URL scheme %s %s (%s)", r.RemoteAddr, r.Method, r.URL.Path, url.Scheme)
		http.Error(w, "Invalid download URL scheme", http.StatusBadRequest)
	}

	infoLog.Printf("%s Downloading (%s) %s", r.RemoteAddr, firstNonEmpty(downloadOptions.Format, "best"), downloadOptions.URL)

	dr, err := yh.YDLS.Download(
		r.Context(),
		downloadOptions,
		debugLog,
	)
	if err != nil {
		infoLog.Printf("%s Download failed %s %s (%s)", r.RemoteAddr, r.Method, r.URL.Path, err.Error())
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
