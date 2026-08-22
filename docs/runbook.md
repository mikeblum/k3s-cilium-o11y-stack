# Runbook

Start here when something is broken. [Triage](#triage) narrows down what is
wrong, [Symptoms](#symptoms) maps what you are seeing to a fix, and
[Restoring a component](#restoring-a-component) covers putting something back
after it has been deleted or has drifted.

## Triage

Observe before acting. Every layer has a status target:

```bash
make setup            # are the host tools still installed?
make cilium-status    # Cilium agent, hubble-relay, hubble-ui
make o11y-status      # all o11y pods, plus the ClickHouse CRs
make o11y-routes      # HTTPRoute accepted/attached status
make tailscale-status # operator and ingress proxy pods, if installed
```

From there, read the logs of whatever is unhealthy
(`kubectl logs -n <ns> <pod> --tail=40`) and describe the failing object
(`kubectl describe pod|httproute|gateway …`). Re-run the relevant install
target once you know the cause; all of them are idempotent.

A component that is *misbehaving* usually shows up in these status targets. A
component that has been *deleted* often does not, because the Services and
Ingresses around it survive on their own and keep resolving, so the symptom
presents as a proxy or DNS fault. The two look alike from a browser. When a page
will not load, check that the workload exists before debugging ingress.

## Symptoms

### TLS warning in the browser

The mkcert CA is not trusted on this device. See
[`k8s/tls/README.md`](../k8s/tls/README.md).

### Grafana unreachable

```bash
kubectl describe httproute grafana -n o11y | grep -A5 Status
kubectl get secret example-local-tls -n envoy-gateway-system   # dots-to-dashes of your DOMAIN
```

### Can't log in to Grafana

The admin credential comes from the `grafana-auth` Secret in `secrets.yaml`, but
Grafana only seeds it when it creates a *new* database. On an existing PVC the
env var is ignored, so a rotated password applies cleanly and is then rejected
at the login form. Reconcile the database with the Secret:

```bash
make -C k8s/o11y grafana-admin
```

If the form itself is missing rather than rejecting you, `disable_login_form` is
on. That hides the form at `/login` too, not just on the home page, leaving the
HTTP API as the only way to authenticate:

```bash
curl -s https://grafana.<tailnet>.ts.net/login | grep -o '"disableLoginForm":[a-z]*'
```

### Hubble UI down, or `hubble.<tailnet>.ts.net` not loading

Cilium's agent can be perfectly healthy while `hubble-relay` and `hubble-ui` are
missing, so check that the workloads exist before debugging ingress:

```bash
kubectl get deploy -n kube-system hubble-relay hubble-ui
kubectl -n kube-system exec ds/cilium -c cilium-agent -- cilium-dbg status | grep Hubble
```

`Hubble: Ok` with a missing Deployment means only the UI tier is gone; restore
it with [`make cilium-upgrade`](#restoring-a-component). Verify the whole path
end to end:

```bash
kubectl -n kube-system exec ds/cilium -c cilium-agent -- \
  hubble observe --server "$(kubectl get svc -n kube-system hubble-relay \
  -o jsonpath='{.spec.clusterIP}'):80" --last 5
```

Use the relay's ClusterIP, not its DNS name: the agent runs in the host network
namespace and cannot resolve cluster DNS.

### ClickHouse not receiving data

```bash
kubectl logs -n o11y ds/otelcol-agent --tail=30
kubectl exec -n o11y clickhouse-clickhouse-0-0-0 -- clickhouse-client \
  --query "SELECT ServiceName, count() FROM otel.otel_logs GROUP BY ServiceName"
```

### Telemetry from outside the cluster not arriving

Test the path before debugging client config. A `200` with
`{"partialSuccess":{}}` means the endpoint is fine:

```bash
curl -sv http://otlp.<tailnet>.ts.net:4318/v1/traces \
  -X POST -H 'Content-Type: application/json' -d '{}'
kubectl logs -n tailscale -l tailscale.com/parent-resource=otlp-ts --tail=30
```

### A metric appears twice under different `job` labels

Something is scraping the same endpoint twice. More than one row back confirms
it; remove the overlapping job from `values/prometheus.yaml` or the collector's
`prometheus/scrape`:

```bash
kubectl port-forward -n o11y svc/prometheus-server 9090:80 &
curl -sG http://localhost:9090/api/v1/query \
  --data-urlencode 'query=count by (job) (cilium_agent_bootstrap_seconds_count)'
```

> Always pass PromQL with `-G --data-urlencode`. In a bare URL an unencoded `+`
> arrives as a space, so `.+` becomes `. ` and the query returns empty instead
> of erroring.

### Tailscale proxy missing from Machines

```bash
kubectl logs -n tailscale -l app=operator --tail=50
cd k8s/tailscale && make logs-grafana
```

## Restoring a component

Every component is reconciled by re-running its own install target. Each one
wraps `helm upgrade --install`, which is idempotent: unchanged objects are left
alone, and anything deleted out-of-band is recreated. There is no separate
"repair" command, and you do not need to uninstall first.

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

To restore a single o11y component without re-running the rest, take that
chart's line from `k8s/o11y/Makefile` and run it on its own:

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

### Re-running a Cilium upgrade is safe

Hubble's TLS material is generated with `hubble.tls.auto.method: helm`, which
sounds like it would rotate on every upgrade. It does not: the chart reuses the
existing `cilium-ca` Secret via a `lookup`, so a re-run renders a byte-identical
CA, leaves the DaemonSet spec unchanged, and neither restarts the Cilium agent
nor drops pod networking. Confirm before running:

```bash
kubectl get secret -n kube-system hubble-server-certs \
  -o jsonpath='{.data.ca\.crt}' | base64 -d | openssl x509 -noout -fingerprint -sha256
```

Re-run with `--dry-run=server`, which is required because `lookup` returns empty
on a client-side dry run, then compare the rendered CA to that fingerprint.

### Restoring ClickHouse

ClickHouse is **not** a Helm release, so it does not follow the pattern above.
The operator's chart installs a controller and two CRDs; the database itself is
a `KeeperCluster` + `ClickHouseCluster` pair of CRs. `make o11y-clickhouse` does
both in order, and the two `kubectl wait`s in it are load-bearing: a CR is
rejected outright if its CRD is not yet `Established`, and the operator has to
be up to act on it.

Re-running it is safe. The CRs are applied declaratively, so an unchanged spec
is a no-op and the operator does not restart the server. Run `kubectl diff`
first if you want certainty.

Expect these names, since the operator derives them from the CR name rather than
the chart: pod `clickhouse-clickhouse-0-0-0`, and stable ClusterIP
`clickhouse-server` (the headless Service exists too, but the Tailscale operator
rejects a headless backend). `make -C k8s/o11y ch-client` opens a
`clickhouse-client` shell without needing credentials, because the operator
writes the `default` password into the pod's client config.

> `make -C k8s/o11y uninstall` is the one target that destroys data. The CRDs
> are installed with `keep: true` so `helm uninstall` deliberately leaves them,
> and the explicit `kubectl delete crd` cascades into the cluster and its PVC.
