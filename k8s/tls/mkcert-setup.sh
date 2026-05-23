#!/usr/bin/env bash
# k8s/tls/mkcert-setup.sh
#
# One-time setup: generates a wildcard TLS certificate for *.roguequery.local
# using mkcert and installs it as a Kubernetes Secret for Envoy Gateway.
#
# Re-run this script when the certificate expires (~2 years 3 months from issue).
# Check expiry: make check-expiry
#
# Prerequisites:
#   - mkcert installed: https://github.com/FiloSottile/mkcert
#   - kubectl configured and pointing at roguequery.local cluster
#
# Run from repo root:
#   bash k8s/tls/mkcert-setup.sh

set -euo pipefail

DOMAIN="${DOMAIN:-roguequery.local}"
WILDCARD="*.${DOMAIN}"
SECRET_NAME="${DOMAIN//./-}-tls"
SECRET_NS="envoy-gateway-system"
CERT_DIR="$(dirname "$0")/certs"   # k8s/tls/certs/ — gitignored

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m'

info()    { echo -e "${GREEN}[mkcert]${NC} $*"; }
warn()    { echo -e "${YELLOW}[warn]${NC}  $*"; }
die()     { echo -e "${RED}[error]${NC} $*" >&2; exit 1; }

command -v mkcert >/dev/null || die "mkcert not found. Install from https://github.com/FiloSottile/mkcert"
command -v kubectl >/dev/null || die "kubectl not found"

# ── Install mkcert CA into the system trust store ────────────────────────────
# This is what makes browsers trust the generated cert without a warning.
# On Windows + WSL2: run 'mkcert -install' in a Windows terminal (not WSL),
# then re-run this script in WSL for the cert generation steps only.
info "Installing mkcert CA into system trust store..."
mkcert -install

CAROOT="$(mkcert -CAROOT)"
info "CA root: $CAROOT"
info "  rootCA.pem:     $CAROOT/rootCA.pem"
info "  rootCA-key.pem: $CAROOT/rootCA-key.pem  (keep this safe)"

# ── Generate wildcard cert ───────────────────────────────────────────────────
mkdir -p "$CERT_DIR"
pushd "$CERT_DIR" >/dev/null

info "Generating certificate for $DOMAIN and $WILDCARD..."
mkcert "$DOMAIN" "$WILDCARD"

# mkcert names files after the first domain argument
CERT_FILE="${DOMAIN}+1.pem"
KEY_FILE="${DOMAIN}+1-key.pem"

[[ -f "$CERT_FILE" ]] || die "Expected cert file $CERT_FILE not found"
[[ -f "$KEY_FILE"  ]] || die "Expected key file $KEY_FILE not found"

# Print expiry so it's visible in terminal output (and easily greppable in CI)
info "Certificate expiry:"
openssl x509 -in "$CERT_FILE" -noout -dates

popd >/dev/null

# ── Create / update Kubernetes Secret ───────────────────────────────────────
# The Secret must live in the same namespace as the Gateway (envoy-gateway-system)
# so Envoy Gateway can reference it without a ReferenceGrant.
info "Creating TLS secret '$SECRET_NAME' in namespace '$SECRET_NS'..."

kubectl create secret tls "$SECRET_NAME" \
  --cert="$CERT_DIR/$CERT_FILE" \
  --key="$CERT_DIR/$KEY_FILE"  \
  --namespace "$SECRET_NS"     \
  --dry-run=client -o yaml | kubectl apply -f -

info "Secret applied."
info ""
info "Next steps:"
info "  1. Apply the updated Gateway:    kubectl apply -f k8s/envoy-gateway/gateway.yaml"
info "  2. Apply updated routes:         kubectl apply -f k8s/o11y/manifests/gateway-routes.yaml"
info "  3. Verify HTTPS:                 curl -I https://grafana.${DOMAIN}"
info ""
warn "Remember: distribute the CA cert to other devices that need to trust ${DOMAIN}:"
warn "  CA cert: $CAROOT/rootCA.pem"
warn "  See k8s/tls/README.md for platform-specific install instructions."
