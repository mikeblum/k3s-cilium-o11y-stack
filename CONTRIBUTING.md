# Contributing

Thanks for taking a look. This is a homelab observability stack, so the bar for
a change is that someone can reproduce it on their own single-node cluster and
understand why it is there six months later.

## Getting a cluster to test on

You need a Linux host with a spare LAN IP range. [SETUP.md](SETUP.md) walks
through the bootstrap; [AGENT.md](AGENT.md) is the same path in a form a coding
agent can follow. Everything runs on one machine, so a spare box, an old
laptop, or a VM with a real kernel all work. Docker Desktop on macOS does not,
because eBPF needs the host kernel.

There is no CI. `make` targets and a cluster are the test suite.

## Where things live

| Path | What belongs there |
|------|--------------------|
| `Makefile` | Top-level targets. Each one delegates to a component Makefile or a script in `infra/`. |
| `infra/<component>/install.sh` | Bootstrap steps that run before the cluster can host anything (k3s, Cilium, Envoy Gateway). |
| `k8s/<component>/Makefile` | Install, uninstall, and status for one namespace. |
| `k8s/<component>/values/` | Helm values, one file per chart. |
| `k8s/<component>/manifests/` | Plain YAML applied with `kubectl apply`. |
| `helm/cilium/values.yaml` | Cilium values, kept at the root because Cilium installs before the `k8s/` layout applies. |
| `docs/` | Prose for humans. See [Documentation](#documentation) below. |

## Conventions

### Pin every chart version

Chart versions live in the component Makefile as variables (`PROM_VERSION`,
`GRAFANA_VERSION`, and so on). An unpinned chart means Helm installs whatever is
newest in the local repo cache, which has already caused an unannounced
mid-deploy drift from prometheus 29.23.0 to 29.24.0. Bump versions in their own
commit so a regression is easy to bisect.

### Keep install targets idempotent

Every `make *-install` wraps `helm upgrade --install` or `kubectl apply`.
Re-running a target is how the project repairs a component, so anything that
only works on a clean cluster is a bug. See
[docs/runbook.md](docs/runbook.md).

### Never commit secrets

`k8s/o11y/secrets.yaml` and `k8s/tailscale/values.secret.yaml` are gitignored.
When you add a credential, add the key to the matching `*.example.yaml` and to
`CH_SECRET_KEYS` in `k8s/o11y/Makefile`, which fails the install early with a
readable message instead of letting a pod crash-loop on a missing key.

### Put scrape targets in the collector

Prometheus's annotation-driven discovery is deliberately off. Adding a job in
both places stores every series twice under two `job` labels. See
[docs/telemetry.md](docs/telemetry.md).

### Comment the why, not the what

The values files carry a lot of comments explaining why a setting is what it is,
usually because the obvious alternative broke something. That context is the
most valuable thing in this repo. A comment restating the YAML key below it is
not.

## Verifying a change

Before opening a PR, check what your change would actually do. For Helm:

```bash
helm upgrade --install <release> <chart> -n <ns> -f <values> --dry-run=server
```

For plain manifests, `kubectl diff` prints nothing when a re-run is a no-op:

```bash
kubectl diff -f k8s/o11y/manifests/clickhouse-cluster.yaml
```

Then apply it for real, confirm the affected pods reach a steady state, and say
in the PR what you ran:

```bash
make o11y-status      # or cilium-status, tailscale-status
make o11y-routes
```

If your change touches telemetry flow, confirm data still lands where it should:

```bash
kubectl exec -n o11y clickhouse-clickhouse-0-0-0 -- clickhouse-client \
  --query "SELECT ServiceName, count() FROM otel.otel_logs GROUP BY ServiceName"
```

`make otel-demo-install` is the quickest way to generate traffic worth looking
at; see [docs/otel-demo.md](docs/otel-demo.md).

## Documentation

Docs are split by what a reader is trying to do:

- [README.md](README.md) introduces the project and indexes everything else.
  Keep it short; detail belongs in `docs/`.
- [SETUP.md](SETUP.md) and [AGENT.md](AGENT.md) cover first-time bootstrap.
- [docs/telemetry.md](docs/telemetry.md) covers where signals land and how to
  add a service.
- [docs/runbook.md](docs/runbook.md) covers everything that happens after
  something breaks.
- [docs/otel-demo.md](docs/otel-demo.md) covers the optional demo workload.

When you change behaviour, update the doc that describes it in the same commit.
Cross-reference with relative links so they work on GitHub and on disk.

## Commits and pull requests

Write commit messages in the imperative mood using
[Conventional Commits](https://www.conventionalcommits.org/):

```
feat(o11y): add a scrape job for the Envoy Gateway controller
fix(grafana): restore the login form and declare the admin credential
docs: explain how to restore an individual service
chore(deps): bump the prometheus chart to 29.25.0
```

In the PR description, say what you changed, why, and what you ran to verify it.
If you hit something surprising along the way, put it in a comment next to the
setting it explains. That is usually the most useful part of the change.
