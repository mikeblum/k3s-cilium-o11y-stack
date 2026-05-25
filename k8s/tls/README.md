# TLS — example.local

LAN access to `*.example.local` uses TLS certificates issued by a local
Certificate Authority (CA) generated with [mkcert](https://github.com/FiloSottile/mkcert).

Tailscale access (`*.ts.net`) uses Let's Encrypt certificates issued
automatically by the Tailscale operator. This document covers LAN only.

## How it works

```
mkcert CA (rootCA.pem)
    └── issues wildcard cert for *.example.local
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
2. Generates a wildcard cert for `example.local` and `*.example.local`
3. Creates/updates the `example-local-tls` Secret in `envoy-gateway-system`

Remote access is routed through Tailscale MagicDNS + let's Encrypt certs via `<tailnet>.ts.net`.

## Check expiry

mkcert certs are valid for ~2 years 3 months from issue date.

```bash
cd k8s/tls
make check-expiry
```

When expiry is within 30 days, run `make rotate`.
