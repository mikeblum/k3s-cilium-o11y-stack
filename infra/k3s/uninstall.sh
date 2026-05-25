#!/usr/bin/env bash
set -euo pipefail

echo "Uninstalling k3s..."
/usr/local/bin/k3s-uninstall.sh 2>/dev/null || true
/usr/local/bin/k3s-agent-uninstall.sh 2>/dev/null || true

# Clean up Cilium CNI state left behind
rm -rf /etc/cni/net.d/
rm -rf /opt/cni/bin/cilium*
rm -rf /var/lib/cilium/

echo "Done."
