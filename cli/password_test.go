package main

import (
	"strings"
	"testing"
)

func TestSecretKey(t *testing.T) {
	cases := []struct {
		name    string
		user    string
		want    string
		wantErr string // substring; "" means no error
	}{
		{name: "grafana", user: "grafana", want: "GRAFANA_PASSWORD"},
		{name: "otel writer", user: "otel_writer", want: "OTEL_WRITER_PASSWORD"},
		{name: "default user is localhost only", user: "default", wantErr: "set CLICKHOUSE_PASSWORD"},
		{name: "unknown", user: "nobody", wantErr: `user "nobody"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := secretKey(tc.user)
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
			if got != tc.want {
				t.Errorf("secretKey(%q) = %q, want %q", tc.user, got, tc.want)
			}
		})
	}
}

// TestLoadPasswordEnvWins pins the precedence: with CLICKHOUSE_PASSWORD set,
// no kubectl call happens, so an unknown user resolves fine.
func TestLoadPasswordEnvWins(t *testing.T) {
	t.Setenv("CLICKHOUSE_PASSWORD", "s3cret")

	for _, user := range []string{defaultUser, "nobody"} {
		got, err := loadPassword(user)
		if err != nil {
			t.Fatalf("loadPassword(%q): %v", user, err)
		}
		if got != "s3cret" {
			t.Errorf("loadPassword(%q) = %q, want the env value", user, got)
		}
	}
}

// TestLoadPasswordUnknownUser checks the no-env path fails on the mapping
// rather than shelling out to kubectl for a key that cannot exist.
func TestLoadPasswordUnknownUser(t *testing.T) {
	t.Setenv("CLICKHOUSE_PASSWORD", "")

	if _, err := loadPassword("nobody"); err == nil {
		t.Fatal("expected an error for an unmapped user")
	}
}
