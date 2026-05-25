#!/usr/bin/env bash
set -euo pipefail

# k3s install for Cilium CNI + kube-proxy replacement
# !! Run as root before installing Cilium.
#
# Disables k3s defaults: flannel, network-policy, kube-proxy, traefik, servicelb
# Cilium handles all of the above via eBPF + Envoy.

NODE_IP="${NODE_IP:?NODE_IP not set. run make ip}"

echo "Installing k3s with NODE_IP=${NODE_IP}"

curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC="server \
  --node-ip=${NODE_IP} \
  --flannel-backend=none \
  --disable-network-policy \
  --disable-kube-proxy \
  --disable=traefik \
  --disable=servicelb \
  --disable=metrics-server \
  --write-kubeconfig-mode=644" sh -

echo "Waiting for k3s API to be ready..."
until kubectl get nodes &>/dev/null; do sleep 2; done

echo "k3s ready. Node status:"
kubectl get nodes -o wide

echo ""
echo "Next steps (from repo root):"
echo "  make cilium-install"
echo "  make cilium-lb NODE_CIDR=${NODE_IP%.*}.192/26   # adjust to a free /26 on your LAN"
