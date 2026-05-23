HELM_REPOS := \
	clickhouse=https://charts.clickhouse.com \
	grafana=https://grafana.github.io/helm-charts \
	prometheus-community=https://prometheus-community.github.io/helm-charts

NS         := o11y
DOMAIN     ?= roguequery.local

.PHONY: repos install uninstall status routes

repos:
	$(foreach repo,$(HELM_REPOS), \
		helm repo add $(word 1,$(subst =, ,$(repo))) $(word 2,$(subst =, ,$(repo)));)
	helm repo update

install: repos
	kubectl apply -f namespace.yaml
	helm upgrade --install clickhouse  clickhouse/clickhouse            -n $(NS) -f values/clickhouse.yaml
	helm upgrade --install prometheus  prometheus-community/prometheus  -n $(NS) -f values/prometheus.yaml
	kubectl apply -f manifests/alloy-configmap.yaml
	helm upgrade --install alloy       grafana/alloy                    -n $(NS) -f values/alloy.yaml
	kubectl apply -f manifests/ch-writer-deployment.yaml
	kubectl apply -f manifests/cilium-servicemonitor.yaml
	helm upgrade --install grafana     grafana/grafana                  -n $(NS) -f values/grafana.yaml
	kubectl apply -f manifests/grafana-datasources-configmap.yaml
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
	kubectl describe httproute grafana    -n $(NS)
	kubectl describe httproute hubble-ui  -n kube-system
	kubectl describe httproute alloy-debug -n $(NS)

# Port-forwards for debugging — not needed in normal operation
pf-alloy:
	kubectl port-forward -n $(NS) ds/alloy 12345:12345

pf-clickhouse:
	kubectl port-forward -n $(NS) svc/clickhouse 8123:8123

pf-grafana:
	kubectl port-forward -n $(NS) svc/grafana 3000:3000
