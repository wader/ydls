package ydls

import (
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type baseURLXHeaders int

const (
	trustXHeaders baseURLXHeaders = iota
	dontTrustXHeaders
)

func baseURLFromRequest(r *http.Request, shouldXHeaders baseURLXHeaders) *url.URL {
	schema := ""
	host := ""
	prefix := ""
	if shouldXHeaders == trustXHeaders {
		schema = r.Header.Get("X-Forwarded-Proto")
		host = r.Header.Get("X-Forwarded-Host")
		prefix = r.Header.Get("X-Forwarded-Prefix")
	}

	if schema == "" {
		schema = "http"
		if r.TLS != nil {
			schema = "https"
		}
	}
	if host == "" {
		host = r.Host
	}

	return &url.URL{
		Scheme: schema,
		Host:   host,
		Path:   prefix,
	}
}

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

// Handler is a http.Handler using ydls
type Handler struct {
	YDLS      YDLS
	IndexTmpl *template.Template
	InfoLog   Printer
	DebugLog  Printer
}

func (yh *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	infoLog := yh.InfoLog
	if infoLog == nil {
		infoLog = nopPrinter{}
	}
	debugLog := yh.DebugLog
	if debugLog == nil {
		debugLog = nopPrinter{}
	}

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

	var downloadOptions DownloadOptions
	var downloadOptionsErr error
	if r.URL.Query().Get("url") != "" {
		// ?url=url&format=format&codec=&codec=...
		downloadOptions, downloadOptionsErr = NewDownloadOptionsFromQuery(r.URL.Query(), yh.YDLS.Config.Formats)
	} else {
		// /opt+opt.../http://...
		downloadOptions, downloadOptionsErr = NewDownloadOptionsFromPath(r.URL, yh.YDLS.Config.Formats)
	}
	if downloadOptionsErr != nil {
		infoLog.Printf("%s Invalid request %s %s (%s)", r.RemoteAddr, r.Method, r.URL.Path, downloadOptionsErr.Error())
		http.Error(w, downloadOptionsErr.Error(), http.StatusBadRequest)
		return
	}
	downloadOptions.BaseURL = baseURLFromRequest(r, trustXHeaders)

	formatName := "best"
	if downloadOptions.Format != nil {
		formatName = downloadOptions.Format.Name
	}
	infoLog.Printf("%s Downloading (%s) %s", r.RemoteAddr, formatName, downloadOptions.MediaRawURL)

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
	if dr.Filename != "" {
		w.Header().Set("Content-Disposition",
			fmt.Sprintf("attachment; filename*=UTF-8''%s; filename=\"%s\"",
				urlEncode(dr.Filename), safeContentDispositionFilename(dr.Filename)),
		)
	}

	io.Copy(w, dr.Media)
	dr.Media.Close()
	dr.Wait()
}
