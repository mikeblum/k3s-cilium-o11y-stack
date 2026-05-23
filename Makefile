KUBECONFIG ?= /etc/rancher/k3s/k3s.yaml
# Primary outbound IP — follows the routing table so it picks the right NIC.
# Override with: make k3s-install NODE_IP=192.168.x.y
NODE_IP    ?= $(shell ip route get 1 | awk 'NR==1{for(i=1;i<NF;i++) if($$i=="src") print $$(i+1)}')
DOMAIN     ?= roguequery.local
TLS_SECRET := $(subst .,-, $(DOMAIN))-tls
export KUBECONFIG

.PHONY: help \
        k3s-install k3s-uninstall \
        cilium-install cilium-upgrade cilium-status cilium-lb \
        tls-install tls-check-expiry \
        gateway-apply \
        o11y-install o11y-uninstall o11y-status \
        tailscale-install tailscale-status \
        node-exporter-apply \
        hubble-status hubble-ui \
        contour-export contour-apply contour-clean

# ─── Help ────────────────────────────────────────────────────────────────────

help:
	@echo "roguequery.local IaC — k3s + Cilium + Envoy Gateway + ClickStack + Tailscale"
	@echo ""
	@echo "Variables (override on any target):"
	@echo "  DOMAIN     = $(DOMAIN)"
	@echo "  NODE_IP    = $(NODE_IP)"
	@echo "  Example:   make tls-install DOMAIN=home.example.com"
	@echo ""
	@echo "Bootstrap (run in order on the host):"
	@echo "  make k3s-install          Install k3s"
	@echo "  make cilium-install       Install Cilium via Helm"
	@echo "  make cilium-lb            Apply LB IP pool + L2 policy"
	@echo "  make tls-install          Generate mkcert wildcard cert + k8s Secret"
	@echo "  make gateway-apply        Apply Envoy Gateway + cluster-ingress"
	@echo "  make o11y-install         Install full o11y stack (Helm + manifests)"
	@echo "  make tailscale-install    Install Tailscale operator + ingress resources"
	@echo ""
	@echo "Day-2:"
	@echo "  make cilium-status        Cilium + Hubble pod status"
	@echo "  make hubble-ui            Port-forward Hubble UI → localhost:12000"
	@echo "  make o11y-status          Pod status in observability namespace"
	@echo "  make tls-check-expiry     Print cert expiry date"
	@echo "  make tailscale-status     Tailscale operator + proxy status"

# ─── k3s ─────────────────────────────────────────────────────────────────────

k3s-install:
	@NODE_IP=$(NODE_IP) bash infra/k3s/install.sh

k3s-uninstall:
	@bash infra/k3s/uninstall.sh

# ─── Cilium ──────────────────────────────────────────────────────────────────

cilium-install:
	@NODE_IP=$(NODE_IP) bash infra/cilium/install.sh

cilium-upgrade:
	@NODE_IP=$(NODE_IP) CILIUM_VERSION=$(CILIUM_VERSION) bash infra/cilium/install.sh

cilium-lb:
	@kubectl apply -f manifests/cilium/

cilium-status:
	@kubectl -n kube-system get pods -l k8s-app=cilium -o wide
	@echo ""
	@kubectl -n kube-system get pods -l k8s-app=hubble-relay -o wide
	@echo ""
	@kubectl -n kube-system get pods -l app.kubernetes.io/name=hubble-ui -o wide

# ─── TLS ─────────────────────────────────────────────────────────────────────

tls-install:
	@$(MAKE) -C k8s/tls install DOMAIN=$(DOMAIN)

tls-check-expiry:
	@$(MAKE) -C k8s/tls check-expiry DOMAIN=$(DOMAIN)

# ─── Envoy Gateway ───────────────────────────────────────────────────────────

gateway-apply:
	@DOMAIN=$(DOMAIN) TLS_SECRET=$(TLS_SECRET) envsubst '$${TLS_SECRET}' \
	  < k8s/envoy-gateway/gateway.yaml | kubectl apply -f -

# ─── Observability stack ──────────────────────────────────────────────────────

o11y-install:
	@$(MAKE) -C k8s/o11y install DOMAIN=$(DOMAIN)

o11y-uninstall:
	@$(MAKE) -C k8s/o11y uninstall

o11y-status:
	@$(MAKE) -C k8s/o11y status

# ─── Tailscale ───────────────────────────────────────────────────────────────

tailscale-install:
	@$(MAKE) -C k8s/tailscale install

tailscale-status:
	@$(MAKE) -C k8s/tailscale status

# ─── Misc helpers ─────────────────────────────────────────────────────────────

node-exporter-apply:
	@kubectl apply -f manifests/prometheus/

hubble-status:
	@hubble observe --last 50

hubble-ui:
	@echo "Hubble UI → http://localhost:12000"
	@kubectl -n kube-system port-forward svc/hubble-ui 12000:80

# ─── Contour (legacy — pre-Cilium) ───────────────────────────────────────────

CONTOUR_NS  := projectcontour
CONTOUR_OUT := manifests/contour

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
