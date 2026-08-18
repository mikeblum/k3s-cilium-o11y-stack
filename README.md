# k3s-cilium-o11y-stack 🔭

> Bootstrap your own local eBPF-powered, OTel-compatible observability stack

## OS Support
**Linux 🐧 only** for now

While MacOS supports Docker 🐳, eBPF is trickier to do inside VMs.

## Overview

This project seeks to reduce the complex and costly $$$ cloud ☁️ deployments of Kubernetes-backed observability into a local-first implementation that empowers DevOps, SREs, and other observability practictioners with an industry-standard foundation powered by well-known open-source projects.

Each component of this stack was chosen against the following criteria:
- **Mature and open source** - preferrably part of the CNCF ecosystem
- **OpenTelemetry support 🔭** - OTel is the defacto standard for observing systems
- **eBPF support 🐝** - eBPF is becoming a standard for instrumenting networks and increasingly applications
- **Local-first deployments** - easy to deploy, observe, and modify locally
- **Out-of-the-box visualizations + dashboards** - import and export Grafana dashboards

### Why not use Clickhouse's ClickStack, SigNoz, et al instead?

Since we're using eBPF-powered Cilium to instrument and visualize our stack's network, the Hubble UI packaged with Cilium uses Grafana dashboards and Prometheus metrics. Given these requiremnts it was simpler to have a stack that hews to the least amount of moving parts as opposed to building custom shims to shape the data to fit in a box.

Using Grafana and Hubble for visualizations gives us the best of both worlds - a vibrant collection of Grafana dashboards alongside out-of-the-box visualizations of the stack's network via Hubble. Exposing the base components of ClickHouse and Cilium gives operators a deeper view of how telemetry flows compared to a more all-in-one solution like Signoz or Grafana's LGTM omnibus images.

This observability stack is opinionated in that eBPF is The Way ™️ for observing and securing networks. But to-date its unclear how eBPF meshes with the other traditional pillars of observability: metrics, traces, and logs. By deploying Cilium as the CNI we can do meta-analysis of how telemetry data flows - versus today were a blackbox sidecar is spun up alongside your application and its assumed your data will make it upstream to your vendor of choice.

## Components

| Component | Layer | Role | Access |
|-----------|-------|------|--------|
| [k3s](https://k3s.io) | Cluster | Lightweight Kubernetes; runs with flannel, kube-proxy, and Traefik disabled to make room for Cilium + Envoy Gateway | internal |
| [Cilium](https://cilium.io) | Networking | eBPF CNI — pod networking, kube-proxy replacement, and L2 LoadBalancer IP pool | internal |
| [Hubble UI](https://docs.cilium.io/en/stable/gettingstarted/hubble/) | Networking | Real-time network flow visualization built into Cilium | `hubble.<domain>` |
| [Envoy Gateway](https://gateway.envoyproxy.io) | Ingress | Kubernetes Gateway API controller; routes HTTPS subdomains to in-cluster services | internal |
| [mkcert](https://github.com/FiloSottile/mkcert) | TLS | [@FiloSottile](https://github.com/FiloSottile)'s excellent tool for bringing https to `localhost` | setup CLI |
| [Prometheus](https://prometheus.io) | Observability | Time-series store for infra metrics; backs the Cilium dashboards | internal |
| [OTel Collector](https://opentelemetry.io/docs/collector/) | Observability | Receives OTLP → ClickHouse; scrapes Cilium/Hubble/host → Prometheus | `:4317/:4318` |
| [node-exporter](https://github.com/prometheus/node_exporter) | Observability | Host metrics for the node itself — CPU, memory, disk, filesystem, network | internal |
| [ClickHouse](https://clickhouse.com) | Observability | OLAP database; backend store for logs, traces, and metrics | internal |
| [Grafana](https://grafana.com) | Observability | Dashboards and visualization over Prometheus + ClickHouse | `grafana.<domain>` |
| [Tailscale Operator](https://tailscale.com/kb/1236/kubernetes-operator) | Remote access | *(optional)* Exposes services to your tailnet with auto-provisioned Let's Encrypt TLS | — |

_Exposed services use host-based routing. TLS is an mkcert wildcard on the LAN and auto-provisioned Let's Encrypt over Tailscale, where each also resolves at `<service>.<tailnet>.ts.net`. `<domain>` defaults to `example.local`._

_In-cluster apps send OTLP to `otelcol.o11y.svc.cluster.local:4317`. Producers outside the cluster use `http://otlp.<tailnet>.ts.net:4318` for HTTP or `:4317` for gRPC — see [`k8s/o11y/CLAUDE-CODE-TELEMETRY.md`](k8s/o11y/CLAUDE-CODE-TELEMETRY.md)._

## Architecture

```
Ingress ─ reaching the UIs
  ┌───────────┐  mkcert TLS (LAN)
  │ LAN       │────────────────────▶ Envoy Gateway ──────┐
  └───────────┘                                          ├──▶ Grafana · Hubble UI
  ┌───────────┐  Let's Encrypt (Tailscale)               │
  │ Tailscale │────────────────────▶ Tailscale Operator ─┘
  └───────────┘

Data plane ─ how telemetry flows
                       ┌────────────────┐
  apps ──── OTLP ─────▶│ OTel Collector │──▶ ClickHouse ──┐
                       └────────────────┘                 ├──▶ Grafana
   Cilium · Hubble ◀──── scrape ──┤                       │
                                  └──remote_write──▶ Prometheus
                                                          ▲
                     apiserver · nodes · cadvisor · node-exporter ┘
                     kube-state-metrics
```

**Metrics live in two stores.** Panels return empty rather than erroring when
you query the wrong one:

- **Prometheus** (`prometheus` datasource, PromQL) — Cilium, Hubble,
  cilium-operator, collector self-metrics, node-exporter, apiserver, nodes,
  cadvisor, kube-state-metrics, plus the span-derived RED metrics below.
- **ClickHouse** (`clickhouse` datasource, SQL) — everything arriving over OTLP:
  app logs, traces, and metrics.

**RED metrics come free with traces.** A `spanmetrics` connector on the traces
pipeline derives rate/error/duration series from every span and remote-writes
them to Prometheus as `traces_span_metrics_calls_total` and
`traces_span_metrics_duration_milliseconds_*`, labelled by `service_name`,
`span_name` and `status_code`. Send traces and you get them — no extra
instrumentation. They are PromQL-shaped, which is why they land in Prometheus
rather than ClickHouse.

> Cardinality is one series per service × span name × status code. Name spans
> after routes (`/api/products/{id}`), never raw URLs with IDs in them — each
> distinct span name permanently multiplies series here.

**Scraping lives in the collector, not Prometheus.** Prometheus's
annotation-driven discovery is off, so `prometheus.io/scrape` on a pod does
nothing. Add scrape targets under `prometheus/scrape` in
`k8s/o11y/values/otel-collector.yaml`.

## Adding a service

Exposing a custom service in your cluster is a simple process:

**1.** Set OTLP env vars in your pod spec:

```yaml
env:
  - name: OTEL_EXPORTER_OTLP_ENDPOINT
    value: "http://otelcol.o11y.svc.cluster.local:4317"
  - name: OTEL_SERVICE_NAME
    value: "myapp"
```

Step 1 covers metrics too — OTLP metrics land in ClickHouse. Only continue if
your app exposes a Prometheus-style `/metrics` endpoint instead.

**2.** Add a scrape job under `config.receivers.prometheus/scrape` in
`k8s/o11y/values/otel-collector.yaml` (extend the `go-services` placeholder):

```yaml
- job_name: myapp
  scrape_interval: 15s
  static_configs:
    - targets: ["myapp.myapp.svc.cluster.local:2112"]
```

```bash
make -C k8s/o11y install
```

**3.** Add an HTTPRoute (LAN) and Tailscale Ingress — see `k8s/tailscale/manifests/ingress-grafana.yaml` and `k8s/o11y/manifests/gateway-routes.yaml` for examples.

## OpenTelemetry demo app

*Optional.* The [OTel demo](https://opentelemetry.io/docs/demo/) — ~20 instrumented
microservices and a load generator — deployed into its own `otel-demo` namespace.
It is the workload that actually exercises in-cluster OTLP ingest, so it is also
the fastest way to get real traces into ClickHouse.

```bash
make otel-demo-install
make -C k8s/otel-demo pf-frontend   # then http://localhost:8080/
```

Its own collector and its bundled Jaeger, Prometheus, Grafana and OpenSearch are
all disabled — this stack already provides them, and the services talk straight
to the o11y collector:

```
demo services ──OTLP──▶ otelcol.o11y ──▶ ClickHouse
  (otel-demo ns)         (o11y ns)   └──▶ spanmetrics ──▶ Prometheus
```

Every component builds its endpoint from `OTEL_COLLECTOR_NAME`, so one
`default.envOverrides` entry repoints all of them. Talking directly also keeps
enrichment correct: `k8s_attributes` resolves `from: connection` to the calling
pod, which is the service itself rather than a forwarder.

### Dashboards

The demo's **Spanmetrics** dashboard is provisioned into an **OTel Demo** folder
in Grafana, reading `traces_span_metrics_*` from Prometheus. Those metrics come
from the `spanmetrics` connector in the o11y collector (see *Metrics live in two
stores*), which replaces what the demo's own collector used to derive — so the
dashboard works for any app sending traces, not just this one.

The demo ships seven other dashboards that are **not** imported, because nothing
here produces what they query: `apm` and `demo` need Jaeger and OpenSearch;
`linux`, `NGINX` and `postgresql` need scrape jobs that lived in the demo's
collector; `exemplars` needs the demo's app metrics in Prometheus rather than
ClickHouse; and `opentelemetry-collector` duplicates the OTel folder's gnetId
15983. They are in the chart at `grafana/provisioning/dashboards/` if you want
to adapt one.

For traces and logs, query ClickHouse — either the ClickHouse datasource in
Grafana, or directly:

```bash
kubectl exec -n o11y clickhouse-0 -- clickhouse-client --query \
  "SELECT ServiceName, count() FROM otel.otel_traces GROUP BY ServiceName ORDER BY 2 DESC"
```

**Note:** the demo UI's own `/jaeger/ui/` and `/grafana/` links still 502 — its
Envoy proxies them to in-namespace backends that no longer exist, and it passes
the `/grafana` prefix through unrewritten, which this Grafana does not serve
from. Use `grafana.<domain>` directly.

**Caveat on span-derived latency:** the demo's collector also sanitised span
names before deriving metrics; without it, `checkout` p95 reads as exactly
15000ms because that is the connector's highest finite bucket. Its real p95 is
~23s — `SELECT quantile(0.95)(Duration) FROM otel.otel_traces` has the exact
figure.

Uninstall with `make otel-demo-uninstall` — it drops the namespace, all 25
workloads, and the Grafana dashboard ConfigMap.

---

## Troubleshooting

**TLS warning in browser** — mkcert CA not trusted on this device. See `k8s/tls/README.md`.

**Grafana unreachable:**
```bash
kubectl describe httproute grafana -n o11y | grep -A5 Status
kubectl get secret example-local-tls -n envoy-gateway-system   # dots-to-dashes of your DOMAIN
```

**ClickHouse not receiving data:**
```bash
kubectl logs -n o11y ds/otelcol-agent --tail=30
kubectl exec -n o11y clickhouse-0 -- clickhouse-client \
  --query "SELECT ServiceName, count() FROM otel.otel_logs GROUP BY ServiceName"
```

**Telemetry from outside the cluster not arriving** — test the path before
debugging client config. A `200` with `{"partialSuccess":{}}` means the endpoint
is fine:
```bash
curl -sv http://otlp.<tailnet>.ts.net:4318/v1/traces \
  -X POST -H 'Content-Type: application/json' -d '{}'
kubectl logs -n tailscale -l tailscale.com/parent-resource=otlp-ts --tail=30
```

**A metric appears twice under different `job` labels** — two things are
scraping the same endpoint. More than one row back means it's collected twice;
remove the overlapping job from `values/prometheus.yaml` or the collector's
`prometheus/scrape`:
```bash
kubectl port-forward -n o11y svc/prometheus-server 9090:80 &
curl -sG http://localhost:9090/api/v1/query \
  --data-urlencode 'query=count by (job) (cilium_agent_bootstrap_seconds_count)'
```

> Always pass PromQL with `-G --data-urlencode`. In a bare URL an unencoded `+`
> arrives as a space, so `.+` becomes `. ` and the query returns empty instead
> of erroring.

**Tailscale proxy missing from Machines:**
```bash
kubectl logs -n tailscale -l app=operator --tail=50
cd k8s/tailscale && make logs-grafana
```
