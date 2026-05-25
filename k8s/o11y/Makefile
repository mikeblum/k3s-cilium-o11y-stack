HELM_REPOS := \
	clickhouse=https://charts.pascaliske.dev \
	grafana=https://grafana.github.io/helm-charts \
	prometheus-community=https://prometheus-community.github.io/helm-charts

NS         := o11y
DOMAIN     ?= example.local

# GRAFANA_CLICKHOUSE_PASSWORD is read from the clickhouse-users Secret and must be
# exported in the shell before running `make install` so envsubst can substitute it
# into grafana-datasources-configmap.yaml.
# Example: export GRAFANA_CLICKHOUSE_PASSWORD=$(kubectl get secret -n o11y clickhouse-users \
#            -o jsonpath='{.data.grafana_password}' | base64 -d)

.PHONY: repos install uninstall status routes

repos:
	$(foreach repo,$(HELM_REPOS), \
		helm repo add $(word 1,$(subst =, ,$(repo))) $(word 2,$(subst =, ,$(repo)));)
	helm repo update

install: repos
	kubectl apply -f namespace.yaml
	# ClickHouse users Secret must exist before ClickHouse starts (provides GRAFANA_PASSWORD + OTEL_WRITER_PASSWORD).
	# Create it separately: kubectl apply -f manifests/clickhouse-users-secret.yaml
	kubectl apply -f manifests/clickhouse-users-override.yaml
	helm upgrade --install clickhouse  clickhouse/clickhouse            -n $(NS) -f values/clickhouse.yaml --version 1.0.0
	helm upgrade --install prometheus  prometheus-community/prometheus  -n $(NS) -f values/prometheus.yaml
	kubectl apply -f manifests/alloy-configmap.yaml
	helm upgrade --install alloy       grafana/alloy                    -n $(NS) -f values/alloy.yaml
	kubectl apply -f manifests/ch-writer-deployment.yaml
	kubectl apply -f manifests/alloy-dashboards-configmap.yaml
	kubectl apply -f manifests/clickhouse-dashboards-configmap.yaml
	helm upgrade --install grafana     grafana/grafana                  -n $(NS) -f values/grafana.yaml
	DOMAIN=$(DOMAIN) GRAFANA_CLICKHOUSE_PASSWORD=$(GRAFANA_CLICKHOUSE_PASSWORD) \
		envsubst '$${DOMAIN} $${GRAFANA_CLICKHOUSE_PASSWORD}' \
		< manifests/grafana-datasources-configmap.yaml | kubectl apply -f -
	kubectl apply -f manifests/reference-grants.yaml
	DOMAIN=$(DOMAIN) envsubst '$${DOMAIN}' < manifests/gateway-routes.yaml | kubectl apply -f -

uninstall:
	helm uninstall grafana prometheus alloy clickhouse -n $(NS) || true
	kubectl delete -f manifests/ --ignore-not-found
	kubectl delete namespace $(NS) --ignore-not-found

status:
	kubectl get pods -n $(NS)

routes:
	kubectl get httproute -n $(NS)
	kubectl get httproute -n kube-system
	kubectl describe httproute grafana      -n $(NS)
	kubectl describe httproute hubble-ui    -n kube-system
	kubectl describe httproute alloy-debug  -n $(NS)
	kubectl describe httproute clickhouse   -n $(NS)

# Port-forwards for debugging — ClickHouse is accessible at clickhouse.${DOMAIN} via Envoy Gateway
pf-alloy:
	kubectl port-forward -n $(NS) ds/alloy 12345:12345

pf-grafana:
	kubectl port-forward -n $(NS) svc/grafana 3000:3000
