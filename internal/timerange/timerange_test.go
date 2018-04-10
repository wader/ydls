package timerange

import (
	"testing"
	"time"
)

func TestIsZero(t *testing.T) {
	if !(TimeRange{}).IsZero() {
		t.Errorf("Expected IsZero not true for TimeRange{}")
	}
}

func TestParseDuration(t *testing.T) {
	for _, c := range []struct {
		s              string
		expected       Duration
		expectedString string
		expectedErr    bool
	}{
		{"", 0, "", true},
		{"a", 0, "", true},

		{"1", Duration(time.Second * 1), "1s", false},
		{"12", Duration(time.Second * 12), "12s", false},
		{"123", Duration(time.Second * 123), "2m3s", false},

		{"1s", Duration(time.Second * 1), "1s", false},
		{"12s", Duration(time.Second * 12), "12s", false},
		{"123s", Duration(time.Second * 123), "2m3s", false},

		{"1m", Duration(time.Minute * 1), "1m", false},
		{"12m", Duration(time.Minute * 12), "12m", false},
		{"123m", Duration(time.Minute * 123), "2h3m", false},

		{"1h", Duration(time.Hour * 1), "1h", false},
		{"12h", Duration(time.Hour * 12), "12h", false},
		{"123h", Duration(time.Hour * 123), "123h", false},

		{"1h2m3s", Duration(time.Hour*1 + time.Minute*2 + time.Second*3), "1h2m3s", false},
		{"1h3s", Duration(time.Hour*1 + time.Second*3), "1h3s", false},
		{"1h2m", Duration(time.Hour*1 + time.Minute*2), "1h2m", false},
		{"2m3s", Duration(time.Minute*2 + time.Second*3), "2m3s", false},
	} {
		actual, actualErr := NewDurationFromString(c.s)
		if c.expectedErr && actualErr == nil {
			t.Errorf("%s, expected error", c.s)
		}

		if actual != c.expected {
			t.Errorf("%s, got %v expected %v", c.s, actual, c.expected)
		}

		if actual.String() != c.expectedString {
			t.Errorf("%s, got %v expected %v", c.s, actual.String(), c.expectedString)
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

		{"10", TimeRange{0, Duration(time.Second * 10)}, time.Second * 10, false},
		{"10s", TimeRange{0, Duration(time.Second * 10)}, time.Second * 10, false},

		{"10s-20s", TimeRange{Duration(time.Second * 10), Duration(time.Second * 20)}, time.Second * 10, false},
		{"10s-10s", TimeRange{Duration(time.Second * 10), Duration(time.Second * 10)}, 0, false},

		{"10s-9s", TimeRange{0, 0}, 0, true},
	} {
		actualTr, actualErr := NewTimeRangeFromString(c.s)
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
