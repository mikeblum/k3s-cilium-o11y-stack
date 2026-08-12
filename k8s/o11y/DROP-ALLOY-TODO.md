# Drop Alloy → vanilla OTel Collector + ClickHouse

Goal: remove Grafana Alloy and the `ch-writer` shim, replacing both with a single
standard `otel/opentelemetry-collector-contrib` deployment that writes to ClickHouse
via the **native `clickhouse` exporter** — as close as possible to the plain
docker-compose reference config we already run for Claude Code telemetry elsewhere.

## Locked decisions
- **Raw manifest** (not Helm). Evolve `ch-writer-deployment.yaml` into the full
  collector — it's already a working Deployment + ConfigMap + Service; we just add the
  OTLP receiver, `k8sattributes` + `batch`, and a ClusterRole. No chart-version drift,
  no `helm template` indirection, config maps 1:1 to the docker-compose reference.
- **Dedicated `otel` ClickHouse user** + Secret for its password (parity with the
  reference repo), replacing the passwordless default-user path.
- **Option B** — Prometheus scrapes Cilium/Hubble/operator directly; the collector is
  pure OTLP → ClickHouse.

## The key insight

`ch-writer` is **already** a vanilla otelcol-contrib with the native ClickHouse
exporter (`k8s/o11y/manifests/ch-writer-deployment.yaml`). The "weird shim" is only
that Alloy forwards OTLP to it instead of writing directly. So this migration is not a
rewrite — it's a **collapse of Alloy + ch-writer into one collector**, moving the
`k8sattributes` + `batch` processors and the OTLP receiver onto the same otelcol
process that already owns the ClickHouse exporter.

What Alloy does today, and where each job lands after the migration:

| Alloy job today | After dropping Alloy |
|---|---|
| OTLP receiver `:4317/:4318` | otel-collector `otlp` receiver (unchanged) |
| `k8sattributes` processor | otel-collector `k8sattributes` processor (contrib, unchanged) |
| `batch` processor | otel-collector `batch` processor (unchanged) |
| forward OTLP → ch-writer → ClickHouse | otel-collector **native `clickhouse` exporter** (no more hop) |
| scrape cilium-agent `:9962`, hubble `:9965`, cilium-operator `:9963` → Prometheus | **decision below** — Prometheus scrapes directly (recommended) |
| scrape Alloy self `:12345` → Prometheus (mixin dashboards) | otel-collector self-metrics `:8888` → Prometheus (new dashboard) |

## Decision needed: where does Cilium/Hubble metric scraping go?

The Cilium/Hubble Grafana dashboards (gnetId 16611/16612/16613/18015) are **PromQL**
against the `prometheus` datasource. Those metrics must keep flowing into Prometheus;
ClickHouse can't back those dashboards without a full rewrite. Two options:

- **Option B — Prometheus scrapes Cilium directly (RECOMMENDED).** Add `scrape_configs`
  to `values/prometheus.yaml` for cilium-agent (pod/endpoints role), hubble, and
  cilium-operator. The collector then does **only** OTLP → ClickHouse, making its
  config essentially identical to the docker-compose reference. Cleanest separation:
  metrics-for-dashboards (Prometheus) fully decoupled from app OTLP telemetry
  (collector → ClickHouse). Cilium docs publish these scrape_configs directly.
  - Removes the DaemonSet-for-`localhost` requirement → collector is a single Deployment.

- **Option A — Collector scrapes and remote-writes to Prometheus (lower churn).**
  Collector runs a `prometheus` receiver (k8s SD, endpoints role) + `prometheusremotewrite`
  exporter → Prometheus, mirroring Alloy's current flow. Keeps one component owning
  scraping, but keeps the collector further from "vanilla" and needs k8s SD tuning.

**This TODO assumes Option B.** Switch the scraping tasks if we pick A.

---

## Tasks

### 1. Build the collector manifest (raw)
- [ ] Rename `ch-writer-deployment.yaml` → `otel-collector-deployment.yaml`; rename the
  Deployment/ConfigMap/Service `ch-writer` → `otel-collector`. Add a ServiceAccount.
- [ ] Config (mirror the docker-compose reference):
  - `otlp` receiver grpc `:4317` + http `:4318`
  - processors: `memory_limiter`, `k8sattributes`, `batch`
  - `clickhouse` exporter → `tcp://clickhouse.o11y.svc.cluster.local:9000`, db `otel`,
    tables `otel_logs` / `otel_traces` / `otel_metrics`, `create_schema: true`,
    `compress: lz4`, `async_insert: true`
  - pipelines: traces / logs / metrics → clickhouse
  - drop the `otlp_http/toast` upstream forwarder (not used here)
- [ ] Name the Service `otel-collector` (apps' new OTLP endpoint).

### 2. RBAC for k8sattributes
- [ ] Hand-written ServiceAccount + ClusterRole + ClusterRoleBinding: `get/list/watch`
  on pods, namespaces, nodes. Replaces the RBAC the Alloy chart created
  (`values/alloy.yaml: rbac.create`). Ship it in the same manifest file as the collector.

### 3. ClickHouse auth — dedicated `otel` user
- [ ] Add the `otel` user to the ClickHouse users override (network-reachable, password via
  `password_sha256_hex`), mirroring the reference repo's `clickhouse-users.xml`.
- [ ] Create a Secret holding `CLICKHOUSE_PASSWORD`; wire it into the collector env and
  the `clickhouse` exporter (`username: otel`, `password: ${env:CLICKHOUSE_PASSWORD}`).
- [ ] Point Grafana's ClickHouse datasource at the `otel` user too
  (`grafana-datasources-configmap.yaml` — currently no creds); supply its password via
  the datasource `secureJsonData`.
- [ ] Confirm table names + `otel` database still match the datasource
  (`otel_logs`, `otel_traces`). No change expected.
- [ ] Once collector + Grafana both use `otel`, nothing needs the **default** user opened
  to the network. Re-lock it: either replace `clickhouse-users-override.yaml` so it adds
  the `otel` user (network `::/0`) and drops the `<default><networks>::/0` opener, or
  delete the override entirely and let the chart's localhost-only default stand. The
  `otel` user's own network stanza is what cross-pod clients rely on.

### 4. Cilium/Hubble scraping → Prometheus (Option B)
- [ ] Add `scrape_configs` (or `extraScrapeConfigs`) to `values/prometheus.yaml`:
  - cilium-agent `:9962` (kubernetes_sd, endpoints/pod role, `io.cilium/app` selector)
  - hubble `:9965`
  - cilium-operator `:9963` (kube-system)
- [ ] Re-enable Prometheus's own scraping; update the stale comment that says
  "Alloy handles all scraping — disable Prometheus's own scrapers."
- [ ] Keep `web.enable-remote-write-receiver` only if we keep any remote_write path
  (not needed under Option B — collector no longer pushes metrics).
- [ ] Delete/repurpose `cilium-servicemonitor.yaml` (its header already says it's
  reference-only; no Prometheus Operator here).

### 5. Collector self-observability
- [ ] Expose collector telemetry on `:8888` and scrape it (Prometheus scrape_config, or
  collector `prometheus` self-receiver → ClickHouse like the reference's `metrics/internal`).
- [ ] Replace the Alloy mixin dashboards with an OpenTelemetry Collector dashboard
  (grafana.com gnetId 15983 or 12553).
- [ ] Delete `alloy-dashboards-configmap.yaml` and the `dashboardsConfigMaps: otel: alloy-dashboards`
  + `otel` provider wiring in `values/grafana.yaml`. **(Note: this file is currently
  modified in the working tree — that change becomes moot.)**

### 6. Makefile
- [ ] Add `open-telemetry=https://open-telemetry.github.io/opentelemetry-helm-charts`
  to `HELM_REPOS` (if using Helm).
- [ ] Replace the `alloy-configmap` apply + `helm ... alloy` + `ch-writer` apply +
  `alloy-dashboards` apply with the otel-collector install (+ its configmap/RBAC).
- [ ] `uninstall`: drop `alloy` from `helm uninstall`, add `otel-collector`.
- [ ] `routes`: drop `alloy-debug` describe.
- [ ] `pf-alloy` → `pf-otel` (or remove; collector has health_check `:13133` / zpages `:55679`,
  no rich debug UI like Alloy's `:12345`).

### 7. Envoy Gateway route
- [ ] Remove the `alloy-debug` HTTPRoute from `gateway-routes.yaml` (path `/alloy` → `alloy:12345`).
  Optionally add a route to the collector's zpages `:55679` if we want a debug surface.

### 8. Files to delete
- [ ] `k8s/o11y/manifests/alloy-configmap.yaml`
- [ ] `k8s/o11y/values/alloy.yaml`
- [ ] `k8s/o11y/manifests/ch-writer-deployment.yaml` (folded into the collector)
- [ ] `k8s/o11y/manifests/alloy-dashboards-configmap.yaml`
- [ ] `k8s/o11y/manifests/cilium-servicemonitor.yaml` (unless kept as reference)

### 9. Docs
- [ ] `README.md`: Components table (drop Alloy + ch-writer rows, add "OTel Collector"),
  architecture diagram, and "Adding a service" — change OTLP endpoint from
  `alloy.o11y.svc.cluster.local:4317` → `otel-collector.o11y.svc.cluster.local:4317`,
  and the "add a scrape target" step from editing `alloy-configmap.yaml` to editing
  Prometheus scrape_configs (Option B) or the collector config.
- [ ] `README.md` Troubleshooting: `deploy/ch-writer` logs → `deploy/otel-collector`.
- [ ] `AGENT.md`: lines 13 + 188 (component list + install order).
- [ ] `SETUP.md`: lines 92–93 (pod list), 158–161 (ch-writer note).
- [ ] `docs/architecture.excalidraw`: redraw Alloy/ch-writer → OTel Collector.

## Gaps / risks to watch
- **Node-local scraping.** Alloy hit `localhost:9962/:9965` because it was a DaemonSet.
  Under Option B, Prometheus reaches every cilium-agent pod via k8s SD (endpoints role) —
  no DaemonSet needed. If we ever pick Option A, the collector must either run as a
  DaemonSet or use k8s SD too (do **not** rely on `localhost`).
- **`go_services` placeholder.** Currently an empty Alloy scrape. Post-migration, custom
  app metrics either become a Prometheus scrape_config or are pushed as OTLP metrics to
  the collector → ClickHouse. Document the chosen pattern in "Adding a service."
- **TTL drift.** ch-writer uses `ttl: 720h` (30d); the reference uses `ttl: 0` (forever).
  Pick one deliberately; note the exporter only applies TTL to tables it creates.
- **Metrics split-brain.** After migration, Cilium/infra metrics live in Prometheus and
  app OTLP metrics live in ClickHouse. That's intentional, but Grafana users need to pick
  the right datasource per panel. Call it out in the README.
