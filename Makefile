KUBECONFIG ?= /etc/rancher/k3s/k3s.yaml
export KUBECONFIG

.PHONY: help \
        k3s-install k3s-uninstall \
        cilium-install cilium-upgrade cilium-status \
        cilium-apply cilium-lb \
        clickstack-apply clickstack-delete \
        alloy-apply grafana-apply clickhouse-apply \
        node-exporter-apply \
        apply-all delete-all \
        hubble-status hubble-ui \
        contour-export contour-apply contour-clean

# ─── Help ────────────────────────────────────────────────────────────────────

help:
	@echo "roguequery.local IaC — k3s + Cilium + ClickStack"
	@echo ""
	@echo "Bootstrap (run in order):"
	@echo "  make k3s-install        Install k3s (run on NUC as root)"
	@echo "  make cilium-install     Install Cilium via Helm"
	@echo "  make cilium-lb          Apply LB IP pool + L2 policy"
	@echo "  make clickstack-apply   Deploy ClickHouse + Alloy + Grafana"
	@echo ""
	@echo "Day-2:"
	@echo "  make cilium-upgrade     Upgrade Cilium in-place"
	@echo "  make cilium-status      Show Cilium + Hubble health"
	@echo "  make hubble-status      Run hubble observe (requires hubble CLI)"
	@echo "  make hubble-ui          Port-forward Hubble UI to localhost:12000"
	@echo ""
	@echo "Teardown:"
	@echo "  make k3s-uninstall      Remove k3s from node"

# ─── k3s ─────────────────────────────────────────────────────────────────────

k3s-install:
	@bash infra/k3s/install.sh

k3s-uninstall:
	@bash infra/k3s/uninstall.sh

# ─── Cilium ──────────────────────────────────────────────────────────────────

cilium-install:
	@bash infra/cilium/install.sh

cilium-upgrade:
	@CILIUM_VERSION=$(CILIUM_VERSION) bash infra/cilium/install.sh

cilium-apply:
	@kubectl apply -f manifests/cilium/

cilium-lb: cilium-apply
	@echo "LB pool and L2 policy applied."

cilium-status:
	@kubectl -n kube-system get pods -l k8s-app=cilium -o wide
	@echo ""
	@kubectl -n kube-system get pods -l k8s-app=hubble-relay -o wide
	@echo ""
	@kubectl -n kube-system get pods -l app.kubernetes.io/name=hubble-ui -o wide

# ─── ClickStack ───────────────────────────────────────────────────────────────

clickstack-apply: clickhouse-apply alloy-apply grafana-apply node-exporter-apply
	@echo "ClickStack deployed to observability namespace."

clickhouse-apply:
	@kubectl apply -f manifests/clickhouse/

alloy-apply:
	@kubectl apply -f manifests/alloy/

grafana-apply:
	@kubectl apply -f manifests/grafana/

node-exporter-apply:
	@kubectl apply -f manifests/prometheus/

clickstack-delete:
	@kubectl delete -f manifests/grafana/ --ignore-not-found
	@kubectl delete -f manifests/alloy/ --ignore-not-found
	@kubectl delete -f manifests/clickhouse/ --ignore-not-found
	@kubectl delete namespace observability --ignore-not-found

# ─── Full stack ───────────────────────────────────────────────────────────────

apply-all: cilium-lb clickstack-apply

# ─── Observability helpers ────────────────────────────────────────────────────

hubble-status:
	@hubble observe --last 50

hubble-ui:
	@echo "Hubble UI → http://localhost:12000"
	@kubectl -n kube-system port-forward svc/hubble-ui 12000:80

# ─── Contour (legacy — pre-Cilium) ───────────────────────────────────────────

CONTOUR_NS   := projectcontour
CONTOUR_OUT  := manifests/contour

contour-export: contour-clean
	@mkdir -p $(CONTOUR_OUT)
	@kubectl get namespace $(CONTOUR_NS) -o yaml | kubectl apply --dry-run=client -o yaml -f - > $(CONTOUR_OUT)/00-namespace.yaml 2>/dev/null || echo "$(CONTOUR_NS): ⚠ namespace not found"
	@kubectl get crd -l app.kubernetes.io/name=contour -o yaml | kubectl apply --dry-run=client -o yaml -f - > $(CONTOUR_OUT)/01-crds.yaml 2>/dev/null || echo "$(CONTOUR_NS): ⚠ crds not found"
	@kubectl get clusterrole,clusterrolebinding -l app.kubernetes.io/name=contour -o yaml | kubectl apply --dry-run=client -o yaml -f - > $(CONTOUR_OUT)/02-rbac.yaml 2>/dev/null || echo "$(CONTOUR_NS): ⚠ rbac not found"
	@kubectl get serviceaccount,configmap,secret -n $(CONTOUR_NS) -l app.kubernetes.io/name=contour -o yaml | kubectl apply --dry-run=client -o yaml -f - > $(CONTOUR_OUT)/03-config.yaml 2>/dev/null || echo "$(CONTOUR_NS): ⚠ config not found"
	@kubectl get service -n $(CONTOUR_NS) -o yaml | kubectl apply --dry-run=client -o yaml -f - > $(CONTOUR_OUT)/04-services.yaml 2>/dev/null || echo "$(CONTOUR_NS): ⚠ services not found"
	@kubectl get deployment -n $(CONTOUR_NS) -o yaml | kubectl apply --dry-run=client -o yaml -f - > $(CONTOUR_OUT)/05-deployment.yaml 2>/dev/null || echo "$(CONTOUR_NS): ⚠ deployment not found"
	@kubectl get daemonset -n $(CONTOUR_NS) -o yaml | kubectl apply --dry-run=client -o yaml -f - > $(CONTOUR_OUT)/06-daemonset.yaml 2>/dev/null || echo "$(CONTOUR_NS): ⚠ daemonset not found"

contour-apply:
	@kubectl apply -f $(CONTOUR_OUT)/

contour-clean:
	@rm -f $(CONTOUR_OUT)/*.yaml
