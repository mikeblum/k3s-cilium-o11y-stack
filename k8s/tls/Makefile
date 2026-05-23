DOMAIN     ?= example.local
TLS_SECRET := $(subst .,-, $(DOMAIN))-tls
SECRET_NS  := envoy-gateway-system
CERT_DIR   := certs

.PHONY: install check-expiry rotate

install:
	DOMAIN=$(DOMAIN) bash mkcert-setup.sh

check-expiry:
	@kubectl get secret $(TLS_SECRET) -n $(SECRET_NS) \
	  -o jsonpath='{.data.tls\.crt}' | base64 -d | \
	  openssl x509 -noout -dates

rotate: install
	@echo "Certificate rotated. Envoy Gateway picks up the new secret automatically."
	@echo "No pod restarts needed."
