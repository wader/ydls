package stringprioset

import (
	"encoding/json"
	"strings"
)

type Set struct {
	ss []string
}

func New(ss []string) Set {
	if len(ss) == 0 {
		return Set{}
	}

	u := map[string]bool{}

	for _, s := range ss[1:] {
		u[s] = true
	}

	n := []string{ss[0]}
	for s := range u {
		n = append(n, s)
	}

	return Set{n}
}

func (s Set) Member(a string) bool {
	for _, c := range s.ss {
		if c == a {
			return true
		}
	}
	return false
}

// does not preserve first property
func (s Set) Intersect(a Set) Set {
	counts := map[string]uint{}

	for _, s := range s.ss {
		counts[s] = counts[s] + 1
	}
	for _, s := range a.ss {
		counts[s] = counts[s] + 1
	}

	i := []string{}
	for s, c := range counts {
		if c == 2 {
			i = append(i, s)
		}
	}

	return New(i)
}

func (s Set) Empty() bool {
	return len(s.ss) == 0
}

func (s Set) First() (string, bool) {
	if len(s.ss) > 0 {
		return s.ss[0], true
	}
	return "", false
}

func (s Set) String() string {
	return "[" + strings.Join(s.ss, " ") + "]"
}

// need to be pointer type so value can be assigned
func (s *Set) UnmarshalJSON(b []byte) (err error) {
	var np []string
	err = json.Unmarshal(b, &np)
	*s = New(np)
	return
}
