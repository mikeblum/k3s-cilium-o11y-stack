# Apps

Example and reference workloads that push telemetry into the o11y stack. Each
app lives in its own subdirectory and ships with a `Dockerfile` plus Kubernetes
manifests under `k8s/apps/<name>/`.

| App | Description |
|-----|-------------|
| [otel-go](./otel-go/) | Minimal Go HTTP service (`/ping → PONG`) that exercises traces, logs, and Prometheus metrics via OTel autoexport |

---

## Adding a service

Integrating a new OTel-instrumented workload takes three steps.

### 1. Set OTEL env vars in your pod spec

All exporter configuration comes from environment variables — no code changes
needed when you swap backends.

```yaml
env:
  - name: OTEL_SERVICE_NAME
    value: myapp
  - name: OTEL_EXPORTER_OTLP_ENDPOINT
    value: http://alloy.o11y.svc.cluster.local:4317
  - name: OTEL_EXPORTER_OTLP_PROTOCOL
    value: grpc
  - name: OTEL_TRACES_EXPORTER
    value: otlp
  - name: OTEL_METRICS_EXPORTER
    value: prometheus        # exposes /metrics; Alloy scrapes it
  - name: OTEL_LOGS_EXPORTER
    value: otlp
  - name: OTEL_EXPORTER_PROMETHEUS_PORT
    value: "2112"
```

`OTEL_METRICS_EXPORTER=otlp` is also valid — metrics will flow into ClickHouse
`otel_metrics` instead of Prometheus. Choose based on your dashboard needs.

### 2. Add your service to the Alloy scrape target list

**Edit** the existing `prometheus.scrape "go_services"` targets list in
`k8s/o11y/manifests/alloy-configmap.yaml`. Do **not** add a second block with
the same name — Alloy rejects duplicate component names.

```hcl
prometheus.scrape "go_services" {
  targets = [
    { __address__ = "otel-go.otel-demo.svc.cluster.local:2112", job = "otel-go" },
    // ↓ add your service here
    { __address__ = "myapp.myns.svc.cluster.local:2112", job = "myapp" },
  ]
  scrape_interval = "15s"
  forward_to      = [prometheus.remote_write.local.receiver]
}
```

Apply and hot-reload (no Alloy pod restart needed):

```bash
kubectl apply -f k8s/o11y/manifests/alloy-configmap.yaml
```

### 3. Add an HTTPRoute and optional Tailscale Ingress

For LAN access, add a host-based HTTPRoute. See
`k8s/apps/otel-go/httproute.yaml` and `k8s/o11y/manifests/gateway-routes.yaml`
for working examples.

For remote access (anywhere on your tailnet), see
`k8s/tailscale/manifests/ingress-grafana.yaml` for the Tailscale Ingress
pattern.

---

## otel-go

A minimal Go HTTP service that validates the full telemetry pipeline end-to-end.

```
GET /ping  →  PONG
```

**Signals exercised:**

| Signal | Exporter | Destination |
|--------|----------|-------------|
| Traces | OTLP gRPC | Alloy → ch-writer → ClickHouse `otel_traces` |
| Logs | OTLP gRPC | Alloy → ch-writer → ClickHouse `otel_logs` |
| Metrics | Prometheus `:2112` | Alloy scrape → Prometheus remote-write |

**Build and deploy:**

Image tags are always the short git SHA (`git rev-parse --short HEAD`) — never `latest`.

```bash
cd k8s/apps/otel-go

# Option A — single-node k3s: build locally and import into containerd (no registry needed)
make load IMAGE=ghcr.io/<you>/otel-go    # builds + streams into k3s ctr

# Option B — push to ghcr.io (requires: echo $GITHUB_TOKEN | docker login ghcr.io -u <user> --password-stdin)
make push IMAGE=ghcr.io/<you>/otel-go

# Deploy (uses the same SHA tag automatically)
make deploy DOMAIN=example.local

# Wire into Alloy (hot-reload)
kubectl apply -f ../../o11y/manifests/alloy-configmap.yaml

# Generate traffic
kubectl run -it --rm pinger --image=curlimages/curl:8.12.1 --restart=Never -- \
  sh -c 'for i in $(seq 1 30); do curl -s otel-go.otel-demo.svc.cluster.local:8080/ping; sleep 1; done'
```

**Verify data is flowing:**

```bash
# Traces in ClickHouse
kubectl exec -n o11y clickhouse-0 -- \
  clickhouse-client --query "SELECT count() FROM otel.otel_traces WHERE ServiceName='otel-go'"

# Logs in ClickHouse
kubectl exec -n o11y clickhouse-0 -- \
  clickhouse-client --query "SELECT count() FROM otel.otel_logs WHERE ResourceAttributes['service.name']='otel-go'"

# Metrics in Prometheus
kubectl exec -n o11y -l app.kubernetes.io/name=prometheus -- \
  wget -qO- 'http://localhost:9090/api/v1/query?query=ping_total{job="otel-go"}'
```

**Swap exporters without rebuilding** — e.g. to debug locally with console output:

```bash
SHA=$(git rev-parse --short HEAD)
OTEL_TRACES_EXPORTER=console OTEL_METRICS_EXPORTER=console OTEL_LOGS_EXPORTER=console \
  docker run --rm -p 8080:8080 -p 2112:2112 ghcr.io/<you>/otel-go:${SHA}
```
