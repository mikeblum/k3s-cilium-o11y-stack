# AGENT.md — Deploying the homelab o11y stack

This file guides a Claude Code agent (or any AI agent) through deploying this
stack autonomously on a fresh Linux host. Read it top to bottom; do not skip steps.

---

## What this stack is

Single-node k3s cluster running:
- **Cilium** CNI (kube-proxy replacement, Hubble UI, L2 LoadBalancer)
- **Envoy Gateway** (HTTPS ingress via mkcert wildcard cert)
- **Grafana 13** + **Prometheus** + **OTel Collector** (DaemonSet) + **ClickHouse** + **node-exporter**

**Linux only.** Tested on Pop!_OS 24.04 / Ubuntu 24.04 (kernel 6.x). Not tested on macOS or Windows.

---

## Variables

All are auto-detected; override any on the command line.

| Variable | Default (auto-detected) | Override example |
|----------|------------------------|-----------------|
| `DOMAIN` | hostname minus first label (`k8s.mylab.local` → `mylab.local`) | `make tls-install DOMAIN=mylab.local` |
| `NODE_IP` | primary outbound IP (`ip route get 1`) | `make k3s-install NODE_IP=192.168.1.10` |
| `NODE_CIDR` | `NODE_IP/26` | `make cilium-lb NODE_CIDR=192.168.1.192/26` |
| `CILIUM_VERSION` | `1.17.3` | `make cilium-install CILIUM_VERSION=1.17.4` |
| `ENVOY_GATEWAY_VERSION` | `v1.8.0` | `make gateway-install ENVOY_GATEWAY_VERSION=v1.9.0` |

Run `make help` to see current resolved values before executing any target.

---

## Prerequisites

Install before running any `make` targets.

```bash
# helm — required by cilium-install, gateway-install, o11y-install
curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash

# mkcert — required by tls-install
sudo apt install mkcert          # Ubuntu/Debian/Pop!_OS
# or: brew install mkcert        # macOS (not officially supported)

# envsubst — required by cilium-lb, gateway-apply, o11y-install
sudo apt install gettext
```

Check all at once:
```bash
make setup
```

`kubectl` is installed by k3s — it is available after `make k3s-install`.

---

## Bootstrap order (run in order, once, on the host)

Every step prints the next command to run. If a step fails, see the
Failure Modes section below before retrying.

### 1 — k3s

```bash
sudo make k3s-install
```

**Success:** node appears in `kubectl get nodes` with status `Ready` (may take ~30s).
Note: the node will be `NotReady` until Cilium (step 2) is installed.

**Idempotent:** yes — re-running upgrades k3s in place.

---

### 2 — Cilium

```bash
sudo make cilium-install
```

**Success:**
```
daemon set "cilium" successfully rolled out
```
Then verify:
```bash
kubectl exec -n kube-system ds/cilium -- curl -s http://localhost:9962/metrics | head -5
```
Should return Prometheus metric lines.

**Idempotent:** yes — `helm upgrade --install`.

---

### 3 — Cilium LB pool

```bash
make ip   # print your primary NIC IP, e.g. 192.168.1.10
# Pick a free /26 range in your LAN — check with: arp-scan or ping sweep
sudo make cilium-lb NODE_CIDR=192.168.1.192/26
```

**Success:** `ciliumloadbalancerippool.cilium.io/lan-pool created`

The first IP in NODE_CIDR will be assigned to the Envoy Gateway LoadBalancer service.
Make sure NODE_CIDR does not overlap with DHCP ranges or other static hosts.

**Idempotent:** yes — `kubectl apply`.

---

### 4 — Envoy Gateway controller

```bash
sudo make gateway-install
```

**Success:**
```
deployment "envoy-gateway" successfully rolled out
```

This creates the `envoy-gateway-system` namespace. Required before tls-install.

**Idempotent:** yes — `helm upgrade --install`.

---

### 5 — TLS (mkcert wildcard cert)

```bash
make tls-install
```

DOMAIN is auto-detected from the system hostname. To override:
```bash
make tls-install DOMAIN=mylab.local
```

**Success:**
```
[mkcert] Secret applied.
```
Verify:
```bash
kubectl get secret $(make -s -C k8s/tls check-expiry 2>/dev/null; \
  hostname | sed 's/[^.]*\.//; s/\./-/g')-tls -n envoy-gateway-system
# e.g.: kubectl get secret mylab-local-tls -n envoy-gateway-system
```

The CA cert is at `~/.local/share/mkcert/rootCA.pem`. Distribute this to any
device that needs to trust HTTPS on your domain. See `k8s/tls/README.md`.

**Idempotent:** yes — `kubectl apply --dry-run=client`.

---

### 6 — Envoy Gateway routes

```bash
make gateway-apply
```

**Success:**
```
gateway.gateway.networking.k8s.io/cluster-ingress created
```
Verify:
```bash
kubectl get gateway cluster-ingress -n envoy-gateway-system \
  -o jsonpath='{.status.conditions[?(@.type=="Programmed")].status}'
# Expected: True (may take 10-15s)
```

**Idempotent:** yes — `kubectl apply`.

---

### 7 — o11y stack

```bash
make o11y-install
```

This installs (in order): ClickHouse → Prometheus → node-exporter → OTel Collector → Grafana,
then applies all manifests and HTTPRoutes. It requires `k8s/o11y/secrets.yaml`
(gitignored — copy from `secrets.example.yaml`) for the ClickHouse passwords.

**Success:**
```bash
make o11y-status
# Expected: all pods 1/1 or 2/2 Running
```

Verify data flow end-to-end:
```bash
kubectl exec -n o11y clickhouse-0 -- \
  clickhouse-client --query "SHOW TABLES FROM otel"
# Expected: otel_logs, otel_traces, otel_metrics_* tables listed
```

Verify Grafana is reachable (add to /etc/hosts first — see below):
```bash
curl -Lk https://grafana.<your-domain>
# Expected: HTTP 200 (Grafana login page)
```

**Idempotent:** yes — all `helm upgrade --install` + `kubectl apply`.

---

### 8 — /etc/hosts (LAN access)

Add to `/etc/hosts` on any desktop that needs to reach the services:
```
<LB-IP>  <domain> grafana.<domain> hubble.<domain>
```

Get the LB IP:
```bash
kubectl get gateway cluster-ingress -n envoy-gateway-system \
  -o jsonpath='{.status.addresses[0].value}'
```

---

### 9 — Tailscale (optional, manual)

Requires human interaction — OAuth credentials must be obtained from the Tailscale admin
console before running `make install`. The admin console steps (enable MagicDNS, update
ACL, create OAuth client) are documented in `SETUP.md` § 6a and **must be done
first**.

```bash
cd k8s/tailscale
cp values.secret.yaml.example values.secret.yaml
# edit values.secret.yaml: fill in clientId + clientSecret from SETUP.md § 6a
make install
```

**Success:** proxy pods appear in `make status` — one per Ingress (grafana, hubble) plus one
per exposed Service (otlp).
May take 30–60 s on first run while Let's Encrypt certs are issued.

---

### 10 — OTel demo app (optional)

```bash
make otel-demo-install
```

Deploys the OpenTelemetry demo (~20 microservices + load generator) into its own
`otel-demo` namespace. Its collector and bundled backends are disabled; services
send OTLP straight to `otelcol.o11y`. This is what puts real traces in
ClickHouse — without it `otel_traces` stays empty.

Requires step 7 first: the demo has no collector of its own, so `otelcol.o11y`
must already be up or every service logs export failures.

**Success:**
```bash
make otel-demo-status          # 25 pods Running
kubectl exec -n o11y clickhouse-0 -- clickhouse-client --query \
  "SELECT count() FROM otel.otel_traces"
# Expected: non-zero and climbing within ~2 min of the pods going Ready
```

Also confirm the span-derived metrics reached Prometheus, which is what the
provisioned "OTel Demo → Spanmetrics" dashboard reads:
```bash
kubectl port-forward -n o11y svc/prometheus-server 9090:80 &
curl -sG http://localhost:9090/api/v1/query \
  --data-urlencode 'query=sum by (service_name) (traces_span_metrics_calls_total)'
# Expected: one series per demo service
```

Allow a few minutes on first run — 25 images to pull.

**Idempotent:** yes — `helm upgrade --install` + `kubectl apply`.

---

## Idempotency summary

| Target | Safe to re-run? | Notes |
|--------|----------------|-------|
| `k3s-install` | ✓ | upgrades k3s in place |
| `cilium-install` | ✓ | helm upgrade |
| `cilium-lb` | ✓ | kubectl apply |
| `gateway-install` | ✓ | helm upgrade |
| `tls-install` | ✓ | kubectl apply --dry-run |
| `gateway-apply` | ✓ | kubectl apply |
| `o11y-install` | ✓ | helm upgrade + kubectl apply |
| `otel-demo-install` | ✓ | helm upgrade |
| `tailscale-install` | ✓ | helm upgrade + kubectl apply |

---

## When something looks wrong

Don't guess — use the status targets to observe before acting:

```bash
make setup            # missing tools?
make cilium-status    # Cilium + Hubble pods
make o11y-status      # all o11y pods (look for CrashLoopBackOff / ErrImagePull)
make o11y-routes      # HTTPRoute accepted/attached status
make tailscale-status # operator + ingress proxy pods (if installed)
```

From there: read pod logs (`kubectl logs -n <ns> <pod> --tail=40`),
describe the failing resource (`kubectl describe pod/httproute/gateway …`),
and re-run the relevant `make <step>` once the root cause is clear.
All install targets are idempotent — re-running is always safe.

---

## Post-deploy verification checklist

```bash
# 1. All o11y pods Running
make o11y-status

# 2. Gateway Programmed
kubectl get gateway cluster-ingress -n envoy-gateway-system

# 3. ClickHouse tables created
kubectl exec -n o11y clickhouse-0 -- \
  clickhouse-client --query "SHOW TABLES FROM otel"

# 4. Grafana reachable (via LB IP with Host header)
LB=$(kubectl get gateway cluster-ingress -n envoy-gateway-system \
  -o jsonpath='{.status.addresses[0].value}')
curl -skL -H "Host: grafana.$(hostname | sed 's/[^.]*\.//')" https://$LB \
  -o /dev/null -w "%{http_code}\n"
# Expected: 200
```
