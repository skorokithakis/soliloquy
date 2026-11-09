package auth

import (
	"strings"
	"testing"
)

func TestValidateUsername(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
		ok    bool
	}{
		{name: "lowercase", input: "alice", want: "alice", ok: true},
		{name: "uppercase normalized", input: "ALICE", want: "alice", ok: true},
		{name: "surrounding space", input: "  bob_1  ", want: "bob_1", ok: true},
		{name: "allowed punctuation", input: "a-b_c9", want: "a-b_c9", ok: true},
		{name: "too short", input: "ab", ok: false},
		{name: "too long", input: strings.Repeat("a", 21), ok: false},
		{name: "space inside", input: "has space", ok: false},
		{name: "dot", input: "dot.name", ok: false},
		{name: "at sign", input: "a@b", ok: false},
		{name: "empty", input: "", ok: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ValidateUsername(test.input)
			if test.ok && err != nil {
				t.Fatalf("ValidateUsername(%q) error = %v", test.input, err)
			}
			if !test.ok && err == nil {
				t.Fatalf("ValidateUsername(%q) accepted invalid name", test.input)
			}
			if test.ok && got != test.want {
				t.Errorf("ValidateUsername(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}
