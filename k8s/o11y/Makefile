HELM_REPOS := \
	clickhouse=https://charts.pascaliske.dev \
	grafana=https://grafana.github.io/helm-charts \
	open-telemetry=https://open-telemetry.github.io/opentelemetry-helm-charts \
	prometheus-community=https://prometheus-community.github.io/helm-charts

NS         := o11y
# node-exporter only — the one namespace allowed host access. See namespace.yaml.
HOST_NS    := host
DOMAIN     ?= example.local

.PHONY: repos install uninstall status routes secrets

repos:
	$(foreach repo,$(HELM_REPOS), \
		helm repo add $(word 1,$(subst =, ,$(repo))) $(word 2,$(subst =, ,$(repo)));)
	helm repo update

# Fails early with a readable message rather than letting Helm install a
# collector that CrashLoopBackOffs on a missing Secret.
secrets:
	@test -f secrets.yaml || { \
		echo "ERROR: secrets.yaml not found."; \
		echo "       cp secrets.example.yaml secrets.yaml and fill in the passwords."; \
		exit 1; }

# Chart versions are pinned so `make install` is reproducible. Leaving one
# unpinned means Helm silently takes whatever is newest in the repo cache —
# prometheus drifted 29.23.0 -> 29.24.0 mid-deploy that way. Bump deliberately,
# and re-check values/prometheus.yaml's scrapeConfigs override when you do.
CH_VERSION      := 1.0.0
PROM_VERSION    := 29.24.0
OTELCOL_VERSION := 0.166.0
GRAFANA_VERSION := 10.5.15
NODEEXP_VERSION := 4.56.1

install: repos secrets
	kubectl apply -f namespace.yaml
	kubectl apply -f secrets.yaml
	kubectl apply -f manifests/clickhouse-users-override.yaml
	helm upgrade --install clickhouse    clickhouse/clickhouse                        -n $(NS) -f values/clickhouse.yaml       --version $(CH_VERSION)
	helm upgrade --install prometheus    prometheus-community/prometheus              -n $(NS) -f values/prometheus.yaml       --version $(PROM_VERSION)
	helm upgrade --install node-exporter prometheus-community/prometheus-node-exporter -n $(HOST_NS) -f values/node-exporter.yaml --version $(NODEEXP_VERSION)
	helm upgrade --install otelcol       open-telemetry/opentelemetry-collector       -n $(NS) -f values/otel-collector.yaml   --version $(OTELCOL_VERSION)
	helm upgrade --install grafana       grafana/grafana                              -n $(NS) -f values/grafana.yaml          --version $(GRAFANA_VERSION)
	kubectl apply -f manifests/grafana-datasources-configmap.yaml
	kubectl apply -f manifests/reference-grants.yaml
	DOMAIN=$(DOMAIN) envsubst '$${DOMAIN}' < manifests/gateway-routes.yaml | kubectl apply -f -

uninstall:
	helm uninstall grafana prometheus otelcol clickhouse -n $(NS) || true
	helm uninstall node-exporter -n $(HOST_NS) || true
	kubectl delete -f manifests/ --ignore-not-found
	kubectl delete namespace $(NS) $(HOST_NS) --ignore-not-found

status:
	kubectl get pods -n $(NS)
	kubectl get pods -n $(HOST_NS)

routes:
	kubectl get httproute -n $(NS)
	kubectl get httproute -n kube-system
	kubectl describe httproute grafana    -n $(NS)
	kubectl describe httproute hubble-ui  -n kube-system

# Port-forwards for debugging — not needed in normal operation
pf-clickhouse:
	kubectl port-forward -n $(NS) svc/clickhouse 8123:8123

pf-grafana:
	kubectl port-forward -n $(NS) svc/grafana 3000:3000

# Collector health endpoint (:13133) and self-metrics (:8888).
# There is no debug UI — inspect the collector through these two ports.
pf-otel:
	kubectl port-forward -n $(NS) ds/otelcol-agent 13133:13133 8888:8888
