// Package k8s wraps client-go bits used by the lab backend.
// We rely on the user's kubeconfig (Kind context) — no in-cluster auth.
package k8s

import (
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Client bundles the typed clientset plus its underlying REST config (needed
// for pod exec).
type Client struct {
	Set    *kubernetes.Clientset
	Config *rest.Config
}

// NewClient builds a Client from the first available source:
//  1. $KUBECONFIG
//  2. ~/.kube/config
//
// In-cluster config is intentionally skipped — the lab backend runs on the host.
func NewClient() (*Client, error) {
	path := os.Getenv("KUBECONFIG")
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("user home: %w", err)
		}
		path = filepath.Join(home, ".kube", "config")
	}
	cfg, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig %q: %w", path, err)
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("new clientset: %w", err)
	}
	return &Client{Set: cs, Config: cfg}, nil
}
