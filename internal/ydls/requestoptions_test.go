package ydls

import "testing"

func TestNewRequestOptionsFromOpts(t *testing.T) {
	ydls := ydlsFromEnv(t)

	requestOptions, requestOptionsErr := NewRequestOptionsFromOpts(
		[]string{"mp4", "mp3", "h264", "retranscode", "10s-20s", "10items"},
		ydls.Config.Formats,
	)

	if requestOptionsErr != nil {
		t.Errorf("unexpected error: %s", requestOptionsErr)
	}
	if requestOptions.Format.Name != "mp4" {
		t.Errorf("expected format mp4, got %s", requestOptions.Format.Name)
	}
	if requestOptions.Codecs[0] != "mp3" || requestOptions.Codecs[1] != "h264" {
		t.Errorf("expected codecs mp4 h264, got %s", requestOptions.Codecs)
	}
	if !requestOptions.Retranscode {
		t.Errorf("expected retranscode")
	}
	if requestOptions.TimeRange.String() != "10s-20s" {
		t.Errorf("expected timerange 10s-20s, got %s", requestOptions.TimeRange.String())
	}
	if requestOptions.Items != 10 {
		t.Errorf("expected 10 items, got %d", requestOptions.Items)
	}

}
