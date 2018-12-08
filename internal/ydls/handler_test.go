package ydls

import (
	"crypto/tls"
	"html/template"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/wader/ydls/internal/leaktest"
)

func TestBaseURLFromRequest(t *testing.T) {
	type rvars struct {
		tls  bool
		host string
	}
	type xvars struct {
		proto  string
		host   string
		prefix string
	}
	for _, c := range []struct {
		r               rvars
		x               xvars
		baseURLXHeaders baseURLXHeaders
		expect          string
	}{
		{rvars{true, "b"}, xvars{"", "", ""}, trustXHeaders, "https://b"},
		{rvars{false, "b"}, xvars{"", "", ""}, dontTrustXHeaders, "http://b"},

		{rvars{false, "b"}, xvars{"d", "e", "f"}, trustXHeaders, "d://e/f"},
		{rvars{false, "b"}, xvars{"d", "e", "f"}, dontTrustXHeaders, "http://b"},
		{rvars{true, "b"}, xvars{"d", "e", "f"}, dontTrustXHeaders, "https://b"},

		{rvars{true, "b"}, xvars{"d", "", ""}, trustXHeaders, "d://b"},
		{rvars{false, "b"}, xvars{"d", "", ""}, trustXHeaders, "d://b"},

		{rvars{true, "b"}, xvars{"", "e", ""}, trustXHeaders, "https://e"},
		{rvars{false, "b"}, xvars{"", "e", ""}, trustXHeaders, "http://e"},

		{rvars{true, "b"}, xvars{"", "", "f"}, trustXHeaders, "https://b/f"},
		{rvars{false, "b"}, xvars{"", "", "f"}, trustXHeaders, "http://b/f"},
	} {
		r := &http.Request{URL: &url.URL{}, Header: http.Header{}}
		if c.r.tls {
			r.TLS = &tls.ConnectionState{}
		}
		r.Host = c.r.host
		r.Header.Set("X-Forwarded-Proto", c.x.proto)
		r.Header.Set("X-Forwarded-Host", c.x.host)
		r.Header.Set("X-Forwarded-Prefix", c.x.prefix)

		actual := baseURLFromRequest(r, c.baseURLXHeaders).String()
		if actual != c.expect {
			t.Errorf("proto:%t:%s host:%s:%s prefix:%s trust:%d, got %v expected %v",
				c.r.tls, c.x.proto,
				c.r.host, c.x.host,
				c.x.prefix,
				c.baseURLXHeaders, actual, c.expect)
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

func ydlsHandlerFromEnv(t *testing.T) *Handler {
	h := &Handler{}
	var err error

	h.YDLS, err = NewFromFile(os.Getenv("CONFIG"))
	if err != nil {
		t.Fatalf("failed to read config: %s", err)
	}

	return h
}

func TestYDLSHandlerDownload(t *testing.T) {
	if !testNetwork || !testFfmpeg || !testYoutubedl {
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
