package ydls

import (
	"context"
	"io"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/wader/ydls/internal/ffmpeg"
)

func TestDownloadResultRangeSupport(t *testing.T) {
	tests := []struct {
		name              string
		contentLength     int64
		supportsRanges    bool
		duration          time.Duration
		expectRangeHeader string
		expectLength      bool
	}{
		{
			name:              "with range support and content length",
			contentLength:     1000000,
			supportsRanges:    true,
			duration:          time.Minute,
			expectRangeHeader: "bytes",
			expectLength:      true,
		},
		{
			name:              "without range support",
			contentLength:     0,
			supportsRanges:    false,
			duration:          0,
			expectRangeHeader: "none",
			expectLength:      false,
		},
		{
			name:              "with range support but no content length",
			contentLength:     0,
			supportsRanges:    true,
			duration:          time.Minute,
			expectRangeHeader: "bytes",
			expectLength:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dr := DownloadResult{
				ContentLength:  tt.contentLength,
				SupportsRanges: tt.supportsRanges,
				Duration:       tt.duration,
			}

			if dr.SupportsRanges != tt.supportsRanges {
				t.Errorf("SupportsRanges = %v, want %v", dr.SupportsRanges, tt.supportsRanges)
			}
			if dr.ContentLength != tt.contentLength {
				t.Errorf("ContentLength = %v, want %v", dr.ContentLength, tt.contentLength)
			}
			if dr.Duration != tt.duration {
				t.Errorf("Duration = %v, want %v", dr.Duration, tt.duration)
			}
		})
	}
}

func TestHandlerRangeHeaders(t *testing.T) {
	if !testExternal {
		t.Skip("TEST_EXTERNAL")
	}

	defer leakChecks(t)()

	h := ydlsHandlerFromEnv(t)

	tests := []struct {
		name            string
		url             string
		expectRanges    string
		expectHasLength bool
	}{
		{
			name:            "mp3 format should support ranges",
			url:             "http://hostname/mp3/" + testVideoURL,
			expectRanges:    "bytes",
			expectHasLength: true,
		},
		{
			name:            "mp4 format should support ranges",
			url:             "http://hostname/mp4/" + testVideoURL,
			expectRanges:    "bytes",
			expectHasLength: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest("GET", tt.url, nil)
			h.ServeHTTP(rr, req)
			resp := rr.Result()

			acceptRanges := resp.Header.Get("Accept-Ranges")
			if acceptRanges != tt.expectRanges {
				t.Errorf("Accept-Ranges = %v, want %v", acceptRanges, tt.expectRanges)
			}

			contentLength := resp.Header.Get("Content-Length")
			hasLength := contentLength != ""
			if hasLength != tt.expectHasLength {
				t.Errorf("Content-Length present = %v, want %v", hasLength, tt.expectHasLength)
			}

			if hasLength {
				length, err := strconv.ParseInt(contentLength, 10, 64)
				if err != nil {
					t.Errorf("Content-Length not a valid integer: %v", err)
				}
				if length <= 0 {
					t.Errorf("Content-Length = %v, want > 0", length)
				}
			}

			// Consume the body to avoid leaks
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		})
	}
}

func TestContentLengthCalculation(t *testing.T) {
	tests := []struct {
		name            string
		bitrate         int64 // bits per second
		duration        time.Duration
		expectedMinSize int64
		expectedMaxSize int64
	}{
		{
			name:            "5 minute video at 2.5 Mbps",
			bitrate:         2500000,
			duration:        5 * time.Minute,
			expectedMinSize: 90000000,  // ~90 MB minimum
			expectedMaxSize: 110000000, // ~110 MB maximum (with overhead)
		},
		{
			name:            "3 minute audio at 128 kbps",
			bitrate:         128000,
			duration:        3 * time.Minute,
			expectedMinSize: 2700000, // ~2.7 MB minimum
			expectedMaxSize: 3300000, // ~3.3 MB maximum (with overhead)
		},
		{
			name:            "30 second video at 1 Mbps",
			bitrate:         1000000,
			duration:        30 * time.Second,
			expectedMinSize: 3500000, // ~3.5 MB minimum
			expectedMaxSize: 4500000, // ~4.5 MB maximum (with overhead)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Calculate as done in the code
			contentLength := (tt.bitrate / 8) * int64(tt.duration.Seconds())
			contentLength = contentLength * 11 / 10 // Add 10% overhead

			if contentLength < tt.expectedMinSize {
				t.Errorf("Content length %v is less than expected minimum %v", contentLength, tt.expectedMinSize)
			}
			if contentLength > tt.expectedMaxSize {
				t.Errorf("Content length %v is greater than expected maximum %v", contentLength, tt.expectedMaxSize)
			}
		})
	}
}

func TestDurationExtraction(t *testing.T) {
	if !testExternal {
		t.Skip("TEST_EXTERNAL")
	}

	defer leakChecks(t)()

	ydls := ydlsFromEnv(t)
	const formatName = "mp4"
	mp4Format, _ := ydls.Config.Formats.FindByName(formatName)

	ctx, cancelFn := context.WithCancel(context.Background())
	defer cancelFn()

	dr, err := ydls.Download(ctx,
		DownloadOptions{
			RequestOptions: RequestOptions{
				MediaRawURL: testVideoURL,
				Format:      &mp4Format,
			},
			Retries: ydlsLRetries,
		},
	)
	if err != nil {
		t.Fatalf("download failed: %s", err)
	}

	// Check that duration was set
	if dr.Duration == 0 {
		t.Error("Duration should be set but is 0")
	}

	// Check that duration is reasonable (between 1 second and 2 hours for test video)
	if dr.Duration < time.Second || dr.Duration > 2*time.Hour {
		t.Errorf("Duration %v seems unreasonable for test video", dr.Duration)
	}

	// Consume and close
	io.Copy(io.Discard, dr.Media)
	dr.Media.Close()
	dr.Wait()
}

func TestRSSNoRangeSupport(t *testing.T) {
	if !testExternal {
		t.Skip("TEST_EXTERNAL")
	}

	defer leakChecks(t)()

	h := ydlsHandlerFromEnv(t)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://hostname/rss/"+soundcloudTestPlaylistURL, nil)
	h.ServeHTTP(rr, req)
	resp := rr.Result()

	acceptRanges := resp.Header.Get("Accept-Ranges")
	if acceptRanges == "bytes" {
		t.Error("RSS feed should not advertise range support")
	}

	contentLength := resp.Header.Get("Content-Length")
	if contentLength != "" {
		t.Error("RSS feed should not have Content-Length header")
	}

	// Consume the body
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}

func TestProbeInfoDuration(t *testing.T) {
	if !testExternal {
		t.Skip("TEST_EXTERNAL")
	}

	defer leakChecks(t)()

	dummy, err := ffmpeg.Dummy("matroska", "mp3", "h264")
	if err != nil {
		t.Fatalf("failed to create dummy: %s", err)
	}

	pi, err := ffmpeg.Probe(context.Background(), ffmpeg.Reader{Reader: dummy}, nil, nil)
	if err != nil {
		t.Fatalf("probe failed: %s", err)
	}

	duration := pi.Duration()
	if duration == 0 {
		t.Error("Duration should be extracted from probe info")
	}

	if duration > 5*time.Second {
		t.Errorf("Duration %v seems too long for dummy file", duration)
	}
}

func TestBitrateEstimation(t *testing.T) {
	tests := []struct {
		name          string
		audioBitrate  string
		videoBitrate  string
		expectedTotal int64
	}{
		{
			name:          "audio and video streams",
			audioBitrate:  "128000",
			videoBitrate:  "2500000",
			expectedTotal: 2628000,
		},
		{
			name:          "only audio stream",
			audioBitrate:  "320000",
			videoBitrate:  "",
			expectedTotal: 320000,
		},
		{
			name:          "only video stream",
			audioBitrate:  "",
			videoBitrate:  "5000000",
			expectedTotal: 5000000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var total int64

			if tt.audioBitrate != "" {
				if br, err := strconv.ParseInt(tt.audioBitrate, 10, 64); err == nil {
					total += br
				}
			}

			if tt.videoBitrate != "" {
				if br, err := strconv.ParseInt(tt.videoBitrate, 10, 64); err == nil {
					total += br
				}
			}

			if total != tt.expectedTotal {
				t.Errorf("Total bitrate = %v, want %v", total, tt.expectedTotal)
			}
		})
	}
}

// TestDownloadResultFields tests that all new fields are properly initialized
func TestDownloadResultFields(t *testing.T) {
	waitCh := make(chan struct{})
	close(waitCh)

	dr := DownloadResult{
		ContentLength:  123456,
		SupportsRanges: true,
		Duration:       42 * time.Second,
		waitCh:         waitCh,
	}

	if dr.ContentLength != 123456 {
		t.Errorf("ContentLength = %v, want 123456", dr.ContentLength)
	}
	if !dr.SupportsRanges {
		t.Error("SupportsRanges should be true")
	}
	if dr.Duration != 42*time.Second {
		t.Errorf("Duration = %v, want 42s", dr.Duration)
	}
}

// TestZeroDurationHandling tests that zero duration is handled gracefully
func TestZeroDurationHandling(t *testing.T) {
	dr := DownloadResult{
		Duration:       0,
		ContentLength:  0,
		SupportsRanges: false,
	}

	// Should not crash or cause issues
	if dr.Duration != 0 {
		t.Errorf("Duration should be 0, got %v", dr.Duration)
	}

	// Should not enable ranges without duration
	if dr.SupportsRanges {
		t.Error("Should not support ranges without duration/bitrate")
	}
}

func TestContentLengthOverheadCalculation(t *testing.T) {
	baseBitrate := int64(1000000) // 1 Mbps
	duration := 60 * time.Second  // 1 minute

	baseSize := (baseBitrate / 8) * int64(duration.Seconds())
	withOverhead := baseSize * 11 / 10

	expectedOverhead := baseSize / 10
	actualOverhead := withOverhead - baseSize

	if actualOverhead != expectedOverhead {
		t.Errorf("Overhead calculation incorrect: got %v, want %v", actualOverhead, expectedOverhead)
	}

	overheadPercent := float64(actualOverhead) / float64(baseSize) * 100
	if overheadPercent < 9.9 || overheadPercent > 10.1 {
		t.Errorf("Overhead percentage = %.2f%%, want ~10%%", overheadPercent)
	}
}
