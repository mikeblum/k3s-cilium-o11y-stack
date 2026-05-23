NS := tailscale

.PHONY: repo install uninstall status logs-grafana logs-hubble

repo:
	helm repo add tailscale https://pkgs.tailscale.com/helmcharts
	helm repo update

install: repo
	@test -f values.secret.yaml || \
		(echo "ERROR: values.secret.yaml missing — copy from values.secret.yaml.example and fill in OAuth creds"; exit 1)
	helm upgrade --install tailscale-operator tailscale/tailscale-operator \
		--namespace $(NS) \
		--create-namespace \
		--values values.yaml \
		--values values.secret.yaml \
		--wait
	kubectl apply -f manifests/

uninstall:
	kubectl delete -f manifests/ --ignore-not-found
	helm uninstall tailscale-operator -n $(NS) || true

status:
	@echo "=== Operator pod ==="
	kubectl get pods -n $(NS) -l app=operator
	@echo ""
	@echo "=== Ingress resources ==="
	kubectl get ingress -n observability
	kubectl get ingress -n kube-system
	@echo ""
	@echo "=== Tailscale proxy pods (one per Ingress) ==="
	kubectl get pods -n $(NS) -l tailscale.com/parent-resource-type=ingress

logs-grafana:
	$(eval POD := $(shell kubectl get pod -n tailscale \
		--selector=tailscale.com/parent-resource-type=ingress,tailscale.com/parent-resource=grafana-ts,tailscale.com/parent-resource-ns=observability \
		-ojsonpath='{.items[0].metadata.name}'))
	kubectl logs $(POD) -n tailscale -f

logs-hubble:
	$(eval POD := $(shell kubectl get pod -n tailscale \
		--selector=tailscale.com/parent-resource-type=ingress,tailscale.com/parent-resource=hubble-ts,tailscale.com/parent-resource-ns=kube-system \
		-ojsonpath='{.items[0].metadata.name}'))
	kubectl logs $(POD) -n tailscale -f
