// Pod listing + watch helpers. Watch events get translated into SSE Events
// via the broadcaster.
package k8s

import (
	"context"
	"log/slog"
	"time"

	"github.com/redis-lab/backend/internal/events"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
)

// PodInfo is the trimmed-down view we hand to the API layer.
type PodInfo struct {
	Name   string `json:"name"`
	IP     string `json:"ip"`
	Status string `json:"status"`
	Node   string `json:"node"`
	Ready  bool   `json:"ready"`
}

// ListPods returns pods in ns. Empty namespace is a programmer error.
func (c *Client) ListPods(ctx context.Context, ns string) ([]PodInfo, error) {
	pl, err := c.Set.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]PodInfo, 0, len(pl.Items))
	for i := range pl.Items {
		out = append(out, toPodInfo(&pl.Items[i]))
	}
	return out, nil
}

func toPodInfo(p *corev1.Pod) PodInfo {
	ready := false
	for _, c := range p.Status.Conditions {
		if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
			ready = true
			break
		}
	}
	return PodInfo{
		Name:   p.Name,
		IP:     p.Status.PodIP,
		Status: string(p.Status.Phase),
		Node:   p.Spec.NodeName,
		Ready:  ready,
	}
}

// WatchPods runs a long-lived watch loop on ns, translating each event into
// an SSE Event published on broadcaster lab channel `lab`.
// Auto-reconnects on errors with a small backoff. Returns when ctx is canceled.
func (c *Client) WatchPods(ctx context.Context, ns, lab string, bc *events.Broadcaster) {
	log := slog.With("ns", ns, "lab", lab)
	for {
		if ctx.Err() != nil {
			return
		}
		w, err := c.Set.CoreV1().Pods(ns).Watch(ctx, metav1.ListOptions{})
		if err != nil {
			log.Warn("watch pods failed, retrying", "err", err)
			select {
			case <-time.After(2 * time.Second):
				continue
			case <-ctx.Done():
				return
			}
		}
		log.Info("pod watch started")
		ch := w.ResultChan()
	loop:
		for {
			select {
			case <-ctx.Done():
				w.Stop()
				return
			case ev, ok := <-ch:
				if !ok {
					log.Info("pod watch channel closed")
					break loop
				}
				p, ok := ev.Object.(*corev1.Pod)
				if !ok {
					continue
				}
				bc.Publish(events.Event{
					Lab:  lab,
					Type: mapWatchType(ev.Type),
					Payload: map[string]any{
						"pod": toPodInfo(p),
					},
				})
			}
		}
		w.Stop()
	}
}

func mapWatchType(t watch.EventType) string {
	switch t {
	case watch.Added:
		return "pod_added"
	case watch.Modified:
		return "pod_modified"
	case watch.Deleted:
		return "pod_deleted"
	default:
		return "pod_event"
	}
}
