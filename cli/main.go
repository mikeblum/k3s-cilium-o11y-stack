// Command otel-clickhouse-stack prints Claude Code usage summaries from the
// cluster's ClickHouse instance (otel database, otel_metrics_sum table).
//
// ClickHouse is not exposed outside the cluster, so the native port has to be
// forwarded first — see `make port-forward`.
//
// Human-readable tables and JSON both go to stdout; status lines, notices, and
// errors go to stderr, so `otel-clickhouse-stack --json | jq` streams cleanly.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// noticeTimeFormat is the minute-precision layout for the "since" notice.
const noticeTimeFormat = "2006-01-02 15:04"

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

// dimension is the token breakdown selected by the -by flag.
type dimension string

const (
	dimType  dimension = "type"
	dimModel dimension = "model"
)

type tokenRow struct {
	Day   time.Time `ch:"day" json:"day"`
	Dim   string    `ch:"dim" json:"dim"`
	Total float64   `ch:"total" json:"tokens"`
}

type costRow struct {
	Day   time.Time `ch:"day" json:"day"`
	Model string    `ch:"model" json:"model"`
	Cost  float64   `ch:"cost" json:"cost_usd"`
}

func main() {
	var (
		addr   = flag.String("addr", "localhost:9000", "ClickHouse native TCP address")
		user   = flag.String("user", defaultUser, "ClickHouse user")
		db     = flag.String("db", "otel", "ClickHouse database")
		days   = flag.Int("days", 7, "number of days to report, ending today")
		daily  = flag.Bool("daily", false, "report today only (shorthand for -days 1)")
		since  = flag.String("since", "", "relative lookback ending today, e.g. 1mo, 12w, 90d, 36h (alternative to -days)")
		by     = flag.String("by", string(dimType), "token breakdown dimension: type or model")
		tz     = flag.String("tz", localZone(), "IANA timezone for day boundaries, e.g. UTC or America/Chicago")
		asJSON = flag.Bool("json", false, "emit JSON to stdout instead of tables")
		showV  = flag.Bool("version", false, "print the version and exit")
	)
	flag.Parse()

	if *showV {
		fmt.Println(version)
		return
	}

	dim := dimension(*by)
	if dim != dimType && dim != dimModel {
		fail("usage", fmt.Errorf("-by must be %q or %q", dimType, dimModel))
	}

	// The window and the day buckets share one zone: TimeUnix is stored in UTC, so
	// grouping by the UTC day while cutting off at local midnight would bucket
	// late-evening usage into the following day.
	loc, err := time.LoadLocation(*tz)
	if err != nil {
		fail("tz", fmt.Errorf("unknown timezone %q", *tz))
	}

	cutoff, err := resolveWindow(*since, *days, isFlagSet("days"), *daily, time.Now().In(loc))
	if err != nil {
		fail("since", err)
	}

	pw, err := loadPassword(*user)
	if err != nil {
		fail("password", err)
	}

	// No deadline on the context: the driver turns one into a max_execution_time
	// setting on every query, and the readonly profile the grafana user runs
	// under refuses any setting change ("Cannot modify 'max_execution_time'
	// setting in readonly mode"). The timeouts below bound the work client side
	// instead, which needs nothing from the server.
	ctx := context.Background()

	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr:        []string{*addr},
		Auth:        clickhouse.Auth{Database: *db, Username: *user, Password: pw},
		Compression: &clickhouse.Compression{Method: clickhouse.CompressionLZ4},
		DialTimeout: 5 * time.Second,
		ReadTimeout: 30 * time.Second,
	})
	if err != nil {
		fail("connect", err)
	}
	defer conn.Close()

	if err := conn.Ping(ctx); err != nil {
		fail("ping (is `make port-forward` running?)", err)
	}

	tokens, err := queryTokens(ctx, conn, cutoff, dim, *tz)
	if err != nil {
		fail("tokens", err)
	}
	var byModel []tokenRow
	if dim != dimModel {
		byModel, err = queryTokens(ctx, conn, cutoff, dimModel, *tz)
		if err != nil {
			fail("tokens by model", err)
		}
	}
	costs, err := queryCosts(ctx, conn, cutoff, *tz)
	if err != nil {
		fail("cost", err)
	}

	fmt.Fprintf(os.Stderr, "Reporting usage since %s (%s)\n", cutoff.Format(noticeTimeFormat), *tz)

	if *asJSON {
		emitJSON(tokens, byModel, costs)
		return
	}
	printTokens(tokens, dim)
	if byModel != nil {
		fmt.Println()
		printTokens(byModel, dimModel)
	}
	fmt.Println()
	printCosts(costs)
}

// resolveWindow turns the mutually exclusive --since / --days / --daily flags
// into an absolute cutoff time (queries report usage at or after it). --since
// honors sub-day durations exactly; --days and --daily start at midnight of the
// first included day. daysSet reports whether --days was passed explicitly, so
// an explicit --days can conflict with --since while the default does not. now
// is injected for testability.
func resolveWindow(since string, days int, daysSet, daily bool, now time.Time) (cutoff time.Time, err error) {
	switch {
	case since != "" && (daysSet || daily):
		return time.Time{}, fmt.Errorf("--since is mutually exclusive with --days/--daily")
	case since != "":
		d, perr := parseSince(since)
		if perr != nil {
			return time.Time{}, fmt.Errorf("%w; use a duration such as 1mo, 12w, 90d, or 36h", perr)
		}
		if d <= 0 {
			return time.Time{}, fmt.Errorf("must be a positive duration, got %q", since)
		}
		return now.Add(-d), nil
	case daily:
		return midnight(now), nil
	default:
		if days < 1 {
			return time.Time{}, fmt.Errorf("--days must be >= 1")
		}
		return midnight(now).AddDate(0, 0, -(days - 1)), nil
	}
}

// midnight returns the start of now's day in its location.
func midnight(now time.Time) time.Time {
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
}

// localZone returns the host's IANA timezone name, falling back to UTC. The name
// itself is needed, not a *time.Location: ClickHouse resolves the zone server
// side, and time.Local reports only "Local", which it does not recognize. TZ wins
// when set; otherwise the /etc/localtime symlink carries the name on Linux and
// macOS.
func localZone() string {
	if tz := os.Getenv("TZ"); tz != "" {
		return tz
	}
	if p, err := os.Readlink("/etc/localtime"); err == nil {
		if _, name, ok := strings.Cut(p, "zoneinfo/"); ok {
			return name
		}
	}
	return "UTC"
}

func isFlagSet(name string) bool {
	set := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
}

func queryTokens(ctx context.Context, conn driver.Conn, cutoff time.Time, by dimension, tz string) ([]tokenRow, error) {
	q := fmt.Sprintf(`
		SELECT toDate(TimeUnix, ?) AS day, Attributes['%s'] AS dim, sum(Value) AS total
		FROM otel_metrics_sum
		WHERE MetricName = 'claude_code.token.usage'
		  AND TimeUnix >= ?
		GROUP BY day, dim
		ORDER BY day, dim`, by)

	var rows []tokenRow
	if err := conn.Select(ctx, &rows, q, tz, cutoff); err != nil {
		return nil, err
	}
	if by == dimModel {
		for i := range rows {
			rows[i].Dim = shortModel(rows[i].Dim)
		}
	}
	return rows, nil
}

func queryCosts(ctx context.Context, conn driver.Conn, cutoff time.Time, tz string) ([]costRow, error) {
	q := `
		SELECT toDate(TimeUnix, ?) AS day, Attributes['model'] AS model, sum(Value) AS cost
		FROM otel_metrics_sum
		WHERE MetricName = 'claude_code.cost.usage'
		  AND TimeUnix >= ?
		GROUP BY day, model
		ORDER BY day, cost DESC`

	var rows []costRow
	if err := conn.Select(ctx, &rows, q, tz, cutoff); err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].Model = shortModel(rows[i].Model)
	}
	return rows, nil
}

func printTokens(rows []tokenRow, by dimension) {
	label := strings.ToUpper(string(by))
	tw := newTable("TOKENS BY "+label, "DAY", label, "TOKENS")
	var grand float64
	for _, r := range rows {
		tw.row(r.Day.Format(time.DateOnly), r.Dim, comma(int64(r.Total)))
		grand += r.Total
	}
	tw.flush(len(rows), comma(int64(grand)))
}

func printCosts(rows []costRow) {
	tw := newTable("COST BY MODEL (USD)", "DAY", "MODEL", "COST")
	var grand float64
	for _, r := range rows {
		tw.row(r.Day.Format(time.DateOnly), r.Model, fmt.Sprintf("$%.4f", r.Cost))
		grand += r.Cost
	}
	tw.flush(len(rows), fmt.Sprintf("$%.4f", grand))
}

func emitJSON(tokens, tokensByModel []tokenRow, costs []costRow) {
	out := struct {
		Tokens        []tokenRow `json:"tokens"`
		TokensByModel []tokenRow `json:"tokens_by_model,omitempty"`
		Costs         []costRow  `json:"costs"`
	}{tokens, tokensByModel, costs}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		fail("json", err)
	}
}

func shortModel(m string) string {
	m = strings.TrimPrefix(m, "global.anthropic.")
	if i := strings.Index(m, "-2"); i > 0 { // trim date suffix like -20251001-v1:0
		m = m[:i]
	}
	return m
}

func fail(what string, err error) {
	fmt.Fprintf(os.Stderr, "error: %s: %v\n", what, err)
	os.Exit(1)
}
