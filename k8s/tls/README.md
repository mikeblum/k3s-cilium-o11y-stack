# TLS — roguequery.local

LAN access to `*.roguequery.local` uses TLS certificates issued by a local
Certificate Authority (CA) generated with [mkcert](https://github.com/FiloSottile/mkcert).

Tailscale access (`*.ts.net`) uses Let's Encrypt certificates issued
automatically by the Tailscale operator. This document covers LAN only.

## How it works

```
mkcert CA (rootCA.pem)
    └── issues wildcard cert for *.roguequery.local
            └── stored as k8s Secret in envoy-gateway-system
                    └── referenced by Envoy Gateway HTTPS listener
```

Browsers and clients trust the cert because they trust the mkcert CA.
Any device that doesn't have the CA installed will see a TLS warning.

## First-time setup

```bash
cd k8s/tls
make install
```

This:
1. Installs the mkcert CA into the local system trust store
2. Generates a wildcard cert for `roguequery.local` and `*.roguequery.local`
3. Creates/updates the `roguequery-local-tls` Secret in `envoy-gateway-system`

## Check expiry

mkcert certs are valid for ~2 years 3 months from issue date.

```bash
cd k8s/tls
make check-expiry
```

When expiry is within 30 days, run `make rotate`.

## CA distribution — per device

Any device that browses to `*.roguequery.local` needs to trust the mkcert CA.
The CA cert is at `$(mkcert -CAROOT)/rootCA.pem`.

### macOS

```bash
mkcert -install
# Or manually:
sudo security add-trusted-cert -d -r trustRoot \
  -k /Library/Keychains/System.keychain "$(mkcert -CAROOT)/rootCA.pem"
```

### Windows (run in PowerShell as Administrator, NOT WSL)

```powershell
# mkcert -install handles this if run in a Windows terminal
mkcert -install

# Or manually import via certmgr:
# certlm.msc → Trusted Root Certification Authorities → Import → rootCA.pem
```

### WSL2 / Debian / Ubuntu

```bash
# Copy the CA cert from the Windows host
# CAROOT is typically C:\Users\<you>\AppData\Local\mkcert on Windows
cp /mnt/c/Users/<you>/AppData/Local/mkcert/rootCA.pem /usr/local/share/ca-certificates/mkcert-roguequery.crt
sudo update-ca-certificates

# Verify
openssl verify -CAfile /etc/ssl/certs/ca-certificates.crt \
  <(kubectl get secret roguequery-local-tls -n envoy-gateway-system \
    -o jsonpath='{.data.tls\.crt}' | base64 -d)
```

### Firefox (any platform)

Firefox manages its own trust store and ignores the OS store by default.

Preferences → Privacy & Security → Certificates → View Certificates →
Authorities → Import → select `rootCA.pem` → check "Trust this CA to identify websites"

### Mobile (iOS / Android) — via Tailscale instead

Mobile devices on your LAN would need a manual CA install (MDM or
Settings). This is usually not worth the friction.

**Recommendation**: access roguequery.local from mobile via Tailscale
(`grafana.<tailnet>.ts.net`) where Let's Encrypt certs are used and no
CA distribution is needed.

## Files in this directory

```
k8s/tls/
├── Makefile                  # make install / check-expiry / rotate
├── mkcert-setup.sh           # cert generation and Secret creation
├── README.md                 # this file
└── certs/                    # gitignored — generated cert files live here
    ├── roguequery.local+1.pem
    └── roguequery.local+1-key.pem
```

The `certs/` directory is gitignored. Never commit private key files.
The mkcert CA root key (`rootCA-key.pem`) is in `$(mkcert -CAROOT)` on
your local machine and should never leave it.
