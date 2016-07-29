package main

import (
	"net/url"
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

func TestContentDispositionFilename(t *testing.T) {
	for _, c := range []struct {
		s      string
		expect string
	}{
		{" abcdefghijklmnopqruvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789", " abcdefghijklmnopqruvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"},
		{"SPÆCIAL", "SP_CIAL"},
		{"\\\"/", "___"},
	} {
		actual := contentDispositionFilename(c.s)
		if actual != c.expect {
			t.Errorf("%s, got %v expected %v", c.s, actual, c.expect)
		}
	}
}
