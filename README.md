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

That said, we do take ClickStack's foundation. The `clickstack` chart bundles HyperDX, MongoDB, and its own OTel Collector on top of the [official ClickHouse operator](https://github.com/ClickHouse/clickhouse-operator), and HyperDX cannot be switched off. So we install that operator directly and declare the `KeeperCluster` + `ClickHouseCluster` ourselves in [`k8s/o11y/manifests/clickhouse-cluster.yaml`](k8s/o11y/manifests/clickhouse-cluster.yaml) - same upstream ClickHouse, none of the parts Grafana already covers.

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
| [ClickHouse](https://clickhouse.com) | Observability | OLAP database; backend store for logs, traces, and metrics. Run by ClickHouse's [official Kubernetes operator](https://github.com/ClickHouse/clickhouse-operator) | internal |
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
  app logs, traces, and metrics, plus ClickHouse's own `system` log tables.

Grafana ships with four folders of provisioned dashboards: **Cilium** and
**Host** (PromQL), **OTel** (the collector's own metrics, plus log/trace/service
explorers over ClickHouse), and **ClickHouse** — query, cluster, data, and
system-metrics dashboards from the [ClickHouse datasource
plugin](https://github.com/grafana/clickhouse-datasource/tree/main/src/dashboards)
for watching the database itself.

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
kubectl exec -n o11y clickhouse-clickhouse-0-0-0 -- clickhouse-client --query \
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

## Restoring a single service

**Yes — every component is reconciled by re-running its own install target.**
Each one wraps `helm upgrade --install`, which is idempotent: unchanged objects
are left alone, and anything deleted out-of-band is recreated. There is no
separate "repair" command, and you do not need to uninstall first.

| Component | Command | Namespace |
|-----------|---------|-----------|
| Cilium + Hubble (relay, UI) | `make cilium-upgrade` | `kube-system` |
| Cilium LB pool + L2 policy | `make cilium-lb` | `kube-system` |
| Envoy Gateway controller | `make gateway-install` | `envoy-gateway-system` |
| Gateway + GatewayClass | `make gateway-apply` | `envoy-gateway-system` |
| mkcert wildcard cert | `make tls-install` | `envoy-gateway-system` |
| Grafana, Prometheus, collector, node-exporter | `make o11y-install` | `o11y` |
| ClickHouse (operator + CRs) | `make o11y-clickhouse` | `o11y` |
| Tailscale operator + Ingresses | `make tailscale-install` | `tailscale` |
| OTel demo app | `make otel-demo-install` | `otel-demo` |

To restore a single o11y component without re-running the rest, use that
chart's line from `k8s/o11y/Makefile` on its own:

```bash
helm upgrade --install grafana grafana/grafana -n o11y \
  -f k8s/o11y/values/grafana.yaml --version 10.5.15
```

Check what a target would change before running it. For Helm:

```bash
helm upgrade --install <release> <chart> -n <ns> -f <values> --dry-run=server
```

For the plain-manifest steps, `kubectl diff` prints nothing when a re-run would
be a no-op:

```bash
kubectl diff -f k8s/o11y/manifests/clickhouse-cluster.yaml
```

### Restoring one workload without touching the release

Re-running a whole chart re-applies every object in it. When a single
Deployment has been deleted and you want to recreate *only* that one, take it
from the release Helm already has stored:

```bash
helm get manifest cilium -n kube-system > /tmp/cilium.yaml
# extract the Deployment you need, then:
kubectl apply -f /tmp/hubble-deployments.yaml
```

Helm stamps ownership metadata at apply time, so it is **not** in
`helm get manifest` output. Add it back or the next `helm upgrade` fails with
*"invalid ownership metadata"*:

```yaml
metadata:
  annotations:
    meta.helm.sh/release-name: cilium
    meta.helm.sh/release-namespace: kube-system
  labels:
    app.kubernetes.io/managed-by: Helm
```

### Is a Cilium upgrade safe to re-run?

Yes. Hubble's TLS material is generated with `hubble.tls.auto.method: helm`,
which sounds like it would rotate on every upgrade — it does not. The chart
reuses the existing `cilium-ca` Secret via a `lookup`, so a re-run renders a
byte-identical CA, leaves the DaemonSet spec unchanged, and does not restart
the Cilium agent or drop pod networking. Confirm before running:

```bash
kubectl get secret -n kube-system hubble-server-certs \
  -o jsonpath='{.data.ca\.crt}' | base64 -d | openssl x509 -noout -fingerprint -sha256
```

Re-run with `--dry-run=server` (needed — `lookup` returns empty on a client-side
dry run) and compare the rendered CA to that fingerprint.

### Restoring ClickHouse

ClickHouse is **not** a Helm release, so it does not follow the pattern above.
The operator's chart installs a controller and two CRDs; the database itself is
a `KeeperCluster` + `ClickHouseCluster` pair of CRs. `make o11y-clickhouse` does
both in order, and the two `kubectl wait`s in it are load-bearing — a CR is
rejected outright if its CRD is not yet `Established`, and the operator has to
be up to act on it.

Re-running it is safe: the CRs are applied declaratively, so an unchanged spec
is a no-op and the operator does not restart the server. Confirm with
`kubectl diff` before running if you want certainty.

Names to expect, since the operator derives them from the CR name rather than
the chart: pod `clickhouse-clickhouse-0-0-0`, stable ClusterIP
`clickhouse-server` (the headless Service exists too, but the Tailscale operator
rejects a headless backend). `make -C k8s/o11y ch-client` opens a
`clickhouse-client` shell without needing credentials — the operator writes the
`default` password into the pod's client config.

> `make -C k8s/o11y uninstall` is the one thing that destroys data: the CRDs are
> installed with `keep: true` so `helm uninstall` deliberately leaves them, and
> the explicit `kubectl delete crd` cascades into the cluster and its PVC.

---

## Troubleshooting

**TLS warning in browser** — mkcert CA not trusted on this device. See `k8s/tls/README.md`.

**Grafana unreachable:**
```bash
kubectl describe httproute grafana -n o11y | grep -A5 Status
kubectl get secret example-local-tls -n envoy-gateway-system   # dots-to-dashes of your DOMAIN
```

**Hubble UI down / `hubble.<tailnet>.ts.net` not loading** — check whether the
workloads exist at all before debugging ingress. Cilium's agent can be perfectly
healthy while `hubble-relay` and `hubble-ui` are missing; the Services and the
Tailscale Ingress survive on their own and keep resolving, so the symptom looks
like a proxy fault:
```bash
kubectl get deploy -n kube-system hubble-relay hubble-ui
kubectl -n kube-system exec ds/cilium -c cilium-agent -- cilium-dbg status | grep Hubble
```
`Hubble: Ok` with a missing Deployment means only the UI tier is gone — restore
it per *Restoring a single service*. Verify the whole path end to end:
```bash
kubectl -n kube-system exec ds/cilium -c cilium-agent -- \
  hubble observe --server "$(kubectl get svc -n kube-system hubble-relay \
  -o jsonpath='{.spec.clusterIP}'):80" --last 5
```
Use the relay's ClusterIP, not its DNS name — the agent runs in the host network
namespace and cannot resolve cluster DNS.

**ClickHouse not receiving data:**
```bash
kubectl logs -n o11y ds/otelcol-agent --tail=30
kubectl exec -n o11y clickhouse-clickhouse-0-0-0 -- clickhouse-client \
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
