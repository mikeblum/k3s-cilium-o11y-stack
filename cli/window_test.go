package main

import (
	"strings"
	"testing"
	"time"
)

// TestResolveWindow covers window resolution end to end: the --since durations
// (including the day, week, month and year units the stdlib lacks), the
// --days/--daily paths that start at midnight, mutual exclusion, and invalid
// input. Cases with a loc pin the cutoff to the caller's zone, which must match
// the zone the day buckets are grouped in.
func TestResolveWindow(t *testing.T) {
	// A nonzero time-of-day exercises the midnight vs. exact-cutoff distinction.
	// 00:30 UTC is still the previous evening in Chicago, the window where
	// UTC-day and local-day grouping disagree.
	now := time.Date(2026, 7, 22, 0, 30, 0, 0, time.UTC)
	mid := time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC)

	chicago, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}

	cases := []struct {
		name       string
		since      string
		days       int
		daysSet    bool
		daily      bool
		loc        *time.Location // nil means use now as-is (UTC)
		wantCutoff time.Time      // zero means don't check (error case)
		wantErr    string         // substring; "" means no error
	}{
		{name: "since days", since: "90d", wantCutoff: now.Add(-90 * 24 * time.Hour)},
		{name: "since one day", since: "1d", wantCutoff: now.Add(-24 * time.Hour)},
		{name: "since weeks", since: "12w", wantCutoff: now.Add(-12 * 7 * 24 * time.Hour)},
		{name: "since months", since: "1mo", wantCutoff: now.Add(-30 * 24 * time.Hour)},
		{name: "since several months", since: "3mo", wantCutoff: now.Add(-3 * 30 * 24 * time.Hour)},
		{name: "since years", since: "1y", wantCutoff: now.Add(-365 * 24 * time.Hour)},
		{name: "since mixed units", since: "1mo15d", wantCutoff: now.Add(-(30 + 15) * 24 * time.Hour)},
		{name: "since weeks and days", since: "1w3d", wantCutoff: now.Add(-(7 + 3) * 24 * time.Hour)},
		{name: "since days and hours", since: "1d12h", wantCutoff: now.Add(-36 * time.Hour)},
		{name: "since fractional days", since: "0.5d", wantCutoff: now.Add(-12 * time.Hour)},
		{name: "since honors sub-day", since: "36h", wantCutoff: now.Add(-36 * time.Hour)},
		{name: "since minutes", since: "90m", wantCutoff: now.Add(-90 * time.Minute)},
		{name: "since one minute", since: "1m", wantCutoff: now.Add(-time.Minute)},
		{name: "since milliseconds", since: "1ms", wantCutoff: now.Add(-time.Millisecond)},
		{name: "since is trimmed", since: " 90d ", wantCutoff: now.Add(-90 * 24 * time.Hour)},
		{name: "since is zone-independent", since: "36h", loc: chicago, wantCutoff: now.Add(-36 * time.Hour)},
		{name: "since unitless", since: "90", wantErr: `missing unit in duration "90"`},
		{name: "since unknown unit", since: "90x", wantErr: `unknown unit "x"`},
		{name: "since not a duration", since: "banana", wantErr: `invalid duration "banana"`},
		{name: "since error suggests units", since: "banana", wantErr: "use a duration such as 1mo"},
		{name: "since blank", since: "   ", wantErr: "empty duration"},
		{name: "since doubled unit", since: "90dd", wantErr: `invalid duration "90dd"`},
		{name: "since unit first", since: "d90", wantErr: `invalid duration "d90"`},
		{name: "since transposed month", since: "1om", wantErr: `invalid duration "1om"`},
		{name: "since doubled year", since: "1yy", wantErr: `invalid duration "1yy"`},
		{name: "since zero", since: "0d", wantErr: "positive duration"},
		{name: "since and explicit days conflict", since: "5d", days: 5, daysSet: true, wantErr: "mutually exclusive"},
		{name: "since and daily conflict", since: "5d", daily: true, wantErr: "mutually exclusive"},
		{name: "since does not conflict with default days", since: "5d", days: 7, wantCutoff: now.Add(-5 * 24 * time.Hour)},
		{name: "daily is midnight today", daily: true, wantCutoff: mid},
		{name: "daily is local midnight", daily: true, loc: chicago, wantCutoff: time.Date(2026, 7, 21, 0, 0, 0, 0, chicago)},
		{name: "default days from midnight", days: 7, wantCutoff: mid.AddDate(0, 0, -6)},
		{name: "days unclamped", days: 200, daysSet: true, wantCutoff: mid.AddDate(0, 0, -199)},
		{name: "days below one", days: 0, daysSet: true, wantErr: "--days must be >= 1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ref := now
			if tc.loc != nil {
				ref = now.In(tc.loc)
			}
			cutoff, err := resolveWindow(tc.since, tc.days, tc.daysSet, tc.daily, ref)

			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %q, want substring %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !cutoff.Equal(tc.wantCutoff) {
				t.Errorf("cutoff = %v, want %v", cutoff, tc.wantCutoff)
			}
		})
	}
}

// TestLocalZone checks TZ takes precedence and that the fallback yields a name
// ClickHouse can resolve. It cannot prove the "Local" case is avoided: time.Local
// is resolved once per process, so setting TZ here fixes it for the whole run.
func TestLocalZone(t *testing.T) {
	t.Setenv("TZ", "America/New_York")
	if got := localZone(); got != "America/New_York" {
		t.Errorf("localZone() = %q, want TZ value", got)
	}

	t.Setenv("TZ", "")
	got := localZone()
	if got == "Local" || got == "" {
		t.Fatalf("localZone() = %q, want an IANA name or UTC", got)
	}
	if _, err := time.LoadLocation(got); err != nil {
		t.Errorf("localZone() = %q, not loadable: %v", got, err)
	}
}
