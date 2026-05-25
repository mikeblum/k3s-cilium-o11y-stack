# k3s-cilium-o11y-stack 🔭

> Bootstrap your own local eBPF-powered, OTel-compatible observability stack

## OS Support
**Linux 🐧 only** for now

While MacOS supports Docker 🐳, eBPF is trickier to do inside VMs.

## Overview

This project seeks to reduce the complex and costly $$$ cloud ☁️ deployments of Kubernetes-backed observability into a local-first implementation that empowers DevOps, SREs, and other observability practictioners with an industry-standard foundation powered by well-known open-source projects.

Each component of this stack was chosen against the following criteria:
- **Mature and open source** - preferrably part of the CNCF ecosystem
- **OpenTelemetry support 🔭** - OTel is the defacto standard for observing systems
- **eBPF support 🐝** - eBPF is becoming a standard for instrumenting networks and increasingly applications
- **Local-first deployments** - easy to deploy, observe, and modify locally
- **Out-of-the-box visualizations + dashboards** - import and export Grafana dashboards

### Why not use Clickhouse's ClickStack, SigNoz, et al instead?

Since we're using eBPF-powered Cilium to instrument and visualize our stack's network, the Hubble UI packaged with Cilium uses Grafana dashboards and Prometheus metrics. Given these requiremnts it was simpler to have a stack that hews to the least amount of moving parts as opposed to building custom shims to shape the data to fit in a box.

Using Grafana and Hubble for visualizations gives us the best of both worlds - a vibrant collection of Grafana dashboards alongside out-of-the-box visualizations of the stack's network via Hubble. Exposing the base components of ClickHouse and Cilium gives operators a deeper view of how telemetry flows compared to a more all-in-one solution like Signoz or Grafana's LGTM omnibus images.

This observability stack is opinionated in that eBPF is The Way ™️ for observing and securing networks. But to-date its unclear how eBPF meshes with the other traditional pillars of observability: metrics, traces, and logs. By deploying Cilium as the CNI we can do meta-analysis of how telemetry data flows - versus today were a blackbox sidecar is spun up alongside your application and its assumed your data will make it upstream to your vendor of choice.

## Components

| Component | Layer | Role |
|-----------|-------|------|
| [k3s](https://k3s.io) | Cluster | Lightweight Kubernetes; runs with flannel, kube-proxy, and Traefik disabled to make room for Cilium + Envoy Gateway |
| [Cilium](https://cilium.io) | Networking | eBPF CNI — pod networking, kube-proxy replacement, and L2 LoadBalancer IP pool |
| [Hubble UI](https://docs.cilium.io/en/stable/gettingstarted/hubble/) | Networking | Real-time network flow visualization built into Cilium |
| [Envoy Gateway](https://gateway.envoyproxy.io) | Ingress | Kubernetes Gateway API controller; routes HTTPS subdomains to in-cluster services |
| [mkcert](https://github.com/FiloSottile/mkcert) | TLS | [@FiloSottile](https://github.com/FiloSottile)'s excellent tool for bringing https to `localhost` |
| [Prometheus](https://prometheus.io) | Observability | Metrics scraping and time-series storage |
| [Grafana Alloy](https://grafana.com/oss/alloy-opentelemetry-collector/) | Observability | DaemonSet telemetry collector; receives OTLP from apps, scrapes Cilium + Prometheus targets |
| [ClickHouse](https://clickhouse.com) | Observability | OLAP database; backend store for logs, traces, and metrics |
| ch-writer | Observability | Temporary OTLP → ClickHouse bridge (removed once Alloy ships a native ClickHouse exporter — [grafana/alloy#3492](https://github.com/grafana/alloy/issues/3492)) |
| [Grafana](https://grafana.com) | Observability | Dashboards and visualization over Prometheus + ClickHouse |
| [Tailscale Operator](https://tailscale.com/kb/1236/kubernetes-operator) | Remote access | *(optional)* Exposes services to your tailnet with auto-provisioned Let's Encrypt TLS |

## Architecture

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

Every service gets its own subdomain via Envoy Gateway (LAN) or the Tailscale operator (remote). Below are the services powering the observability stack:

| Service | LAN URL | Tailscale URL | Routing | TLS |
|---------|---------|---------------|---------|-----|
| **Grafana** | `https://grafana.example.local` | `https://grafana.<tailnet>.ts.net` | host-based | mkcert wildcard / Let's Encrypt |
| **Hubble UI** | `https://hubble.example.local` | `https://hubble.<tailnet>.ts.net` | host-based | mkcert wildcard / Let's Encrypt |
| **ClickHouse** | `https://clickhouse.example.local` | `https://clickhouse.<tailnet>.ts.net` | host-based | mkcert wildcard / Let's Encrypt |
| **Alloy** | `https://example.local/alloy` | — LAN only | path-based | mkcert wildcard |

**Tailscale** — the Tailscale operator exposes your k8s service to anywhere on your tailnet.
**Alloy** - debug telemetry pipelines

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

- **Adding a service** — see [apps/README.md](./apps/README.md) for the three-step guide (OTLP env vars, Alloy scrape target, HTTPRoute).

---

## Apps & example workloads

See **[apps/README.md](./apps/README.md)** for:
- Step-by-step instructions for integrating any OTel-instrumented service
- The `otel-go` reference app — a minimal Go HTTP service (`/ping → PONG`) that exercises traces, logs, and Prometheus metrics end-to-end

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
