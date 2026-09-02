package service

import "testing"

func TestBodyChars(t *testing.T) {
	cases := map[string]int{
		"":                                      0,
		"# test\n\ntest\n":                      4,
		"---\ntitle: x\narea: work\n---\n# x\n": 0,
		"# T\n\n" + "a long enough paragraph about something real that a vector can hold on to.\n": 74,
	}
	for in, want := range cases {
		if got := bodyChars([]byte(in)); got != want {
			t.Errorf("bodyChars(%q) = %d, want %d", in, got, want)
		}
	}
}
