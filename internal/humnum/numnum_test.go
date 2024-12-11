package humnum_test

import (
	"testing"

	"github.com/wader/ydls/internal/humnum"
)

func TestAtoi(t *testing.T) {
	testCases := []struct {
		s           string
		expectedN   int
		expectedErr string
	}{
		{s: "", expectedN: 0},
		{s: "1000", expectedN: 1000},
		{s: "1K", expectedN: 1000},
		{s: "1k", expectedN: 1000},
		{s: "1M", expectedN: 1000_000},
		{s: "1G", expectedN: 1000_000_000},
		{s: "1Ki", expectedN: 1024},
		{s: "1Mi", expectedN: 1024 * 1024},
		{s: "1Gi", expectedN: 1024 * 1024 * 1024},
		{s: "1Kibi", expectedN: 1024},
		{s: "1Mibi", expectedN: 1024 * 1024},
		{s: "1Gibi", expectedN: 1024 * 1024 * 1024},
		{s: "123abc", expectedErr: `unknown suffix "abc"`},
	}
	for _, tC := range testCases {
		t.Run(tC.s, func(t *testing.T) {
			actualN, actualErr := humnum.Atoi(tC.s)
			if (tC.expectedErr != "" && (actualErr == nil || tC.expectedErr != actualErr.Error())) && actualN != tC.expectedN {
				t.Errorf("expected %v (%v), got %v (%v)", tC.expectedN, tC.expectedErr, actualN, actualErr)
			}
		})
	}
}
