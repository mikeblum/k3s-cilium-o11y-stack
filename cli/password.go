package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	// defaultUser is the read-only ClickHouse user the report queries with. The
	// `grafana` profile is readonly across the otel database, which is all this
	// tool needs — see k8s/o11y/manifests/clickhouse-users-override.yaml.
	defaultUser = "grafana"

	// authSecret holds the plaintext passwords for the users defined (as SHA-256
	// hashes) in the users override. It is gitignored and applied by
	// `make o11y-install`; see k8s/o11y/secrets.example.yaml.
	authSecret    = "clickhouse-auth"
	authNamespace = "o11y"
)

// secretKey maps a ClickHouse user to its key in the clickhouse-auth Secret.
// Only the users the override defines are known; anything else has to bring its
// own password, since there is nothing in the cluster to look up.
func secretKey(user string) (string, error) {
	switch user {
	case "grafana":
		return "GRAFANA_PASSWORD", nil
	case "otel_writer":
		return "OTEL_WRITER_PASSWORD", nil
	}
	return "", fmt.Errorf("no %s key for user %q; set CLICKHOUSE_PASSWORD", authSecret, user)
}

// loadPassword resolves the password for user: CLICKHOUSE_PASSWORD wins, and
// otherwise it is read from the cluster's clickhouse-auth Secret. Reading the
// Secret keeps the password out of the working tree — unlike the .env file this
// tool used before the port, it lives only where `make o11y-install` put it.
func loadPassword(user string) (string, error) {
	if pw := os.Getenv("CLICKHOUSE_PASSWORD"); pw != "" {
		return pw, nil
	}
	key, err := secretKey(user)
	if err != nil {
		return "", err
	}
	return passwordFromSecret(key)
}

// passwordFromSecret shells out to kubectl rather than linking client-go: the
// binary is run from a host that already has a working kubeconfig, and matching
// kubectl's context resolution is the whole point.
func passwordFromSecret(key string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "kubectl", "get", "secret", authSecret,
		"-n", authNamespace, "-o", "jsonpath={.data."+key+"}")
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return "", fmt.Errorf("kubectl get secret %s -n %s: %s",
				authSecret, authNamespace, strings.TrimSpace(string(ee.Stderr)))
		}
		return "", fmt.Errorf("kubectl get secret %s -n %s: %w", authSecret, authNamespace, err)
	}

	// jsonpath prints nothing for a missing key, so an empty result means the
	// Secret exists but predates this key rather than that the password is blank.
	enc := strings.TrimSpace(string(out))
	if enc == "" {
		return "", fmt.Errorf("%s in %s has no %s key", authSecret, authNamespace, key)
	}
	pw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return "", fmt.Errorf("decode %s from %s: %w", key, authSecret, err)
	}
	return string(pw), nil
}
