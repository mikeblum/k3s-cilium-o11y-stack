KUBECONFIG ?= /etc/rancher/k3s/k3s.yaml
# Primary outbound IP — derives the NIC via the routing table.
# Override with: make k3s-install NODE_IP=192.168.x.y
NODE_IP    ?= $(shell ip route get 1 | awk 'NR==1{for(i=1;i<NF;i++) if($$i=="src") print $$(i+1)}')
DOMAIN     ?= example.local
TLS_SECRET := $(subst .,-, $(DOMAIN))-tls
export KUBECONFIG

.PHONY: help \
		ip \
        k3s-install k3s-uninstall \
        cilium-install cilium-upgrade cilium-status cilium-lb \
        tls-install tls-check-expiry \
        gateway-apply \
        o11y-install o11y-uninstall o11y-status \
        tailscale-install tailscale-status

# ─── Help ────────────────────────────────────────────────────────────────────

help:
	@echo "example.local IaC: k3s + Cilium + Envoy Gateway + ClickHouse ClickStack + Tailscale"
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

	# ─── ip ─────────────────────────────────────────────────────────────────────
ip:
	@ip route get 1 | awk 'NR==1{for(i=1;i<NF;i++) if($$i=="src") print $$(i+1)}'

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

# ─── O11y stack ──────────────────────────────────────────────────────

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
