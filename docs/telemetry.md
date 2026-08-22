# How telemetry flows

Where each signal lands, how it gets there, and what to change when you add a
service of your own. See the [README](../README.md) for the architecture
diagram this expands on.

## Metrics live in two stores

Query the wrong one and panels come back empty instead of erroring, so it is
worth knowing which is which:

- **Prometheus** (`prometheus` datasource, PromQL) holds Cilium, Hubble,
  cilium-operator, collector self-metrics, node-exporter, apiserver, nodes,
  cadvisor, kube-state-metrics, and the span-derived RED metrics below.
- **ClickHouse** (`clickhouse` datasource, SQL) holds everything arriving over
  OTLP — app logs, traces, and metrics — plus ClickHouse's own `system` tables.

Grafana ships four folders of provisioned dashboards: **Cilium** and **Host**
(PromQL), **OTel** (the collector's own metrics, plus log, trace, and service
explorers over ClickHouse), and **ClickHouse** (query, cluster, data, and
system-metrics dashboards from the [ClickHouse datasource
plugin](https://github.com/grafana/clickhouse-datasource/tree/main/src/dashboards),
for watching the database itself).

## RED metrics come free with traces

A `spanmetrics` connector on the traces pipeline derives rate, error, and
duration series from every span and remote-writes them to Prometheus as
`traces_span_metrics_calls_total` and
`traces_span_metrics_duration_milliseconds_*`, labelled by `service_name`,
`span_name`, and `status_code`. Send traces and you get them, with no extra
instrumentation. They are PromQL-shaped, which is why they land in Prometheus.

> Cardinality is one series per service × span name × status code. Name spans
> after routes (`/api/products/{id}`), never raw URLs with IDs in them: each
> distinct span name permanently multiplies series here.

## Scraping lives in the collector, not Prometheus

Prometheus's annotation-driven discovery is off, so `prometheus.io/scrape` on a
pod does nothing. Add scrape targets under `prometheus/scrape` in
[`k8s/o11y/values/otel-collector.yaml`](../k8s/o11y/values/otel-collector.yaml).

## Adding a service

**1.** Set OTLP env vars in your pod spec:

```yaml
env:
  - name: OTEL_EXPORTER_OTLP_ENDPOINT
    value: "http://otelcol.o11y.svc.cluster.local:4317"
  - name: OTEL_SERVICE_NAME
    value: "myapp"
```

That covers metrics too, since OTLP metrics land in ClickHouse. Continue only if
your app exposes a Prometheus-style `/metrics` endpoint instead.

**2.** Add a scrape job under `config.receivers.prometheus/scrape` in
`k8s/o11y/values/otel-collector.yaml`, extending the `go-services` placeholder:

```yaml
- job_name: myapp
  scrape_interval: 15s
  static_configs:
    - targets: ["myapp.myapp.svc.cluster.local:2112"]
```

```bash
make -C k8s/o11y install
```

**3.** Add an HTTPRoute for the LAN and a Tailscale Ingress. See
`k8s/o11y/manifests/gateway-routes.yaml` and
`k8s/tailscale/manifests/ingress-grafana.yaml` for examples.

## Sending telemetry from outside the cluster

In-cluster apps send OTLP to `otelcol.o11y.svc.cluster.local:4317`. Producers
outside the cluster use `http://otlp.<tailnet>.ts.net:4318` for HTTP or `:4317`
for gRPC; see
[`k8s/o11y/CLAUDE-CODE-TELEMETRY.md`](../k8s/o11y/CLAUDE-CODE-TELEMETRY.md) for a
worked example.

One OTLP receiver serves both paths, which is safe only because
`k8s_attributes` excludes the Tailscale proxy pods. Without that exclude, every
external payload would be attributed to the proxy instead of its real sender.
Anything else that forwards OTLP on behalf of other pods needs the same
treatment.
