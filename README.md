# k3s-cilium-o11y-stack 🔭

Single-node homelab observability template — k3s · Cilium · Envoy Gateway · Grafana · ClickHouse · Prometheus · Alloy.

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

## Services & Routes

Every service gets its own subdomain via Envoy Gateway (LAN) or the Tailscale operator (remote).

| Service | LAN URL | Tailscale URL | Routing | TLS |
|---------|---------|---------------|---------|-----|
| **Grafana** | `https://grafana.example.local` | `https://grafana.<tailnet>.ts.net` | host-based | mkcert wildcard / Let's Encrypt |
| **Hubble UI** | `https://hubble.example.local` | `https://hubble.<tailnet>.ts.net` | host-based | mkcert wildcard / Let's Encrypt |
| **Alloy debug** | `https://example.local/alloy` | — LAN only | path-based | mkcert wildcard |

- **LAN** — wildcard cert covers any `*.example.local` subdomain; add a single `/etc/hosts` entry per desktop pointing to the LoadBalancer IP.
- **Tailscale** — Let's Encrypt cert auto-provisioned by the Tailscale operator; no hosts-file changes, works anywhere on your tailnet.
- **Adding a service** — see [Adding a service](#adding-a-service) for the HTTPRoute + optional Tailscale Ingress pattern.

## Stack

| Component | Layer | Role |
|-----------|-------|------|
| [k3s](https://k3s.io) | Cluster | Lightweight Kubernetes; runs with flannel, kube-proxy, and Traefik disabled to make room for Cilium + Envoy Gateway |
| [Cilium](https://cilium.io) | Networking | eBPF CNI — pod networking, kube-proxy replacement, and L2 LoadBalancer IP pool |
| [Hubble UI](https://docs.cilium.io/en/stable/gettingstarted/hubble/) | Networking | Real-time network flow visualization built into Cilium |
| [Envoy Gateway](https://gateway.envoyproxy.io) | Ingress | Kubernetes Gateway API controller; routes HTTPS subdomains to in-cluster services |
| [mkcert](https://github.com/FiloSottile/mkcert) | TLS | Local CA; issues a `*.example.local` wildcard cert for LAN HTTPS |
| [Prometheus](https://prometheus.io) | Observability | Metrics scraping and time-series storage |
| [Grafana Alloy](https://grafana.com/oss/alloy-opentelemetry-collector/) | Observability | DaemonSet telemetry collector; receives OTLP from apps, scrapes Cilium + Prometheus targets |
| [ClickHouse](https://clickhouse.com) | Observability | OLAP database; backend store for logs, traces, and metrics |
| ch-writer | Observability | Temporary OTLP → ClickHouse bridge (removed once Alloy ships a native ClickHouse exporter — [grafana/alloy#3492](https://github.com/grafana/alloy/issues/3492)) |
| [Grafana](https://grafana.com) | Observability | Dashboards and visualization over Prometheus + ClickHouse |
| [Tailscale Operator](https://tailscale.com/kb/1236/kubernetes-operator) | Remote access | *(optional)* Exposes services to your tailnet with auto-provisioned Let's Encrypt TLS |

---

## Prerequisites

Tools required on the host before bootstrapping:

| Tool | Install |
|------|---------|
| `helm` | `curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 \| bash` |
| `mkcert` | `sudo apt install mkcert` |
| `envsubst` | `sudo apt install gettext` |

`kubectl` is provided by k3s (`/usr/local/bin/kubectl` → `k3s`). Run after `make k3s-install`.

Check all at once:
```bash
make setup
```

---

## Bootstrap

Run these once, in order, on the host.

> **Optional: Tailscale remote access** — steps 1–5 stand up the full stack on your LAN. If you also want the `https://grafana.<tailnet>.ts.net` / `https://hubble.<tailnet>.ts.net` URLs accessible outside the LAN, continue to [Step 6: Tailscale](#6-tailscale) after step 5.

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

> **Complete the admin console steps before running `make install`** — the OAuth client requires the tags to exist in the ACL first.

**6a. Tailscale admin console** (one-time, in this order)

1. **Settings → DNS** → enable MagicDNS and HTTPS.

2. **Access Controls** → merge the following blocks into your tailnet ACL
   (see `k8s/tailscale/acl-snippet.json` for the full ready-to-paste snippet):

   ```json
   "tagOwners": {
     "tag:k8s-operator": ["autogroup:admin"],
     "tag:k8s-ingress":  ["autogroup:admin"]
   },
   "acls": [
     { "action": "accept", "src": ["autogroup:members"], "dst": ["tag:k8s-ingress:*"] }
   ],
   "nodeAttrs": [
     { "target": ["tag:k8s-ingress", "tag:k8s-operator"], "attr": ["noExpiry"] }
   ]
   ```

   `tagOwners` must exist before you can create an OAuth client with these tags.
   `noExpiry` prevents proxy pods from going offline when key rotation hits.

3. **Settings → OAuth clients** → create a client with:
   - Scopes: Devices → Core / Auth Keys / Services → **Write**
   - Tag: `tag:k8s-operator`
   - Save the `clientId` and `clientSecret`

**6b. Install the operator**

> **Cilium compatibility:** `make install` automatically annotates the `tailscale` namespace
> with `io.cilium/no-track-port="0"` after Helm creates it. This is required when Cilium
> runs in kube-proxy replacement mode — without it, Cilium's eBPF connection tracking
> intercepts proxy pod traffic and connections hang indefinitely.

```bash
cd k8s/tailscale
cp values.secret.yaml.example values.secret.yaml
# fill in clientId + clientSecret from step 6a
make install
```

Verify proxy pods come up (one per Ingress):

```bash
cd k8s/tailscale && make status
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
