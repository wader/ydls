package stringprioset

import (
	"encoding/json"
	"testing"
)

func TestMember(t *testing.T) {
	s := New([]string{"a", "b"})

	if !s.Member("a") {
		t.Error("expectd a to be a member")
	}
	if !s.Member("b") {
		t.Error("expectd a to be a member")
	}
	if s.Member("c") {
		t.Error("expectd c to not be a member")
	}
}

func TestIntersect(t *testing.T) {
	s1 := New([]string{"a", "b"})
	s2 := New([]string{"c", "b"})
	i1 := s1.Intersect(s2)
	i2 := s2.Intersect(s1)

	for _, s := range []Set{i1, i2} {
		if !s.Member("b") {
			t.Error("expectd b to be a member")
		}
		if s.Member("c") {
			t.Error("expectd c to not be a member")
		}
	}
}

func TestEmpty(t *testing.T) {
	e := New([]string{})
	s := New([]string{"a"})

	if !e.Empty() {
		t.Error("expectd e to be empty")
	}
	if s.Empty() {
		t.Error("expectd s to not be empty")
	}
}

func TestFirst(t *testing.T) {
	s := New([]string{"a", "b"})
	e := New([]string{})

	if v, ok := s.First(); !ok || v != "a" {
		t.Error("expectd a to be first")
	}

	if v, ok := e.First(); ok || v != "" {
		t.Error("expectd first to be empty")
	}
}

func TestString(t *testing.T) {
	s := New([]string{"a", "b"})

	if v := s.String(); v != "[a b]" {
		t.Errorf("expectd String to be [a b], got %v", v)
	}
}

func TestUnmarshalJSON(t *testing.T) {
	s := Set{}
	if err := json.Unmarshal([]byte(`["a", "b"]`), &s); err != nil {
		t.Fatal(err)
	}

	if !s.Member("a") {
		t.Error("expectd a to be a member")
	}
	if !s.Member("b") {
		t.Error("expectd a to be a member")
	}
	if s.Member("c") {
		t.Error("expectd c to not be a member")
	}
}
