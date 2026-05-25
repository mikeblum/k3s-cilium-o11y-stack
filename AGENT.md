# AGENT.md — Deploying the homelab o11y stack

This file guides a Claude Code agent (or any AI agent) through deploying this
stack autonomously on a fresh Linux host. Read it top to bottom; do not skip steps.

---

## What this stack is

Single-node k3s cluster running:
- **Cilium** CNI (kube-proxy replacement, Hubble UI, L2 LoadBalancer)
- **Envoy Gateway** (HTTPS ingress via mkcert wildcard cert)
- **Grafana 13** + **Prometheus** + **Alloy** (DaemonSet) + **ClickHouse** + **ch-writer**

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
make prereqs
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

This installs (in order): ClickHouse → Prometheus → Alloy → ch-writer → Grafana,
then applies all manifests and HTTPRoutes.

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

Requires human interaction (OAuth credentials from Tailscale admin console).

```bash
cd k8s/tailscale
cp values.secret.yaml.example values.secret.yaml
# edit values.secret.yaml: fill in clientId + clientSecret
make install TAILSCALE_TAILNET=my-operator-name
```

See `README.md` § Tailscale for the full admin console setup steps.

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
| `tailscale-install` | ✓ | helm upgrade + kubectl apply |

---

## Common failure modes

### k3s service fails to start
```
Job for k3s.service failed
Error: unrecognized feature gate: GatewayAPI
```
**Fix:** The `GatewayAPI` feature gate was removed in k8s 1.28. Remove it from
`infra/k3s/install.sh` if upgrading from an older version of this repo.

---

### `helm: command not found`
Install helm first: `curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash`

---

### `mkcert not found`
`sudo apt install mkcert` (Ubuntu 20.04+)

---

### ClickHouse auth failure (ch-writer CrashLoopBackOff)
```
default: Authentication failed: password is incorrect
```
The pascaliske ClickHouse chart restricts the default user to localhost. This
stack mounts a `clickhouse-users-override` ConfigMap to open cluster access.
If you see this, verify the ConfigMap and the extraVolumeMounts are applied:
```bash
kubectl get configmap clickhouse-users-override -n o11y
kubectl exec -n o11y clickhouse-0 -- \
  cat /etc/clickhouse-server/users.d/00-network-override.xml
```
If missing, run `make o11y-install` (it applies the ConfigMap before helm upgrade).

---

### Alloy CrashLoopBackOff (config parse error)
```
Error: unrecognized attribute name "labels"
```
The `alloy-configmap.yaml` uses `label {}` blocks (not `labels = [...]`).
Re-apply the configmap and restart:
```bash
kubectl apply -f k8s/o11y/manifests/alloy-configmap.yaml
kubectl rollout restart daemonset/alloy -n o11y
```

---

### Grafana returns HTTP 500 through gateway
Verify the Grafana service port:
```bash
kubectl get svc grafana -n o11y
```
The service exposes port **80**, not 3000. The HTTPRoute must reference port 80.
Check `k8s/o11y/manifests/gateway-routes.yaml` backendRefs.

---

### Gateway not Programmed
```bash
kubectl get gateway cluster-ingress -n envoy-gateway-system \
  -o jsonpath='{.status.conditions[?(@.type=="Programmed")].message}'
```
Common cause: TLS secret missing. Verify:
```bash
kubectl get secret -n envoy-gateway-system | grep tls
```
If missing, re-run `make tls-install`.

---

### Hubble-relay Pending after cilium-install
Normal — the node is `NotReady` until Cilium's DaemonSet fully starts. Wait
~30s; the scheduler will place the pod once the node becomes Ready.

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

# 5. No roguequery references
git grep -i roguequery
# Expected: no output
```
