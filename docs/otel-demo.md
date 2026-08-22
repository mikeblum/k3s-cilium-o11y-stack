# OpenTelemetry demo app

*Optional.* The [OTel demo](https://opentelemetry.io/docs/demo/) is roughly 20
instrumented microservices plus a load generator, deployed into its own
`otel-demo` namespace. It is the workload that exercises in-cluster OTLP ingest
end to end, and so also the fastest way to get real traces into ClickHouse.

```bash
make otel-demo-install
make -C k8s/otel-demo pf-frontend   # then http://localhost:8080/
```

Its own collector and its bundled Jaeger, Prometheus, Grafana, and OpenSearch
are all disabled, since this stack already provides them. The services talk
straight to the o11y collector:

```
demo services ──OTLP──▶ otelcol.o11y ──▶ ClickHouse
  (otel-demo ns)         (o11y ns)   └──▶ spanmetrics ──▶ Prometheus
```

Every component builds its endpoint from `OTEL_COLLECTOR_NAME`, so a single
`default.envOverrides` entry repoints all of them. Talking directly also keeps
enrichment correct: `k8s_attributes` resolves `from: connection` to the calling
pod, which is the service itself rather than a forwarder.

Configuration lives in
[`k8s/otel-demo/values/otel-demo.yaml`](../k8s/otel-demo/values/otel-demo.yaml).

## Dashboards

The demo's **Spanmetrics** dashboard is provisioned into an **OTel Demo** folder
in Grafana and reads `traces_span_metrics_*` from Prometheus. Those metrics come
from the `spanmetrics` connector in the o11y collector, which replaces what the
demo's own collector used to derive, so the dashboard generalizes to any app
sending traces. See [telemetry.md](telemetry.md) for how that connector is
wired.

Seven other dashboards ship with the demo and are **not** imported, because
nothing here produces what they query. `apm` and `demo` need Jaeger and
OpenSearch; `linux`, `NGINX`, and `postgresql` need scrape jobs that lived in
the demo's collector; `exemplars` needs the demo's app metrics in Prometheus
rather than ClickHouse; and `opentelemetry-collector` duplicates the OTel
folder's gnetId 15983. All seven are in the chart under
`grafana/provisioning/dashboards/` if you want to adapt one.

For traces and logs, query ClickHouse, either through the ClickHouse datasource
in Grafana or directly:

```bash
kubectl exec -n o11y clickhouse-clickhouse-0-0-0 -- clickhouse-client --query \
  "SELECT ServiceName, count() FROM otel.otel_traces GROUP BY ServiceName ORDER BY 2 DESC"
```

## Known rough edges

The demo UI's own `/jaeger/ui/` and `/grafana/` links still return 502. Its
Envoy proxies them to in-namespace backends that no longer exist, and it passes
the `/grafana` prefix through unrewritten, which this Grafana does not serve
from. Use `grafana.<domain>` directly.

Span-derived latency is clamped at the top of the histogram. The connector runs
on its default buckets, whose highest finite bucket is 15s, so `checkout` reads
as exactly 15000ms at p95 while its real p95 is around 23s. ClickHouse has the
exact figure:

```sql
SELECT quantile(0.95)(Duration) FROM otel.otel_traces
```

Span names are also unsanitised here, because the OTTL rules that did that lived
in the demo's collector and were not rebuilt.

## Uninstall

```bash
make otel-demo-uninstall
```

Drops the namespace, all 25 workloads, and the Grafana dashboard ConfigMap.
