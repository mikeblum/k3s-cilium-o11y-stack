#!/usr/bin/env bash
set -euo pipefail

CILIUM_VERSION="${CILIUM_VERSION:-1.17.3}"
NODE_IP="${NODE_IP:-$(hostname -I | awk '{print $1}')}"
KUBECONFIG="${KUBECONFIG:-/etc/rancher/k3s/k3s.yaml}"

export KUBECONFIG

echo "Installing Cilium ${CILIUM_VERSION} with NODE_IP=${NODE_IP}"

helm repo add cilium https://helm.cilium.io/ --force-update
helm repo update cilium

helm upgrade --install cilium cilium/cilium \
  --version "${CILIUM_VERSION}" \
  --namespace kube-system \
  --values "$(dirname "$0")/../../helm/cilium/values.yaml" \
  --set k8sServiceHost="${NODE_IP}" \
  --set k8sServicePort=6443 \
  --wait \
  --timeout 5m

echo "Waiting for Cilium to be ready..."
kubectl -n kube-system rollout status deployment/cilium-operator --timeout=120s
kubectl -n kube-system rollout status daemonset/cilium --timeout=120s

echo ""
echo "Cilium status:"
kubectl -n kube-system get pods -l k8s-app=cilium -o wide

echo ""
echo "Hubble relay status:"
kubectl -n kube-system get pods -l k8s-app=hubble-relay -o wide

echo ""
echo "Next: apply manifests/cilium/ for LB pool + L2 policy, then manifests/ for the o11y stack."
