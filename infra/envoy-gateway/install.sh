#!/usr/bin/env bash
# infra/envoy-gateway/install.sh
#
# Installs Envoy Gateway via Helm into the envoy-gateway-system namespace.
# Run this BEFORE tls-install (tls-install creates a Secret in this namespace).
#
# Override the version:
#   ENVOY_GATEWAY_VERSION=v1.8.0 bash infra/envoy-gateway/install.sh

set -euo pipefail

ENVOY_GATEWAY_VERSION="${ENVOY_GATEWAY_VERSION:-v1.8.0}"
KUBECONFIG="${KUBECONFIG:-/etc/rancher/k3s/k3s.yaml}"

export KUBECONFIG

command -v helm >/dev/null || { echo "ERROR: helm not found. Install: https://helm.sh/docs/intro/install/"; exit 1; }

echo "Installing Envoy Gateway ${ENVOY_GATEWAY_VERSION}..."

helm upgrade --install eg \
  oci://docker.io/envoyproxy/gateway-helm \
  --version "${ENVOY_GATEWAY_VERSION}" \
  --namespace envoy-gateway-system \
  --create-namespace \
  --wait \
  --timeout 5m

echo ""
echo "Waiting for Envoy Gateway to be ready..."
kubectl -n envoy-gateway-system rollout status deployment/envoy-gateway --timeout=120s

echo ""
echo "Envoy Gateway status:"
kubectl -n envoy-gateway-system get pods

echo ""
echo "Next: make tls-install   (creates TLS secret in envoy-gateway-system)"
echo "      make gateway-apply (applies GatewayClass + cluster-ingress Gateway)"
