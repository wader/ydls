package ydls

import (
	"html/template"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/wader/ydls/leaktest"
)

func TestParseFormatDownloadURL(t *testing.T) {
	for _, c := range []struct {
		url            *url.URL
		expectedFormat string
		expectedURL    string
	}{
		{&url.URL{Path: "/format/http://domain/path", RawQuery: "query"},
			"format", "http://domain/path?query"},
		{&url.URL{Path: "/format/http://domain/a/b"},
			"format", "http://domain/a/b"},
		{&url.URL{Path: "/format/domain.com/a/b"},
			"format", "http://domain.com/a/b"},
		{&url.URL{Path: "/http://domain/path", RawQuery: "query"},
			"", "http://domain/path?query"},
		{&url.URL{Path: "/domain.com/path", RawQuery: "query"},
			"", "http://domain.com/path?query"},
		{&url.URL{Path: "/", RawQuery: "url=http://domain.com&format=format"},
			"format", "http://domain.com"},
		{&url.URL{Path: "/", RawQuery: "url=http://domain.com"},
			"", "http://domain.com"},
		{&url.URL{Path: "/", RawQuery: "url=domain.com&format=format"},
			"format", "http://domain.com"},
		{&url.URL{Path: "/", RawQuery: "url=domain.com"},
			"", "http://domain.com"},
		{&url.URL{Path: "/b", RawQuery: "query"},
			"", ""},
		{&url.URL{Path: "/", RawQuery: "query"},
			"", ""},
	} {
		format, URL := parseFormatDownloadURL(c.url)
		if URL == nil {
			if c.expectedURL != "" {
				t.Errorf("url=%+v, got fail, expected format=%v url=%v",
					c.url, c.expectedFormat, c.expectedURL)
			}
		} else {
			if format != c.expectedFormat || URL.String() != c.expectedURL {
				t.Errorf("url=%+v, got format=%v url=%v expected format=%v url=%v",
					c.url, format, URL, c.expectedFormat, c.expectedURL)
			}
		}
	}
}

func TestURLEncode(t *testing.T) {
	for _, c := range []struct {
		s      string
		expect string
	}{
		{"abc", "abc"},
		{"a c", "a%20c"},
		{"/?&", "%2F%3F%26"},
	} {
		actual := urlEncode(c.s)
		if actual != c.expect {
			t.Errorf("%s, got %v expected %v", c.s, actual, c.expect)
		}
	}
}

func TestSafeContentDispositionFilename(t *testing.T) {
	for _, c := range []struct {
		s      string
		expect string
	}{
		{" abcdefghijklmnopqruvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789", " abcdefghijklmnopqruvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"},
		{"SPÆCIAL", "SP_CIAL"},
		{"\\\"/", "___"},
	} {
		actual := safeContentDispositionFilename(c.s)
		if actual != c.expect {
			t.Errorf("%s, got %v expected %v", c.s, actual, c.expect)
		}
	}
}

func ydlsHandlerFromFormatsEnv(t *testing.T) *Handler {
	h := &Handler{}
	var err error

	h.YDLS, err = NewFromFile(os.Getenv("FORMATS"))
	if err != nil {
		t.Fatalf("failed to read formats: %s", err)
	}

	return h
}

func TestYDLSHandlerDownload(t *testing.T) {
	if !testNetwork || !testFfmpeg || !testYoutubeldl {
		t.Skip("TEST_NETWORK, TEST_FFMPEG, TEST_YOUTUBEDL env not set")
	}

	defer leaktest.Check(t)()

	h := ydlsHandlerFromFormatsEnv(t)

	rr := httptest.NewRecorder()
	testMediaURL := "https://www.youtube.com/watch?v=C0DPdy98e4c"
	req := httptest.NewRequest("GET", "http://hostname/mp3/"+testMediaURL, nil)
	h.ServeHTTP(rr, req)
	resp := rr.Result()

	mediaBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	if len(mediaBytes) == 0 {
		t.Errorf("expected to get body")
	}
	if string(mediaBytes[0:2]) == "ID3" {
		t.Errorf("expected ID3 header")
	}
	if resp.Header.Get("Content-Disposition") == "" {
		t.Error("expected a Content-Disposition header")
	}
	if resp.Header.Get("Content-Type") == "mpeg/audio" {
		t.Errorf("expected a Content-Type to be mpeg/audio, got %s", resp.Header.Get("Content-Type"))
	}
}

func TestYDLSHandlerBadURL(t *testing.T) {
	defer leaktest.Check(t)()

	h := ydlsHandlerFromFormatsEnv(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://hostname/badurl", nil)
	h.ServeHTTP(rr, req)
	resp := rr.Result()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected bad request, got %d", resp.StatusCode)
	}
}

func TestYDLSHandlerIndexTemplate(t *testing.T) {
	defer leaktest.Check(t)()

	h := ydlsHandlerFromFormatsEnv(t)
	h.IndexTmpl, _ = template.New("index").Parse("hello")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://hostname/", nil)
	h.ServeHTTP(rr, req)
	resp := rr.Result()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected ok, got %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	if string(body) != "hello" {
		t.Errorf("expected hello, got %s", string(body))
	}
}
