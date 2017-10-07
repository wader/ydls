package timerange

import (
	"testing"
	"time"
)

func TestIsZero(t *testing.T) {
	if !IsZero(TimeRange{}) {
		t.Errorf("Expected IsZero not true for TimeRange{}")
	}
}

func TestParseDuration(t *testing.T) {
	for _, c := range []struct {
		s           string
		expected    time.Duration
		expectedErr bool
	}{
		{"", 0, true},
		{"a", 0, true},

		{"1", time.Second * 1, false},
		{"12", time.Second * 12, false},
		{"123", time.Second * 123, false},

		{"1s", time.Second * 1, false},
		{"12s", time.Second * 12, false},
		{"123s", time.Second * 123, false},

		{"1m", time.Minute * 1, false},
		{"12m", time.Minute * 12, false},
		{"123m", time.Minute * 123, false},

		{"1h", time.Hour * 1, false},
		{"12h", time.Hour * 12, false},
		{"123h", time.Hour * 123, false},

		{"1h2m3s", time.Hour*1 + time.Minute*2 + time.Second*3, false},
		{"1h3s", time.Hour*1 + time.Second*3, false},
		{"1h2m", time.Hour*1 + time.Minute*2, false},
		{"2m3s", time.Minute*2 + time.Second*3, false},
	} {
		actual, actualErr := parseDuration(c.s)
		if c.expectedErr && actualErr == nil {
			t.Errorf("%s, expected error", c.s)
		}

		if actual != c.expected {
			t.Errorf("%s, got %v expected %v", c.s, actual, c.expected)
		}
	}
}

func TestParseDurationRange(t *testing.T) {
	for _, c := range []struct {
		s                string
		expectedTr       TimeRange
		expectedDuration time.Duration
		expectedErr      bool
	}{
		{"", TimeRange{0, 0}, 0, true},
		{"a", TimeRange{0, 0}, 0, true},
		{"-a-a-", TimeRange{0, 0}, 0, true},

		{"10", TimeRange{0, time.Second * 10}, time.Second * 10, false},
		{"10s", TimeRange{0, time.Second * 10}, time.Second * 10, false},

		{"10s-20s", TimeRange{time.Second * 10, time.Second * 20}, time.Second * 10, false},
		{"10s-10s", TimeRange{time.Second * 10, time.Second * 10}, 0, false},

		{"10s-9s", TimeRange{0, 0}, 0, true},
	} {
		actualTr, actualErr := parseDurationRange(c.s)
		if c.expectedErr && actualErr == nil {
			t.Errorf("%s, expected error", c.s)
		} else {
			if actualTr.Start != c.expectedTr.Start || actualTr.Stop != c.expectedTr.Stop {
				t.Errorf("%s, got %v-%v expected %v-%v", c.s, actualTr.Start, actualTr.Stop, c.expectedTr.Start, c.expectedTr.Stop)
			}

			if actualTr.Duration() != c.expectedDuration {
				t.Errorf("%s, got duration %s expected %s", c.s, actualTr.Duration(), actualTr.Duration())
			}
		}
	}
}
