# cli — Claude Code spend reporting

A small Go CLI that prints Claude Code token and cost summaries straight from
the cluster's ClickHouse instance — no Grafana or DB browser needed. It reads
the `otel.otel_metrics_sum` table the OTel Collector writes
(see [../k8s/o11y/CLAUDE-CODE-TELEMETRY.md](../k8s/o11y/CLAUDE-CODE-TELEMETRY.md)
for pointing Claude Code at the collector).

## Connecting

ClickHouse has no Gateway route — nothing outside the cluster can reach it — so
forward the native TCP port and leave it running in another shell:

```bash
make port-forward          # or, from the repo root: make cli-port-forward
```

The password comes from the `clickhouse-auth` Secret in the `o11y` namespace,
resolved through `kubectl` with whatever context is current, so there is no
password to copy into the working tree. `CLICKHOUSE_PASSWORD` overrides it:

```bash
CLICKHOUSE_PASSWORD=… go run . --daily     # skips the kubectl lookup
```

Queries run as the read-only `grafana` user by default. `--user otel_writer`
also resolves from the Secret; any other user needs `CLICKHOUSE_PASSWORD`, since
the cluster has nothing to look up for it.

## Usage

```bash
cd cli
go run . --daily             # today: tokens by type + cost by model
go run . --days 7            # last 7 days
go run . --since 2w          # relative lookback (1mo, 12w, 90d, 36h, 1w3d)
go run . --daily --by model  # tokens grouped by model instead of type
go run . --days 7 --tz UTC   # group days by UTC instead of the host timezone
go run . --since 30d --json | jq '.tokens'   # pipe to jq
```

`make install` puts `otel-clickhouse-stack` on `PATH`, so the same flags work
from anywhere once the port-forward is up.

## Example

```
$ go run . --daily
Reporting usage since 2026-08-15 00:00 (America/Chicago)
== TOKENS BY TYPE ==
DAY         TYPE           TOKENS
2026-08-15  cacheCreation  406,019
2026-08-15  cacheRead      9,344,396
2026-08-15  input          4,319
2026-08-15  output         64,370
TOTAL                      9,819,104

== TOKENS BY MODEL ==
DAY         MODEL             TOKENS
2026-08-15  claude-haiku-4-5  4,323
2026-08-15  claude-opus-4-8   9,814,781
TOTAL                         9,819,104

== COST BY MODEL (USD) ==
DAY         MODEL             COST
2026-08-15  claude-opus-4-8   $8.8131
2026-08-15  claude-haiku-4-5  $0.0055
TOTAL                         $8.8186
```

The `TOKENS BY MODEL` section is added automatically unless `--by model` is
given (which already breaks the first table down by model).

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--days N` | 7 | Number of days to report, ending today |
| `--daily` | | Today only (shorthand for `--days 1`) |
| `--since DUR` | | Relative lookback ending today (`1mo`, `12w`, `90d`, `36h`, `1w3d`); mutually exclusive with `--days`/`--daily` |
| `--by type\|model` | type | Token breakdown dimension |
| `--tz ZONE` | host timezone | IANA timezone for day boundaries (`UTC`, `America/Chicago`) |
| `--json` | | Emit JSON to stdout instead of tables |
| `--addr` | localhost:9000 | ClickHouse native TCP address |
| `--user` | grafana | ClickHouse user |
| `--db` | otel | ClickHouse database |
| `--version` | | Print the version and exit |

## Output streams

Tables and JSON go to **stdout**; status lines go to **stderr**, so
`--json | jq` streams cleanly. Lookbacks are unbounded — the `otel_*` tables
have no TTL, so `--since 1y` reports everything stored.

## Day boundaries

`TimeUnix` is stored in UTC, but days are grouped in the host timezone by default,
so an evening session west of UTC stays on the day it happened rather than rolling
into the next one. The zone is resolved from `TZ`, falling back to
`/etc/localtime` and then `UTC`, and the "Reporting usage since" line names the
zone in effect.

Both halves of the query use one zone: the `--daily`/`--days` cutoff lands on
midnight in that zone, and `toDate` buckets in the same one. Mixing them silently
misfiles usage — grouping by UTC day while cutting off at local midnight moved a
late-evening session onto the following day.

`--tz UTC` groups by UTC day instead. Totals never change with `--tz`; only the
day a row lands on does.

## Tests

```bash
make test        # or: go test ./...
```

The tests are hermetic — no ClickHouse or cluster access required.

## Make targets

`make` (or `make help`) lists every target.

| Target | Description |
|--------|-------------|
| `help` | List available targets |
| `build` | Build `otel-clickhouse-stack` into the working directory |
| `install` | Build into `$GOBIN` (or `$GOPATH/bin`) so it's on `PATH` |
| `run` | `go run .` |
| `port-forward` | Forward ClickHouse's native TCP port (9000) from the cluster |
| `test` | `go test` with race detector and coverage profile |
| `test-html` | Run tests and open the HTML coverage report |
| `fmt` / `fmt-check` | Format with gofmt / fail if anything needs formatting |
| `lint` | golangci-lint |
| `tidy` | `go mod tidy` |
| `pre-commit` | `fmt-check` + `lint` + `test` |
| `docs` | Serve package docs at `http://localhost:6060` |

`build` and `install` stamp `main.version` from `git describe`, which
`--version` prints. `lint` runs out of `tools/go.mod` via `go tool`, so no
separate install is needed.

## Not yet ported

The upstream tool also has `backup` and `restore` subcommands (logical snapshots
of the database over the native protocol) and a `backfill` tool that loads
Claude Code transcripts. Neither came across in this port.
