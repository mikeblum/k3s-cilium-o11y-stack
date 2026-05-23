# roguequery.local — observability stack

k3s + Cilium + Hubble + Envoy Gateway + ClickHouse.

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│ In-cluster signals                                      │
│                                                         │
│  Cilium/Hubble :9962/:9965 ──scrape──┐                  │
│  Go services /metrics      ──scrape──┤                  │
│  Go services OTLP SDK      ──push───▶│  Alloy v1.16.1  │
│                                      │  (DaemonSet)    │
└──────────────────────────────────────┼─────────────────┘
                                       │
                    ┌──────────────────┴──────────────────┐
                    │                                     │
              Prometheus metrics                    OTLP signals
              (Cilium/Hubble/scrape)            (logs/traces/metrics)
                    │                                     │
                    ▼                                     ▼
               Prometheus                           ch-writer
                    │                          (otelcol-contrib)
                    └──────────────┬─────────────────────┘
                                   │
                               Grafana
                       (ClickHouse + Prometheus datasources)
```

### TLS split

```
LAN (roguequery.local)          Tailscale (*.ts.net)
─────────────────────────       ──────────────────────────────
mkcert local CA                 Let's Encrypt (via Tailscale operator)
Envoy Gateway HTTPS listener    Tailscale Ingress proxy per service
*.roguequery.local wildcard     grafana.<tailnet>.ts.net
HTTP:80 → HTTPS:443 redirect    HTTPS only, no redirect needed
CA must be installed per device No CA install — publicly trusted
```

### Access map

| URL | TLS | Network |
|---|---|---|
| `https://grafana.roguequery.local` | mkcert | LAN |
| `https://hubble.roguequery.local` | mkcert | LAN |
| `https://roguequery.local/alloy` | mkcert | LAN (debug) |
| `https://grafana.<tailnet>.ts.net` | Let's Encrypt | Tailscale |
| `https://hubble.<tailnet>.ts.net` | Let's Encrypt | Tailscale |

---

## Prerequisites

### 1. Cilium installed with metrics enabled

```bash
helm upgrade cilium cilium/cilium \
  --namespace kube-system \
  --reuse-values \
  --set prometheus.enabled=true \
  --set operator.prometheus.enabled=true \
  --set hubble.enabled=true \
  --set hubble.metrics.enableOpenMetrics=true \
  --set hubble.metrics.enabled="{dns,drop,tcp,flow,port-distribution,icmp,httpV2}"
```

Verify before proceeding:

```bash
kubectl exec -n kube-system ds/cilium -- \
  curl -s http://localhost:9962/metrics | head -5
```

### 2. Cilium + Tailscale: kube-proxy replacement caveat

If Cilium runs in kube-proxy replacement mode, annotate the `tailscale`
namespace before installing the operator:

```bash
kubectl annotate namespace tailscale io.cilium/no-track-port="0"
```

### 3. Desktop /etc/hosts

```
192.168.86.101  roguequery.local
192.168.86.101  grafana.roguequery.local
192.168.86.101  hubble.roguequery.local
```

---

## Install order

### Step 1 — TLS certificate (run once, then every ~2 years)

```bash
cd k8s/tls
make install
```

Generates a wildcard cert for `*.roguequery.local` using mkcert and stores
it as a Kubernetes Secret in `envoy-gateway-system`. See `k8s/tls/README.md`
for CA distribution instructions per device/platform.

### Step 2 — Envoy Gateway

```bash
kubectl apply -f k8s/envoy-gateway/gateway.yaml
```

Adds the HTTPS listener (port 443) to `cluster-ingress`. The HTTP listener
(port 80) issues a 301 redirect to HTTPS for all traffic.

Verify it is programmed:

```bash
kubectl get gateway cluster-ingress -n envoy-gateway-system \
  -o jsonpath='{.status.conditions[?(@.type=="Programmed")].status}'
# Expected: True
```

### Step 3 — Observability stack

```bash
cd k8s/o11y
make install
make status
```

Expected steady state:

```
NAME                              READY   STATUS
alloy-xxxxx (×nodes)              1/1     Running
ch-writer-xxxxx                   1/1     Running
clickhouse-0                      1/1     Running
grafana-xxxxx                     1/1     Running
prometheus-server-xxxxx           1/1     Running
```

### Step 4 — Tailscale operator

```bash
cd k8s/tailscale
cp values.secret.yaml.example values.secret.yaml
# Fill in OAuth clientId and clientSecret from:
# https://login.tailscale.com/admin/settings/oauth
make install
make status
```

First-time Tailscale setup in admin console:
1. Settings → DNS → enable MagicDNS and HTTPS
2. Settings → OAuth clients → create with Devices Core/Auth Keys/Services write + tag:k8s-operator
3. Merge `acl-snippet.json` into https://login.tailscale.com/admin/acls

#### Key expiry for operator-managed devices

Add to your tailnet ACL policy to disable key expiry for infrastructure devices:

```json
"nodeAttrs": [
  {
    "target": ["tag:k8s-ingress", "tag:k8s-operator"],
    "attr":   ["noExpiry"]
  }
]
```

The operator uses an OAuth client (no expiry) and auto-provisions proxy
device keys. Disabling key expiry on the tagged devices prevents proxy
pods from going offline waiting for re-auth. Personal user devices should
keep expiry enabled.

---

## Verify

```bash
# HTTPS cert is valid and trusted
curl -I https://grafana.roguequery.local

# ClickHouse receiving data
kubectl exec -n observability deploy/ch-writer -- \
  curl -s "http://clickhouse.observability.svc.cluster.local:8123/?query=SELECT+count()+FROM+otel.otel_logs"

# Alloy pipeline health
# kubectl port-forward -n observability ds/alloy 12345:12345
# Open https://roguequery.local/alloy

# Tailscale proxies
cd k8s/tailscale && make status
```

---

## Exposing a new service

This example walks through a Go app in its own namespace (`myapp`).

### What you get when done

| URL | TLS | Network |
|---|---|---|
| `https://myapp.roguequery.local` | mkcert | LAN |
| `https://myapp.<tailnet>.ts.net` | Let's Encrypt | Tailscale |

The wildcard cert (`*.roguequery.local`) already covers `myapp.roguequery.local`
— no cert changes needed.

### 1. Deploy your Go app

```yaml
# k8s/apps/myapp/deployment.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: myapp
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
  namespace: myapp
spec:
  replicas: 1
  selector:
    matchLabels:
      app: myapp
  template:
    metadata:
      labels:
        app: myapp
    spec:
      containers:
        - name: myapp
          image: your-registry/myapp:latest
          ports:
            - containerPort: 8080   # HTTP
            - containerPort: 2112   # Prometheus /metrics
          env:
            - name: OTEL_EXPORTER_OTLP_ENDPOINT
              value: "http://alloy.observability.svc.cluster.local:4317"
            - name: OTEL_SERVICE_NAME
              value: "myapp"
---
apiVersion: v1
kind: Service
metadata:
  name: myapp
  namespace: myapp
spec:
  selector:
    app: myapp
  ports:
    - name: http
      port: 80
      targetPort: 8080
    - name: metrics
      port: 2112
      targetPort: 2112
```

### 2. ReferenceGrant

Required so the Gateway (in `envoy-gateway-system`) can route to Services
in your new namespace:

```yaml
# k8s/apps/myapp/reference-grant.yaml
apiVersion: gateway.networking.k8s.io/v1beta1
kind: ReferenceGrant
metadata:
  name: gateway-to-myapp
  namespace: myapp
spec:
  from:
    - group: gateway.networking.k8s.io
      kind: Gateway
      namespace: envoy-gateway-system
  to:
    - group: ""
      kind: Service
```

### 3. Envoy HTTPRoute (LAN, HTTPS)

```yaml
# k8s/apps/myapp/httproute.yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: myapp
  namespace: myapp
spec:
  parentRefs:
    - name: cluster-ingress
      namespace: envoy-gateway-system
      sectionName: https       # attach to HTTPS listener
  hostnames:
    - myapp.roguequery.local
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /
      backendRefs:
        - name: myapp
          port: 80
```

### 4. Tailscale Ingress

```yaml
# k8s/apps/myapp/ingress-tailscale.yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: myapp-ts
  namespace: myapp
  annotations:
    tailscale.com/tags: "tag:k8s-ingress"
spec:
  ingressClassName: tailscale
  rules:
    - http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: myapp
                port:
                  number: 80
  tls:
    - hosts:
        - myapp    # → myapp.<tailnet>.ts.net
```

### 5. Add Prometheus scrape target

Edit `k8s/o11y/manifests/alloy-configmap.yaml` — add to `go_services`:

```hcl
prometheus.scrape "go_services" {
  targets = [
    { __address__ = "myapp.myapp.svc.cluster.local:2112" },
  ]
  scrape_interval = "15s"
  forward_to      = [prometheus.remote_write.local.receiver]
}
```

```bash
kubectl apply -f k8s/o11y/manifests/alloy-configmap.yaml
# Alloy hot-reloads — no restart needed
```

### 6. hosts entry + apply

```bash
# Add to /etc/hosts:
# 192.168.86.101  myapp.roguequery.local

kubectl apply -f k8s/apps/myapp/

# Wait for Tailscale proxy (~60s)
kubectl get pods -n tailscale -l tailscale.com/parent-resource=myapp-ts

# Verify
curl -I https://myapp.roguequery.local
```

### Go OTel SDK quickstart

```go
package main

import (
    "context"
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
    "go.opentelemetry.io/otel/sdk/resource"
    "go.opentelemetry.io/otel/sdk/trace"
    semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

// InitTracer wires up the global OTel tracer.
// Reads OTEL_EXPORTER_OTLP_ENDPOINT from env — set in the pod spec
// to http://alloy.observability.svc.cluster.local:4317.
func InitTracer(ctx context.Context, serviceName string) (func(context.Context) error, error) {
    exporter, err := otlptracegrpc.New(ctx)
    if err != nil {
        return nil, err
    }
    res, err := resource.New(ctx,
        resource.WithAttributes(semconv.ServiceName(serviceName)),
    )
    if err != nil {
        return nil, err
    }
    tp := trace.NewTracerProvider(
        trace.WithBatcher(exporter),
        trace.WithResource(res),
    )
    otel.SetTracerProvider(tp)
    return tp.Shutdown, nil
}
```

Alloy enriches every span with `k8s.pod.name`, `k8s.namespace.name`, and
`k8s.node.name` via the `k8sattributes` processor — no SDK changes needed.

---

## Troubleshooting

### Browser shows TLS warning for roguequery.local

The mkcert CA is not trusted on this device. See `k8s/tls/README.md` for
platform-specific CA install instructions.

```bash
# Check cert expiry
cd k8s/tls && make check-expiry
```

### Tailscale proxy not appearing in Machines list

```bash
kubectl logs -n tailscale -l app=operator --tail=50
cd k8s/tailscale && make logs-grafana
```

Common causes: `tagOwners` missing from ACL; OAuth scopes incomplete;
Cilium kube-proxy replacement mode (see Prerequisites §2).

### Grafana not reachable

```bash
# Route accepted by Gateway?
kubectl describe httproute grafana -n observability | grep -A5 Status

# TLS Secret present?
kubectl get secret roguequery-local-tls -n envoy-gateway-system

# Gateway programmed?
kubectl get gateway cluster-ingress -n envoy-gateway-system
```

### ClickHouse not receiving spans

```bash
# ch-writer connected?
kubectl logs -n observability deploy/ch-writer --tail=30

# Alloy pipeline healthy?
# Open https://roguequery.local/alloy — check otelcol.exporter.otlp.ch_writer
```

---

## Known TODOs

**ch-writer**: temporary shim until `otelcol.exporter.clickhouse` lands in
Alloy ([grafana/alloy#3492](https://github.com/grafana/alloy/issues/3492)).
When it lands: delete `manifests/ch-writer-deployment.yaml`, move exporter
config into `manifests/alloy-configmap.yaml`, re-run `make install`.
