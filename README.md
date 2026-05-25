# Local O11y Stack 🔭

k3s + Cilium + Envoy Gateway + ClickHouse o11y stack.

```
                    ┌─────────────────────────── k3s cluster ─────────────────────────────────────┐
                    │ ┌── kube-system ──────────────┐  ┌── o11y ──────────────────────────────┐   │
 ┌──────────────┐   │ │                             │  │                                      │   │
 │ LAN          │───┼─┼─▶ Envoy Gateway        ─────┼──┼──────────────────────▶ Grafana       │   │
 │ HTTPS/mkcert │   │ │   → Grafana · Hubble UI     │  │                       ▲    ▲         │   │
 └──────────────┘   │ │            │                │  │                       │    │         │   │
                    │ │            ▼                │  │  ClickHouse ──────────┘    │         │   │
 ┌──────────────┐   │ │        Hubble UI            │  │      ▲                     │         │   │
 │ Tailscale    │───┼─┼─▶ Tailscale Operator   ─────┼──┼──────┼─────────── Prometheus         │   │
 │ HTTPS/LE     │   │ │   → Grafana · Hubble UI     │  │      │                    ▲          │   │
 └──────────────┘   │ │                             │  │  ch-writer          remote_write     │   │
                    │ │  Cilium + Hubble ────scrape──┼──┼──▶ Alloy (DaemonSet)               │   │
                    │ │  :9962 · :9965              │  │       :4317/:4318 ◀── apps (OTLP)   │   │
                    │ └─────────────────────────────┘  └──────────────────────────────────────┘   │
                    └─────────────────────────────────────────────────────────────────────────────┘
```

| URL | Network |
|---|---|
| `https://grafana.example.local` | LAN |
| `https://hubble.example.local` | LAN |
| `https://grafana.<tailnet>.ts.net` | Tailscale |
| `https://hubble.<tailnet>.ts.net` | Tailscale |

---

## Bootstrap

Run these once, in order, on the host.

### 1. k3s + Cilium

From the repo root:

```bash
make k3s-install    # installs k3s with flannel/kube-proxy/traefik disabled
make cilium-install # installs Cilium via Helm (Hubble + Envoy proxy enabled)
make cilium-lb      # applies LB IP pool + L2 announcement policy
```

`cilium-lb` derives `NODE_CIDR` from your primary NIC's IP (auto-detected). Override if needed:

```bash
make cilium-lb NODE_CIDR=192.168.1.192/26   # a free /26 range on your LAN
```

Verify Cilium is up before continuing:

```bash
kubectl exec -n kube-system ds/cilium -- curl -s http://localhost:9962/metrics | head -5
```

### 2. Envoy Gateway

```bash
make gateway-install   # installs the Envoy Gateway controller into envoy-gateway-system
```

This creates the `envoy-gateway-system` namespace — required before TLS setup.

### 3. TLS

```bash
make tls-install
```

Requires [mkcert](https://github.com/FiloSottile/mkcert) on the host. Generates a `*.example.local` wildcard cert and stores it as a Secret in `envoy-gateway-system`. See `k8s/tls/README.md` for how to trust the CA on other devices.

Add to `/etc/hosts` on your desktop:

```
<host-ip>  example.local grafana.example.local hubble.example.local
```

### 4. Envoy Gateway routes

```bash
make gateway-apply   # applies GatewayClass + cluster-ingress Gateway
```

Verify:

```bash
kubectl get gateway cluster-ingress -n envoy-gateway-system \
  -o jsonpath='{.status.conditions[?(@.type=="Programmed")].status}'
# Expected: True
```

### 5. o11y stack

```bash
make o11y-install
make o11y-status
```

Expected steady state:

```
alloy-xxxxx          1/1  Running   # DaemonSet — one per node
ch-writer-xxxxx      1/1  Running
clickhouse-0         1/1  Running
grafana-xxxxx        1/1  Running
prometheus-server    1/1  Running
```

### 6. Tailscale

```bash
cd k8s/tailscale
cp values.secret.yaml.example values.secret.yaml
# fill in OAuth clientId + clientSecret
make install
```

First-time Tailscale admin console setup:
1. Settings → DNS → enable MagicDNS + HTTPS
2. OAuth client → Devices (Core/Auth Keys/Services) write, tag `tag:k8s-operator`
3. Merge `acl-snippet.json` into your tailnet ACL

Also add `noExpiry` to your ACL for operator-managed devices to prevent proxy pods going offline:

```json
"nodeAttrs": [{ "target": ["tag:k8s-ingress", "tag:k8s-operator"], "attr": ["noExpiry"] }]
```

If Cilium runs in kube-proxy replacement mode, annotate the namespace first:

```bash
kubectl annotate namespace tailscale io.cilium/no-track-port="0"
```

---

## Verify

```bash
curl -I https://grafana.example.local

kubectl exec -n o11y deploy/ch-writer -- \
  curl -s "http://clickhouse.o11y.svc.cluster.local:8123/?query=SELECT+count()+FROM+otel.otel_logs"
```

Alloy debug UI: `https://example.local/alloy`

---

## Adding a service

The wildcard cert covers any `*.example.local` subdomain — no cert changes needed.

**1.** Set OTLP env vars in your pod spec:

```yaml
env:
  - name: OTEL_EXPORTER_OTLP_ENDPOINT
    value: "http://alloy.o11y.svc.cluster.local:4317"
  - name: OTEL_SERVICE_NAME
    value: "myapp"
```

**2.** Add a scrape target to `k8s/o11y/manifests/alloy-configmap.yaml`:

```hcl
prometheus.scrape "go_services" {
  targets         = [{ __address__ = "myapp.myapp.svc.cluster.local:2112" }]
  scrape_interval = "15s"
  forward_to      = [prometheus.remote_write.local.receiver]
}
```

```bash
kubectl apply -f k8s/o11y/manifests/alloy-configmap.yaml  # hot-reloads, no restart
```

**3.** Add an HTTPRoute (LAN) and Tailscale Ingress — see `k8s/tailscale/manifests/ingress-grafana.yaml` and `k8s/o11y/manifests/gateway-routes.yaml` for examples.

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
kubectl logs -n o11y deploy/ch-writer --tail=30
```

**Tailscale proxy missing from Machines:**
```bash
kubectl logs -n tailscale -l app=operator --tail=50
cd k8s/tailscale && make logs-grafana
```

> **Note:** `ch-writer` is a temporary otelcol-contrib sidecar that handles the ClickHouse write path until `otelcol.exporter.clickhouse` lands natively in Alloy ([grafana/alloy#3492](https://github.com/grafana/alloy/issues/3492)). When it does: delete `ch-writer-deployment.yaml` and move the exporter block into `alloy-configmap.yaml`.
