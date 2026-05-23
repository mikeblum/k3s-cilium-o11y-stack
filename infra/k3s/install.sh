#!/usr/bin/env bash
set -euo pipefail

# k3s install for Cilium CNI + kube-proxy replacement
# Run as root on the NUC before installing Cilium.
#
# Disables: flannel, network-policy, kube-proxy, traefik, servicelb
# Cilium handles all of the above via eBPF + Envoy.

# Resolve primary outbound IP via routing table — picks the right interface
# even on machines with multiple NICs or bridge adapters.
NODE_IP="${NODE_IP:-$(ip route get 1 | awk 'NR==1{for(i=1;i<NF;i++) if($i=="src") print $(i+1)}')}"

echo "Installing k3s with NODE_IP=${NODE_IP}"

curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC="server \
  --node-ip=${NODE_IP} \
  --flannel-backend=none \
  --disable-network-policy \
  --disable-kube-proxy \
  --disable=traefik \
  --disable=servicelb \
  --disable=metrics-server \
  --write-kubeconfig-mode=644 \
  --kube-apiserver-arg=feature-gates=GatewayAPI=true" sh -

echo "Waiting for k3s API to be ready..."
until kubectl get nodes &>/dev/null; do sleep 2; done

echo "k3s ready. Node status:"
kubectl get nodes -o wide

echo ""
echo "Next: run infra/cilium/install.sh with NODE_IP=${NODE_IP}"
