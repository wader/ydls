package ydls

import "testing"

func TestNewDownloadOptionsFromOpts(t *testing.T) {
	ydls := ydlsFromEnv(t)

	downloadOptions, downloadOptionsErr := NewDownloadOptionsFromOpts(
		[]string{"mp4", "mp3", "h264", "retranscode", "10s-20s", "10items"},
		ydls.Config.Formats,
	)

	if downloadOptionsErr != nil {
		t.Errorf("unexpected error: %s", downloadOptionsErr)
	}
	if downloadOptions.Format.Name != "mp4" {
		t.Errorf("expected format mp4, got %s", downloadOptions.Format.Name)
	}
	if downloadOptions.Codecs[0] != "mp3" || downloadOptions.Codecs[1] != "h264" {
		t.Errorf("expected codecs mp4 h264, got %s", downloadOptions.Codecs)
	}
	if !downloadOptions.Retranscode {
		t.Errorf("expected retranscode")
	}
	if downloadOptions.TimeRange.String() != "10s-20s" {
		t.Errorf("expected timerange 10s-20s, got %s", downloadOptions.TimeRange.String())
	}
	if downloadOptions.Items != 10 {
		t.Errorf("expected 10 items, got %d", downloadOptions.Items)
	}

}
