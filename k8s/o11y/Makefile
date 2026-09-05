HELM_REPOS := \
	grafana=https://grafana.github.io/helm-charts \
	open-telemetry=https://open-telemetry.github.io/opentelemetry-helm-charts \
	prometheus-community=https://prometheus-community.github.io/helm-charts

# The ClickHouse operator is OCI-only: no `helm repo add`, the URL is the chart.
CHOP_CHART := oci://ghcr.io/clickhouse/clickhouse-operator-helm

NS         := o11y
# The one namespace allowed host access. See namespace.yaml.
NODEEXP_NS := node-exporter
DOMAIN     ?= example.local

.PHONY: repos install uninstall status routes secrets clickhouse ch-client grafana-admin

repos:
	$(foreach repo,$(HELM_REPOS), \
		helm repo add $(word 1,$(subst =, ,$(repo))) $(word 2,$(subst =, ,$(repo)));)
	helm repo update

# Fails early with a readable message rather than letting Helm install a
# collector that CrashLoopBackOffs on a missing Secret. A key that is absent
# rather than wrong is worse: the ClickHouse pod sits in
# CreateContainerConfigError with nothing in its logs, so check for each one.
# ADMIN_USER/ADMIN_PASSWORD back Grafana's login via admin.existingSecret. A
# missing one there is quieter than the ClickHouse case: Grafana starts fine and
# simply refuses the password, which reads as "wrong credentials" rather than a
# misconfiguration.
CH_SECRET_KEYS := DEFAULT_PASSWORD OTEL_WRITER_PASSWORD GRAFANA_PASSWORD \
                  ADMIN_USER ADMIN_PASSWORD

secrets:
	@test -f secrets.yaml || { \
		echo "ERROR: secrets.yaml not found."; \
		echo "       cp secrets.example.yaml secrets.yaml and fill in the passwords."; \
		exit 1; }
	@for key in $(CH_SECRET_KEYS); do \
		grep -q "^ *$$key:" secrets.yaml || { \
			echo "ERROR: secrets.yaml is missing $$key."; \
			echo "       See secrets.example.yaml — every key there is required."; \
			exit 1; }; \
	done

# Chart versions are pinned so `make install` is reproducible. Leaving one
# unpinned means Helm takes whatever is newest in the repo cache without
# saying so; prometheus drifted 29.23.0 -> 29.24.0 mid-deploy that way. Bump on
# purpose,
# and re-check values/prometheus.yaml's scrapeConfigs override when you do.
CHOP_VERSION    := 0.0.7
PROM_VERSION    := 29.24.0
OTELCOL_VERSION := 0.166.0
GRAFANA_VERSION := 10.5.15
NODEEXP_VERSION := 4.56.1

# ClickHouse is not a Helm release. The operator's chart installs a controller
# and two CRDs, and the database itself is a pair of CRs. So `clickhouse` runs
# on its own before everything downstream of it.
install: repos secrets clickhouse
	helm upgrade --install prometheus    prometheus-community/prometheus              -n $(NS) -f values/prometheus.yaml       --version $(PROM_VERSION)
	helm upgrade --install node-exporter prometheus-community/prometheus-node-exporter -n $(NODEEXP_NS) -f values/node-exporter.yaml --version $(NODEEXP_VERSION)
	helm upgrade --install otelcol       open-telemetry/opentelemetry-collector       -n $(NS) -f values/otel-collector.yaml   --version $(OTELCOL_VERSION)
	helm upgrade --install grafana       grafana/grafana                              -n $(NS) -f values/grafana.yaml          --version $(GRAFANA_VERSION)
	kubectl apply -f manifests/grafana-datasources-configmap.yaml
	kubectl apply -f manifests/grafana-dashboard-scrape-targets.yaml
	kubectl apply -f manifests/reference-grants.yaml
	DOMAIN=$(DOMAIN) envsubst '$${DOMAIN}' < manifests/gateway-routes.yaml | kubectl apply -f -

# Operator, then cluster. The CR is rejected outright if its CRD is not yet
# Established, and the operator has to be up to act on it, so both waits are
# load-bearing rather than cosmetic.
clickhouse: secrets
	kubectl apply -f namespace.yaml
	kubectl apply -f secrets.yaml
	helm upgrade --install clickhouse-operator $(CHOP_CHART) -n $(NS) -f values/clickhouse-operator.yaml --version $(CHOP_VERSION)
	kubectl wait --for=condition=Established --timeout=120s \
		crd/clickhouseclusters.clickhouse.com crd/keeperclusters.clickhouse.com
	kubectl rollout status -n $(NS) deploy/clickhouse-operator-controller-manager --timeout=180s
	kubectl apply -f manifests/clickhouse-cluster.yaml
	kubectl apply -f manifests/clickhouse-service.yaml
	kubectl wait --for=condition=Ready --timeout=600s -n $(NS) \
		keepercluster/clickhouse clickhousecluster/clickhouse

# CRDs are installed with keep: true, so `helm uninstall` leaves them behind on
# purpose, since deleting them would cascade into the cluster and its PVC. The
# explicit delete below is the only thing that drops ClickHouse's data.
uninstall:
	helm uninstall grafana prometheus otelcol -n $(NS) || true
	helm uninstall node-exporter -n $(NODEEXP_NS) || true
	kubectl delete -f manifests/ --ignore-not-found
	helm uninstall clickhouse-operator -n $(NS) || true
	kubectl delete crd clickhouseclusters.clickhouse.com keeperclusters.clickhouse.com --ignore-not-found
	kubectl delete namespace $(NS) $(NODEEXP_NS) --ignore-not-found

status:
	kubectl get pods -n $(NS)
	kubectl get pods -n $(NODEEXP_NS)
	kubectl get keepercluster,clickhousecluster -n $(NS)

routes:
	kubectl get httproute -n $(NS)
	kubectl get httproute -n kube-system
	kubectl describe httproute grafana    -n $(NS)
	kubectl describe httproute hubble-ui  -n kube-system

# Reconcile Grafana's admin password with the grafana-auth Secret. Needed
# because Grafana seeds GF_SECURITY_ADMIN_PASSWORD only when it creates a new
# database. On an existing PVC the env var is ignored, so a rotated Secret
# applies cleanly and then fails to log in. Reads the password from the
# container's own env so nothing is passed on the command line.
grafana-admin:
	kubectl exec -n $(NS) deploy/grafana -c grafana -- \
		sh -c 'grafana cli --homepath /usr/share/grafana admin reset-admin-password "$$GF_SECURITY_ADMIN_PASSWORD"'

# A clickhouse-client shell on the server pod. The operator writes the `default`
# user's password into the pod's /etc/clickhouse-client config, so no flags.
ch-client:
	kubectl exec -it -n $(NS) clickhouse-clickhouse-0-0-0 -- clickhouse-client

# Port-forwards for debugging. Not needed in normal operation.
# Service and pod names come from the CR names in manifests/clickhouse-cluster.yaml.
pf-clickhouse:
	kubectl port-forward -n $(NS) svc/clickhouse-server 8123:8123

pf-grafana:
	kubectl port-forward -n $(NS) svc/grafana 3000:3000

# Collector health endpoint (:13133) and self-metrics (:8888).
# There is no debug UI; inspect the collector through these two ports.
pf-otel:
	kubectl port-forward -n $(NS) ds/otelcol-agent 13133:13133 8888:8888
