package ydls

import (
	"html/template"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/wader/ydls/leaktest"
	"github.com/wader/ydls/timerange"
)

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

func ydlsHandlerFromEnv(t *testing.T) *Handler {
	h := &Handler{}
	var err error

	h.YDLS, err = NewFromFile(os.Getenv("CONFIG"))
	if err != nil {
		t.Fatalf("failed to read config: %s", err)
	}

	return h
}

func TestParseFormatDownloadURL(t *testing.T) {
	h := ydlsHandlerFromEnv(t)

	for _, c := range []struct {
		url          *url.URL
		expectedOpts DownloadOptions
		expectedErr  bool
	}{
		{&url.URL{Path: "/mp3/http://domain/path", RawQuery: "query"},
			DownloadOptions{Format: "mp3", URL: "http://domain/path?query"}, false},
		{&url.URL{Path: "/mp3/http://domain/a/b"},
			DownloadOptions{Format: "mp3", URL: "http://domain/a/b"}, false},
		{&url.URL{Path: "/http://domain/path", RawQuery: "query"},
			DownloadOptions{Format: "", URL: "http://domain/path?query"}, false},
		{&url.URL{Path: "/", RawQuery: "url=http://domain.com&format=mp3"},
			DownloadOptions{Format: "mp3", URL: "http://domain.com"}, false},
		{&url.URL{Path: "/", RawQuery: "url=http://domain.com"},
			DownloadOptions{Format: "", URL: "http://domain.com"}, false},
		{&url.URL{Path: "/", RawQuery: "url=http://domain.com&format=mkv&acodec=flac&vcodec=theora"},
			DownloadOptions{Format: "mkv", URL: "http://domain.com", ACodec: "flac", VCodec: "theora"}, false},
		{&url.URL{Path: "/mkv+flac+theora/http://domain.com", RawQuery: ""},
			DownloadOptions{Format: "mkv", URL: "http://domain.com", ACodec: "flac", VCodec: "theora"}, false},
		{&url.URL{Path: "/mkv+flac+theora/http://domain.com", RawQuery: ""},
			DownloadOptions{Format: "mkv", URL: "http://domain.com", ACodec: "flac", VCodec: "theora"}, false},
		{&url.URL{Path: "/mp3+retranscode/http://domain.com", RawQuery: ""},
			DownloadOptions{Format: "mp3", URL: "http://domain.com", ACodec: "", VCodec: "", Retranscode: true}, false},
		{&url.URL{Path: "/", RawQuery: "url=http://domain.com&format=mp3&retranscode=1"},
			DownloadOptions{Format: "mp3", URL: "http://domain.com", ACodec: "", VCodec: "", Retranscode: true}, false},
		{&url.URL{Path: "/mkv+123s/http://domain.com", RawQuery: ""},
			DownloadOptions{Format: "mkv", URL: "http://domain.com", TimeRange: timerange.TimeRange{Stop: time.Second * 123}}, false},
		{&url.URL{Path: "/", RawQuery: "url=http://domain.com&format=mkv&time=123s"},
			DownloadOptions{Format: "mkv", URL: "http://domain.com", TimeRange: timerange.TimeRange{Stop: time.Second * 123}}, false},
		{&url.URL{Path: "/mkv+nope/http://domain.com", RawQuery: ""},
			DownloadOptions{}, true},
	} {
		opts, err := h.parseFormatDownloadURL(c.url)
		if err != nil {
			if !c.expectedErr {
				t.Errorf("url=%+v, got error %v, expected %#v", c.url, err, c.expectedOpts)
			}
		} else {
			if c.expectedErr {
				t.Errorf("url=%+v, got %#v, expected error", c.url, opts)
			} else if opts.Format != c.expectedOpts.Format || opts.URL != c.expectedOpts.URL ||
				opts.ACodec != c.expectedOpts.ACodec || opts.VCodec != c.expectedOpts.VCodec ||
				opts.Retranscode != c.expectedOpts.Retranscode {
				t.Errorf("url=%+v, got %#v, expected %#v", c.url, opts, c.expectedOpts)
			}
		}
	}
}

func TestYDLSHandlerDownload(t *testing.T) {
	if !testNetwork || !testFfmpeg || !testYoutubeldl {
		t.Skip("TEST_NETWORK, TEST_FFMPEG, TEST_YOUTUBEDL env not set")
	}

	defer leaktest.Check(t)()

	h := ydlsHandlerFromEnv(t)

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

	h := ydlsHandlerFromEnv(t)

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

	h := ydlsHandlerFromEnv(t)
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
