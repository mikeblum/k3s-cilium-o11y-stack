# Setup

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

## Bootstrap

Run these once, in order, on the host:

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
<host-ip>  example.local grafana.example.local hubble.example.local clickhouse.example.local
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

Before first install, generate ClickHouse user credentials and store them in a Secret:

```bash
# Generate two random passwords (one per ClickHouse user)
GRAFANA_PW=$(openssl rand -hex 16)
OTEL_WRITER_PW=$(openssl rand -hex 16)

# Write the secret file (gitignored — never committed)
cat > k8s/o11y/manifests/clickhouse-users-secret.yaml <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: clickhouse-users
  namespace: o11y
type: Opaque
stringData:
  grafana_password: "$GRAFANA_PW"
  otel_writer_password: "$OTEL_WRITER_PW"
EOF

kubectl apply -f k8s/o11y/manifests/clickhouse-users-secret.yaml

# Export GRAFANA_CLICKHOUSE_PASSWORD for the Makefile's envsubst step
export GRAFANA_CLICKHOUSE_PASSWORD="$GRAFANA_PW"
```

Then install the stack:

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
# Grafana and ClickHouse reachable on LAN
curl -I https://grafana.example.local
curl -I https://clickhouse.example.local   # ClickHouse HTTP play UI

# Confirm log ingestion is flowing (run after at least one OTel-instrumented workload has run)
GRAFANA_PW=$(kubectl get secret -n o11y clickhouse-users \
  -o jsonpath='{.data.grafana_password}' | base64 -d)
curl -s "https://clickhouse.example.local/?user=grafana&password=${GRAFANA_PW}&query=SELECT+count()+FROM+otel.otel_logs"
```

> **Note:** `ch-writer` is a temporary otelcol-contrib sidecar that handles the ClickHouse write path until `otelcol.exporter.clickhouse` lands natively in Alloy ([grafana/alloy#3492](https://github.com/grafana/alloy/issues/3492)). When it does: delete `ch-writer-deployment.yaml` and move the exporter block into `alloy-configmap.yaml`.
