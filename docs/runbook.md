# Runbook

Start here when something is broken.

If you know what you are seeing, jump straight to it. If you do not, start with
[Triage](#triage).

| What you are seeing | Go to |
|---------------------|-------|
| Browser warns the certificate is untrusted | [TLS warning in the browser](#tls-warning-in-the-browser) |
| Grafana will not load at all | [Grafana unreachable](#grafana-unreachable) |
| Grafana loads but rejects the password, or shows no login form | [Can't log in to Grafana](#cant-log-in-to-grafana) |
| Hubble UI will not load | [Hubble UI down or not loading](#hubble-ui-down-or-not-loading) |
| Dashboards are empty, or logs and traces are missing | [ClickHouse not receiving data](#clickhouse-not-receiving-data) |
| An app outside the cluster is sending OTLP and nothing arrives | [Telemetry from outside the cluster](#telemetry-from-outside-the-cluster) |
| One metric shows up twice under different `job` labels | [A metric appears twice](#a-metric-appears-twice) |
| A Tailscale proxy never appears in the admin console | [Tailscale proxy missing from Machines](#tailscale-proxy-missing-from-machines) |
| A component was deleted and needs to come back | [Restoring a component](#restoring-a-component) |

## Triage

**Check that the workload exists before debugging ingress.** A component that is
*misbehaving* shows up in the status targets below. A component that has been
*deleted* often does not, because the Services and Ingresses around it survive
on their own and keep resolving, so the symptom presents as a proxy or DNS
fault. The two look identical from a browser.

Observe before acting. Every layer has a status target:

```bash
make setup            # are the host tools still installed?
make cilium-status    # Cilium agent, hubble-relay, hubble-ui
make o11y-status      # all o11y pods, plus the ClickHouse CRs
make o11y-routes      # HTTPRoute accepted/attached status
make tailscale-status # operator and ingress proxy pods, if installed
```

Then read the logs of whatever is unhealthy and describe the failing object:

```bash
kubectl logs -n <ns> <pod> --tail=40
kubectl describe pod|httproute|gateway <name> -n <ns>
```

Once you know the cause, re-run the relevant install target. All of them are
idempotent — see [Restoring a component](#restoring-a-component).

## Symptoms

### TLS warning in the browser

The mkcert CA is not trusted on this device. See
[`k8s/tls/README.md`](../k8s/tls/README.md).

### Grafana unreachable

Confirm the route was accepted and the wildcard cert exists. `make o11y-routes`
runs the first of these across every route:

```bash
kubectl describe httproute grafana -n o11y | grep -A5 Status
kubectl get secret example-local-tls -n envoy-gateway-system
```

The Secret name is your `DOMAIN` with dots replaced by dashes, so it will differ
if you changed `DOMAIN`. A missing Secret means [`make
tls-install`](#restoring-a-component) has not run or did not complete.

### Can't log in to Grafana

**The password is rejected.** The admin credential comes from the `grafana-auth`
Secret in `secrets.yaml`, but Grafana only seeds it when it creates a *new*
database. On an existing PVC the env var is ignored, so a rotated password
applies cleanly and is then rejected at the login form. Reconcile the database
with the Secret:

```bash
make -C k8s/o11y grafana-admin
```

**There is no login form at all.** `disable_login_form` is on. That hides the
form at `/login` too, not just on the home page, leaving the HTTP API as the
only way to authenticate:

```bash
curl -s https://grafana.<tailnet>.ts.net/login | grep -o '"disableLoginForm":[a-z]*'
```

### Hubble UI down or not loading

Cilium's agent can be perfectly healthy while `hubble-relay` and `hubble-ui` are
missing, so check that the workloads exist first:

```bash
kubectl get deploy -n kube-system hubble-relay hubble-ui
kubectl -n kube-system exec ds/cilium -c cilium-agent -- cilium-dbg status | grep Hubble
```

`Hubble: Ok` alongside a missing Deployment means only the UI tier is gone;
restore it with [`make cilium-upgrade`](#restoring-a-component). Then verify the
whole path end to end:

```bash
kubectl -n kube-system exec ds/cilium -c cilium-agent -- \
  hubble observe --server "$(kubectl get svc -n kube-system hubble-relay \
  -o jsonpath='{.spec.clusterIP}'):80" --last 5
```

Use the relay's ClusterIP, not its DNS name: the agent runs in the host network
namespace and cannot resolve cluster DNS.

### ClickHouse not receiving data

Ask ClickHouse what it has, then ask the collector why:

```bash
kubectl exec -n o11y clickhouse-clickhouse-0-0-0 -- clickhouse-client \
  --query "SELECT ServiceName, count() FROM otel.otel_logs GROUP BY ServiceName"
kubectl logs -n o11y ds/otelcol-agent --tail=30
```

Rows back means data is landing and the problem is downstream, in the Grafana
datasource or the panel query. No rows means nothing has been written, so look
to the collector logs for export errors. `make -C k8s/o11y ch-client` opens an
interactive shell on the same pod if you want to dig further.

### Telemetry from outside the cluster

Test the path before debugging client config. A `200` with
`{"partialSuccess":{}}` means the endpoint is fine and the problem is in the
sending app:

```bash
curl -sv http://otlp.<tailnet>.ts.net:4318/v1/traces \
  -X POST -H 'Content-Type: application/json' -d '{}'
kubectl logs -n tailscale -l tailscale.com/parent-resource=otlp-ts --tail=30
```

### A metric appears twice

Something is scraping the same endpoint twice. More than one row back confirms
it:

```bash
kubectl port-forward -n o11y svc/prometheus-server 9090:80 &
curl -sG http://localhost:9090/api/v1/query \
  --data-urlencode 'query=count by (job) (cilium_agent_bootstrap_seconds_count)'
```

Fix it by removing the overlapping job from `values/prometheus.yaml` or from the
collector's `prometheus/scrape`.

> Always pass PromQL with `-G --data-urlencode`. In a bare URL an unencoded `+`
> arrives as a space, so `.+` becomes `. ` and the query returns empty instead
> of erroring.

### Tailscale proxy missing from Machines

```bash
kubectl logs -n tailscale -l app=operator --tail=50
cd k8s/tailscale && make logs-grafana
```

## Restoring a component

Re-run the component's own install target. Each one wraps `helm upgrade
--install`, which is idempotent: unchanged objects are left alone, and anything
deleted out-of-band is recreated. There is no separate "repair" command, and you
do not need to uninstall first.

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

Two components do not follow this pattern cleanly. Read
[Cilium](#cilium-upgrades-are-safe-to-re-run) before re-running an upgrade you
are nervous about, and [ClickHouse](#clickhouse-is-not-a-helm-release) before
touching the database.

### Preview what a target would change

For Helm:

```bash
helm upgrade --install <release> <chart> -n <ns> -f <values> --dry-run=server
```

For the plain-manifest steps, `kubectl diff` prints nothing when a re-run would
be a no-op:

```bash
kubectl diff -f k8s/o11y/manifests/clickhouse-cluster.yaml
```

### Restoring one chart without re-running the rest

Take that chart's line from `k8s/o11y/Makefile` and run it on its own:

```bash
helm upgrade --install grafana grafana/grafana -n o11y \
  -f k8s/o11y/values/grafana.yaml --version 10.5.15
```

### Restoring one workload without touching the release

Re-running a whole chart re-applies every object in it. When a single Deployment
has been deleted and you want to recreate only that one, take it from the
release Helm already has stored:

```bash
helm get manifest cilium -n kube-system > /tmp/cilium.yaml
# extract the Deployment you need, then:
kubectl apply -f /tmp/hubble-deployments.yaml
```

Helm stamps ownership metadata at apply time, so it is **not** in `helm get
manifest` output. Add it back or the next `helm upgrade` fails with *"invalid
ownership metadata"*:

```yaml
metadata:
  annotations:
    meta.helm.sh/release-name: cilium
    meta.helm.sh/release-namespace: kube-system
  labels:
    app.kubernetes.io/managed-by: Helm
```

### Cilium upgrades are safe to re-run

Hubble's TLS material is generated with `hubble.tls.auto.method: helm`, which
sounds like it would rotate on every upgrade. It does not: the chart reuses the
existing `cilium-ca` Secret via a `lookup`, so a re-run renders a byte-identical
CA, leaves the DaemonSet spec unchanged, and neither restarts the Cilium agent
nor drops pod networking.

To confirm before running, record the current fingerprint:

```bash
kubectl get secret -n kube-system hubble-server-certs \
  -o jsonpath='{.data.ca\.crt}' | base64 -d | openssl x509 -noout -fingerprint -sha256
```

Then re-run with `--dry-run=server` and compare the rendered CA against it.
Server-side is required, because `lookup` returns empty on a client-side dry run.

### ClickHouse is not a Helm release

The operator's chart installs a controller and two CRDs, but the database itself
is a `KeeperCluster` + `ClickHouseCluster` pair of CRs. `make o11y-clickhouse`
does both in order, and the two `kubectl wait`s in it are load-bearing: a CR is
rejected outright if its CRD is not yet `Established`, and the operator has to be
up to act on it.

Re-running it is safe. The CRs are applied declaratively, so an unchanged spec is
a no-op and the operator does not restart the server. Run `kubectl diff` first if
you want certainty.

Expect these names, since the operator derives them from the CR name rather than
the chart:

| Resource | Name |
|----------|------|
| Server pod | `clickhouse-clickhouse-0-0-0` |
| Stable ClusterIP Service | `clickhouse-server` |

The headless Service exists too, but the Tailscale operator rejects a headless
backend. `make -C k8s/o11y ch-client` opens a `clickhouse-client` shell without
needing credentials, because the operator writes the `default` password into the
pod's client config.

> `make -C k8s/o11y uninstall` is the one target that destroys data. The CRDs are
> installed with `keep: true` so `helm uninstall` deliberately leaves them, and
> the explicit `kubectl delete crd` cascades into the cluster and its PVC.
