package ydls

import (
	"html/template"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
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

	ts := httptest.NewServer(ydlsHandlerFromFormatsEnv(t))
	defer ts.Close()

	testMedia := "https://www.youtube.com/watch?v=uVYWQJ5BB_w"
	testServerURL, _ := url.Parse(ts.URL)
	ydlsTestURL := testServerURL.ResolveReference(&url.URL{
		Path: "/mp3/" + testMedia,
	})

	res, err := http.Get(ydlsTestURL.String())
	if err != nil {
		t.Fatal(err)
	}
	mediaBytes, err := ioutil.ReadAll(io.LimitReader(res.Body, 1024))
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}

	if len(mediaBytes) != 1024 {
		t.Errorf("expected 1024 bytes, got %d", len(mediaBytes))
	}
	if res.Header.Get("Content-Disposition") == "" {
		t.Error("expected a Content-Disposition header")
	}
	if res.Header.Get("Content-Type") == "mpeg/audio" {
		t.Errorf("expected a Content-Type to be mpeg/audio, got %s", res.Header.Get("Content-Type"))
	}
}

func TestYDLSHandlerBadURL(t *testing.T) {
	if !testNetwork || !testFfmpeg || !testYoutubeldl {
		t.Skip("TEST_NETWORK, TEST_FFMPEG, TEST_YOUTUBEDL env not set")
	}

	ts := httptest.NewServer(ydlsHandlerFromFormatsEnv(t))
	defer ts.Close()

	testServerURL, _ := url.Parse(ts.URL)
	ydlsTestURL := testServerURL.ResolveReference(&url.URL{
		Path: "/badurl",
	})

	res, err := http.Get(ydlsTestURL.String())
	if err != nil {
		t.Fatal(err)
	}

	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("expected bad request, got %d", res.StatusCode)
	}
}

func TestYDLSHandlerIndexTemplate(t *testing.T) {
	if !testNetwork || !testFfmpeg || !testYoutubeldl {
		t.Skip("TEST_NETWORK, TEST_FFMPEG, TEST_YOUTUBEDL env not set")
	}

	h := ydlsHandlerFromFormatsEnv(t)
	h.IndexTmpl, _ = template.New("index").Parse("hello")
	ts := httptest.NewServer(h)
	defer ts.Close()

	testServerURL, _ := url.Parse(ts.URL)
	res, err := http.Get(testServerURL.String())
	if err != nil {
		t.Fatal(err)
	}

	if res.StatusCode != http.StatusOK {
		t.Errorf("expected ok, got %d", res.StatusCode)
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}

	if string(body) != "hello" {
		t.Errorf("expected hello, got %s", string(body))
	}
}
