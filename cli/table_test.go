package main

import "testing"

func TestComma(t *testing.T) {
	cases := []struct {
		name string
		in   int64
		want string
	}{
		{name: "zero", in: 0, want: "0"},
		{name: "two digits", in: 42, want: "42"},
		{name: "three digits", in: 999, want: "999"},
		{name: "thousand", in: 1000, want: "1,000"},
		{name: "five digits", in: 12345, want: "12,345"},
		{name: "million", in: 1000000, want: "1,000,000"},
		{name: "negative", in: -1234567, want: "-1,234,567"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := comma(tc.in); got != tc.want {
				t.Errorf("comma(%d) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
