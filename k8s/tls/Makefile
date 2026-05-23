SECRET_NAME := roguequery-local-tls
SECRET_NS   := envoy-gateway-system
CERT_DIR    := certs

.PHONY: install check-expiry rotate

# One-time setup (and rotation when cert expires)
install:
	bash mkcert-setup.sh

# Check when the current cert expires
check-expiry:
	@kubectl get secret $(SECRET_NAME) -n $(SECRET_NS) \
	  -o jsonpath='{.data.tls\.crt}' | base64 -d | \
	  openssl x509 -noout -dates

# Force-rotate: regenerate cert and update the secret
# Use when: cert is near expiry, or you've added new SANs
rotate: install
	@echo "Certificate rotated. Envoy Gateway picks up the new secret automatically."
	@echo "No pod restarts needed."
