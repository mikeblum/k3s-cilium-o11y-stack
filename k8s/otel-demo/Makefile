HELM_REPOS := \
	open-telemetry=https://open-telemetry.github.io/opentelemetry-helm-charts

NS      := otel-demo
RELEASE := my-otel-demo

# Pinned for the same reason as the o11y charts: an unpinned version means Helm
# takes whatever is newest in the repo cache without saying so. 0.41.0 ships
# appVersion 3.0.0.
DEMO_VERSION := 0.41.0

.PHONY: repos install uninstall status logs pf-frontend

repos:
	$(foreach repo,$(HELM_REPOS), \
		helm repo add $(word 1,$(subst =, ,$(repo))) $(word 2,$(subst =, ,$(repo)));)
	helm repo update

install: repos
	kubectl apply -f namespace.yaml
	helm upgrade --install $(RELEASE) open-telemetry/opentelemetry-demo -n $(NS) -f values/otel-demo.yaml --version $(DEMO_VERSION)
	kubectl apply -f manifests/

uninstall:
	helm uninstall $(RELEASE) -n $(NS) || true
	kubectl delete -f manifests/ --ignore-not-found
	kubectl delete namespace $(NS) --ignore-not-found

status:
	kubectl get pods -n $(NS)

# The demo has no collector of its own — services send straight to o11y, so
# ingest problems surface there rather than in this namespace.
logs:
	kubectl logs -n o11y ds/otelcol-agent --tail=40

# The doc's way in: http://localhost:8080/
pf-frontend:
	kubectl port-forward -n $(NS) svc/frontend-proxy 8080:8080
