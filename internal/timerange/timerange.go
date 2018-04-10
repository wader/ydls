package timerange

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Duration time.Duration

var parseDurationReN = regexp.MustCompile(`^\d+$`)                                 // N
var parseDurationReMix = regexp.MustCompile(`^(?:(\d+)h)?(?:(\d+)m)?(?:(\d+)s)?$`) // NhNmNs

// NewDurationFromString create new Duration from string representation
func NewDurationFromString(s string) (Duration, error) {
	if len(s) == 0 {
		return 0, fmt.Errorf("could not parse duration")
	}

	if parseDurationReN.MatchString(s) {
		n, _ := strconv.Atoi(s)
		return Duration(time.Second * time.Duration(n)), nil
	}

	matchesMix := parseDurationReMix.FindStringSubmatch(s)
	if matchesMix == nil {
		return 0, fmt.Errorf("could not parse duration")
	}

	ih, _ := strconv.Atoi(matchesMix[1])
	im, _ := strconv.Atoi(matchesMix[2])
	is, _ := strconv.Atoi(matchesMix[3])

	return Duration(0 +
		time.Hour*time.Duration(ih) +
		time.Minute*time.Duration(im) +
		time.Second*time.Duration(is) +
		0), nil
}

func (d Duration) IsZero() bool {
	return time.Duration(d) == 0
}

func (d Duration) String() string {
	n := uint64(time.Duration(d).Seconds())

	s := n % 60
	n /= 60
	m := n % 60
	n /= 60
	h := n

	parts := []string{}
	if h > 0 {
		parts = append(parts, strconv.Itoa(int(h)), "h")
	}
	if m > 0 {
		parts = append(parts, strconv.Itoa(int(m)), "m")
	}
	if s > 0 {
		parts = append(parts, strconv.Itoa(int(s)), "s")
	}

	return strings.Join(parts, "")
}

// TimeRange start/stop duration
type TimeRange struct {
	Start Duration
	Stop  Duration
}

// NewTimeRangeFromString create new TimeRange from string representation
func NewTimeRangeFromString(s string) (TimeRange, error) {
	parts := strings.Split(s, "-")
	if len(parts) == 1 {
		stop, stopErr := NewDurationFromString(parts[0])
		if stopErr != nil {
			return TimeRange{}, stopErr
		}
		return TimeRange{Stop: stop}, nil
	}

	var tr TimeRange
	var err error

	if tr.Start, err = NewDurationFromString(parts[0]); err != nil {
		return TimeRange{}, err
	}
	if tr.Stop, err = NewDurationFromString(parts[1]); err != nil {
		return TimeRange{}, err
	}

	if tr.Start > tr.Stop {
		return TimeRange{}, fmt.Errorf("start after stop")
	}

	return tr, nil
}

// IsZero is start and stop zero
func (tr TimeRange) IsZero() bool {
	return tr.Start == 0 && tr.Stop == 0
}

// Duration duration between start and stop
func (tr TimeRange) Duration() time.Duration {
	return time.Duration(tr.Stop) - time.Duration(tr.Start)
}

func (tr TimeRange) String() string {
	if tr.Start.IsZero() {
		return tr.Stop.String()
	} else {
		if tr.Stop.IsZero() {
			return tr.Start.String() + "-"
		} else {
			return tr.Start.String() + "-" + tr.Stop.String()
		}
	}
}
