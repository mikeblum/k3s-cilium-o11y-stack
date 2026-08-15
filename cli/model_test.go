package main

import "testing"

func TestShortModel(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "opus", in: "global.anthropic.claude-opus-4-8", want: "claude-opus-4-8"},
		{name: "sonnet", in: "global.anthropic.claude-sonnet-5", want: "claude-sonnet-5"},
		{name: "haiku with date suffix", in: "global.anthropic.claude-haiku-4-5-20251001-v1:0", want: "claude-haiku-4-5"},
		{name: "no prefix", in: "claude-opus-4-8", want: "claude-opus-4-8"},
		{name: "empty", in: "", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shortModel(tc.in); got != tc.want {
				t.Errorf("shortModel(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
