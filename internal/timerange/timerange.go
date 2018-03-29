package timerange

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// TimeRange start/stop duration
type TimeRange struct {
	Start time.Duration
	Stop  time.Duration
}

// NewFromString create new TimeRange from string representation
func NewFromString(s string) (TimeRange, error) {
	return parseDurationRange(s)
}

// N or NhNmNs
var parseDurationReN = regexp.MustCompile(`^\d+$`)
var parseDurationReMix = regexp.MustCompile(`^(?:(\d+)h)?(?:(\d+)m)?(?:(\d+)s)?$`)

func parseDuration(s string) (time.Duration, error) {
	if len(s) == 0 {
		return 0, fmt.Errorf("Could not parse duration")
	}

	if parseDurationReN.MatchString(s) {
		n, _ := strconv.Atoi(s)
		return time.Second * time.Duration(n), nil
	}

	matchesMix := parseDurationReMix.FindStringSubmatch(s)
	if matchesMix == nil {
		return 0, fmt.Errorf("Could not parse duration")
	}

	ih, _ := strconv.Atoi(matchesMix[1])
	im, _ := strconv.Atoi(matchesMix[2])
	is, _ := strconv.Atoi(matchesMix[3])

	return (0 +
		time.Hour*time.Duration(ih) +
		time.Minute*time.Duration(im) +
		time.Second*time.Duration(is) +
		0), nil
}

func parseDurationRange(s string) (TimeRange, error) {
	parts := strings.Split(s, "-")
	if len(parts) == 1 {
		stop, stopErr := parseDuration(parts[0])
		if stopErr != nil {
			return TimeRange{}, stopErr
		}
		return TimeRange{Stop: stop}, nil
	}

	var tr TimeRange
	var err error

	if tr.Start, err = parseDuration(parts[0]); err != nil {
		return TimeRange{}, err
	}
	if tr.Stop, err = parseDuration(parts[1]); err != nil {
		return TimeRange{}, err
	}

	if tr.Start > tr.Stop {
		return TimeRange{}, fmt.Errorf("Start after stop")
	}

	return tr, nil
}

// IsZero is start and stop zero
func (tr TimeRange) IsZero() bool {
	return tr.Start == 0 && tr.Stop == 0
}

// Duration duration between start and stop
func (tr TimeRange) Duration() time.Duration {
	return tr.Stop - tr.Start
}
