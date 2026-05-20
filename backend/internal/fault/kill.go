// Package fault injects failures into the lab clusters. "Kill master" is
// implemented as a plain pod delete — the StatefulSet controller recreates
// it, giving us the failover signal we want to observe.
package fault

import (
	"context"

	"github.com/redis-lab/backend/internal/k8s"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Kill deletes a pod by name. The pod watcher picks up the deletion and emits
// a pod_deleted SSE event automatically.
func Kill(ctx context.Context, c *k8s.Client, namespace, podName string) error {
	return c.Set.CoreV1().Pods(namespace).Delete(ctx, podName, metav1.DeleteOptions{})
}
