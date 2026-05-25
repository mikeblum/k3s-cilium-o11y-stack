KUBECONFIG ?= /etc/rancher/k3s/k3s.yaml
# Primary outbound IP — derives the NIC via the routing table.
# Override with: make k3s-install NODE_IP=192.168.x.y
_IP_CMD    = ip route get 1 | awk 'NR==1{for(i=1;i<NF;i++) if($$i=="src") print $$(i+1)}'
NODE_IP   ?= $(shell $(_IP_CMD))
# /26 allocates 62 addresses
NODE_CIDR  ?= $(NODE_IP)/26

# Auto-detect domain from the hostname: 'k8s.mylab.local' → 'mylab.local'
# Falls back to example.local if the hostname has no dot (e.g. bare 'myhost').
# Override: make tls-install DOMAIN=mylab.local
_DOMAIN_CMD = h=$$(hostname); d=$$(echo "$$h" | cut -d. -f2-); [ "$$d" != "$$h" ] && echo "$$d" || echo "example.local"
DOMAIN      ?= $(shell $(_DOMAIN_CMD))
TLS_SECRET  := $(subst .,-,$(DOMAIN))-tls
ENVOY_GATEWAY_VERSION  ?= v1.8.0

export KUBECONFIG

.PHONY: help prereqs \
		ip \
        k3s-install k3s-uninstall \
        cilium-install cilium-upgrade cilium-status cilium-lb \
        tls-install tls-check-expiry \
        gateway-install gateway-apply \
        o11y-install o11y-uninstall o11y-status \
        tailscale-install tailscale-status

# ─── Help ────────────────────────────────────────────────────────────────────

prereqs:
	@echo "Checking prerequisites..."
	@command -v helm      >/dev/null || { echo "  MISSING: helm   — https://helm.sh/docs/intro/install/"; }
	@command -v kubectl   >/dev/null || { echo "  MISSING: kubectl — bundled with k3s after k3s-install"; }
	@command -v mkcert    >/dev/null || { echo "  MISSING: mkcert  — https://github.com/FiloSottile/mkcert"; }
	@command -v envsubst  >/dev/null || { echo "  MISSING: envsubst — install gettext: sudo apt install gettext"; }
	@command -v helm      >/dev/null && \
	 command -v mkcert    >/dev/null && \
	 command -v envsubst  >/dev/null && \
	 echo "All prerequisites satisfied." || true

help:
	@echo "example.local IaC: k3s + Cilium + Envoy Gateway + ClickHouse ClickStack + Tailscale"
	@echo ""
	@echo "Variables (override on any target):"
	@echo "  DOMAIN                = $(DOMAIN)"
	@echo "  NODE_IP               = $(NODE_IP)"
	@echo "  NODE_CIDR             = $(NODE_CIDR)   (LB IP pool — free range on your LAN)"
	@echo "  ENVOY_GATEWAY_VERSION = $(ENVOY_GATEWAY_VERSION)"
	@echo "  Example:   make tls-install DOMAIN=home.example.com"
	@echo "  Example:   make cilium-lb   NODE_CIDR=192.168.1.192/26"
	@echo ""
	@echo "Bootstrap (run in order on the host):"
	@echo "  make k3s-install          Install k3s"
	@echo "  make cilium-install       Install Cilium via Helm"
	@echo "  make cilium-lb            Apply LB IP pool + L2 policy"
	@echo "  make gateway-install      Install Envoy Gateway controller"
	@echo "  make tls-install          Generate mkcert wildcard cert + k8s Secret"
	@echo "  make gateway-apply        Apply cluster-ingress Gateway + GatewayClass"
	@echo "  make o11y-install         Install full o11y stack (Helm + manifests)"
	@echo "  make tailscale-install    Install Tailscale operator + ingress resources"
	@echo ""

# ─── ip ─────────────────────────────────────────────────────────────────────
ip:
	@$(_IP_CMD)

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
	@NODE_CIDR=$(NODE_CIDR) envsubst '$${NODE_CIDR}' \
	  < manifests/cilium/lb-pool.yaml | kubectl apply -f -
	@kubectl apply -f manifests/cilium/l2-policy.yaml
	@echo ""
	@echo "LB pool ($(NODE_CIDR)) and L2 announcement policy applied."
	@echo "Next: make gateway-install"

cilium-status:
	@kubectl -n kube-system get pods -l k8s-app=cilium -o wide
	@echo ""
	@kubectl -n kube-system get pods -l k8s-app=hubble-relay -o wide
	@echo ""
	@kubectl -n kube-system get pods -l app.kubernetes.io/name=hubble-ui -o wide

# ─── TLS ─────────────────────────────────────────────────────────────────────

tls-install:
	@$(MAKE) -C k8s/tls install DOMAIN=$(DOMAIN)
	@echo ""
	@echo "Next: make gateway-apply DOMAIN=$(DOMAIN)"

tls-check-expiry:
	@$(MAKE) -C k8s/tls check-expiry DOMAIN=$(DOMAIN)

# ─── Envoy Gateway ───────────────────────────────────────────────────────────

gateway-install:
	@ENVOY_GATEWAY_VERSION=$(ENVOY_GATEWAY_VERSION) bash infra/envoy-gateway/install.sh

gateway-apply:
	@TLS_SECRET=$(TLS_SECRET) envsubst '$${TLS_SECRET}' \
	  < k8s/envoy-gateway/gateway.yaml | kubectl apply -f -
	@echo ""
	@echo "Next: make o11y-install"

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
