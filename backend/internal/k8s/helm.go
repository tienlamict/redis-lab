// Thin wrappers around the `helm` and `kubectl` CLIs. We shell out rather than
// import the Helm SDK because the lab only needs install/uninstall and the SDK
// adds significant build/runtime weight.
package k8s

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// HelmInstall runs `helm upgrade --install`. It's idempotent: a missing
// release becomes an install.
func HelmInstall(ctx context.Context, release, chart, namespace, valuesFile string) error {
	args := []string{
		"upgrade", "--install", release, chart,
		"--namespace", namespace,
		"--values", valuesFile,
		"--wait", "--timeout", "5m",
	}
	return runCmd(ctx, "helm", args...)
}

// HelmUninstall removes a release if present; missing release is not an error.
func HelmUninstall(ctx context.Context, release, namespace string) error {
	cmd := exec.CommandContext(ctx, "helm", "uninstall", release, "-n", namespace)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Treat "not found" as success — keeps reset-lab idempotent.
		if bytes.Contains(out, []byte("not found")) {
			return nil
		}
		return fmt.Errorf("helm uninstall: %w: %s", err, string(out))
	}
	return nil
}

// KubectlDeletePVCs removes all PVCs in ns. Required after helm uninstall to
// fully reset state.
func KubectlDeletePVCs(ctx context.Context, namespace string) error {
	return runCmd(ctx, "kubectl", "delete", "pvc", "--all", "-n", namespace, "--ignore-not-found=true")
}

func runCmd(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v: %w: %s", name, args, err, string(out))
	}
	return nil
}
