package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// sinceUnitRe tokenizes a relative duration into number+unit pairs. Multi-char
// units are listed first so the alternation matches "mo" and "ms" before "m".
var sinceUnitRe = regexp.MustCompile(`([0-9]*\.?[0-9]+)(mo|ms|us|µs|ns|w|d|h|m|s|y)`)

// parseSince parses a relative lookback duration. It accepts everything
// time.ParseDuration does (h, m, s, ms, us, ns) and additionally supports day
// ("d"), week ("w"), month ("mo") and year ("y") units, which the standard
// library lacks. A month is a fixed 30 days and a year a fixed 365 days, so
// neither tracks the calendar. Components may be combined, e.g. "1w3d" or "36h".
func parseSince(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}
	// Without the extra units the stdlib handles it with full fidelity.
	if !strings.ContainsAny(strings.ToLower(s), "dwoy") {
		return time.ParseDuration(s)
	}

	matches := sinceUnitRe.FindAllStringSubmatch(s, -1)
	consumed := 0
	for _, m := range matches {
		consumed += len(m[0])
	}
	if len(matches) == 0 || consumed != len(s) {
		return 0, fmt.Errorf("invalid duration %q", s)
	}

	var total time.Duration
	for _, m := range matches {
		switch unit := strings.ToLower(m[2]); unit {
		case "d", "w", "mo", "y":
			val, err := strconv.ParseFloat(m[1], 64)
			if err != nil {
				return 0, fmt.Errorf("invalid duration %q: %w", s, err)
			}
			var span time.Duration
			switch unit {
			case "d":
				span = 24 * time.Hour
			case "w":
				span = 7 * 24 * time.Hour
			case "mo":
				span = 30 * 24 * time.Hour
			case "y":
				span = 365 * 24 * time.Hour
			}
			total += time.Duration(val * float64(span))
		default:
			// Hand standard units back to the stdlib for exact semantics.
			d, err := time.ParseDuration(m[1] + m[2])
			if err != nil {
				return 0, fmt.Errorf("invalid duration %q: %w", s, err)
			}
			total += d
		}
	}
	return total, nil
}
