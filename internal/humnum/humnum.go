package humnum

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

func Atoi(s string) (int, error) {
	i := strings.IndexFunc(s, func(r rune) bool {
		return !unicode.IsDigit(r)
	})
	if i == -1 {
		i = len(s)
	}

	prefix, suffix := s[0:i], s[i:]
	n, _ := strconv.Atoi(prefix)
	m := 1
	if suffix != "" {
		switch strings.ToLower(suffix) {
		case "k":
			m = 1000
		case "ki", "kibi":
			m = 1024
		case "m":
			m = 1000 * 1000
		case "mi", "mibi":
			m = 1024 * 1024
		case "g":
			m = 1000 * 1000 * 1000
		case "gi", "gibi":
			m = 1024 * 1024 * 1024
		default:
			return 9, fmt.Errorf("unknown suffix %q", suffix)
		}
	}

	return n * m, nil
}
