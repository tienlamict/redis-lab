// Sentinel-aware client + a pub/sub listener that surfaces sentinel events
// (sdown, odown, switch-master, failover-state) on the SSE bus.
package redis

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/redis-lab/backend/internal/events"
	"github.com/redis/go-redis/v9"
)

// SentinelClient combines a failover-aware data client with a direct sentinel
// admin client (port 26379).
//
// ExecFn is wired by main to kubectl-exec into the current master pod so that
// data commands work from the host even though sentinel returns in-cluster
// FQDNs unreachable from outside Kind.
type SentinelClient struct {
	failover *redis.Client        // go-redis FailoverClient
	sent     *redis.SentinelClient // for sentinel admin commands
	masterName string

	// ExecFn(podName, args) -> stdout, stderr, err. Set by main.
	ExecFn func(ctx context.Context, podName string, args []string) (string, string, error)
}

// NewSentinel dials addr (host:26379 of any sentinel) and routes commands to
// the current master named `masterName`. The Bitnami chart names it `mymaster`.
func NewSentinel(addr, masterName string) *SentinelClient {
	fo := redis.NewFailoverClient(&redis.FailoverOptions{
		MasterName:    masterName,
		SentinelAddrs: []string{addr},
		DialTimeout:   2 * time.Second,
		ReadTimeout:   2 * time.Second,
	})
	sn := redis.NewSentinelClient(&redis.Options{
		Addr:        addr,
		DialTimeout: 2 * time.Second,
		ReadTimeout: 2 * time.Second,
	})
	return &SentinelClient{failover: fo, sent: sn, masterName: masterName}
}

// Close releases both clients.
func (s *SentinelClient) Close() error {
	_ = s.failover.Close()
	return s.sent.Close()
}

// Cmd executes a command against the current master.
// If ExecFn is set we exec redis-cli inside the master pod (works around
// in-cluster FQDNs not being resolvable from the host). Otherwise we use
// the FailoverClient, which assumes direct network access.
func (s *SentinelClient) Cmd(ctx context.Context, args ...any) (any, error) {
	if s.ExecFn != nil {
		pod, err := s.CurrentMasterPod(ctx)
		if err != nil {
			return nil, err
		}
		strArgs := []string{"redis-cli"}
		for _, a := range args {
			strArgs = append(strArgs, fmt.Sprint(a))
		}
		stdout, stderr, err := s.ExecFn(ctx, pod, strArgs)
		if err != nil && stdout == "" {
			return nil, fmt.Errorf("exec: %w (stderr=%s)", err, strings.TrimSpace(stderr))
		}
		return strings.TrimSpace(stdout), nil
	}
	return s.failover.Do(ctx, args...).Result()
}

// CurrentMasterPod parses the sentinel's master addr (a Bitnami FQDN of the
// form pod.headless.ns.svc...) and returns just the pod name.
func (s *SentinelClient) CurrentMasterPod(ctx context.Context) (string, error) {
	addr, err := s.MasterAddr(ctx)
	if err != nil {
		return "", err
	}
	// addr looks like "redis-sentinel-node-0.redis-sentinel-headless...svc...:6379"
	host := addr
	if i := strings.IndexByte(host, ':'); i > 0 {
		host = host[:i]
	}
	if i := strings.IndexByte(host, '.'); i > 0 {
		host = host[:i]
	}
	if host == "" {
		return "", fmt.Errorf("no master pod resolved")
	}
	return host, nil
}

// MasterAddr returns ip:port of the current master per sentinel.
func (s *SentinelClient) MasterAddr(ctx context.Context) (string, error) {
	res, err := s.sent.GetMasterAddrByName(ctx, s.masterName).Result()
	if err != nil {
		return "", err
	}
	if len(res) < 2 {
		return "", nil
	}
	return res[0] + ":" + res[1], nil
}

// SentinelInfo describes one sentinel node from `SENTINEL SENTINELS`.
type SentinelInfo struct {
	Name  string `json:"name"`
	Addr  string `json:"addr"`
	Flags string `json:"flags"`
}

// MasterInfo is parsed from `SENTINEL MASTER mymaster`.
type MasterInfo struct {
	Name  string `json:"name"`
	Addr  string `json:"addr"`
	Flags string `json:"flags"`
}

// ReplicaInfo is parsed from `SENTINEL REPLICAS mymaster`.
type ReplicaInfo struct {
	Name  string `json:"name"`
	Addr  string `json:"addr"`
	Flags string `json:"flags"`
}

// Master returns flags + addr for the monitored master.
func (s *SentinelClient) Master(ctx context.Context) (MasterInfo, error) {
	res, err := s.sent.Master(ctx, s.masterName).Result()
	if err != nil {
		return MasterInfo{}, err
	}
	mi := MasterInfo{}
	if name, ok := res["name"]; ok {
		mi.Name = name
	}
	mi.Addr = res["ip"] + ":" + res["port"]
	mi.Flags = res["flags"]
	return mi, nil
}

// Replicas returns the known replicas of mymaster.
func (s *SentinelClient) Replicas(ctx context.Context) ([]ReplicaInfo, error) {
	res, err := s.sent.Replicas(ctx, s.masterName).Result()
	if err != nil {
		return nil, err
	}
	out := make([]ReplicaInfo, 0, len(res))
	for _, m := range res {
		out = append(out, ReplicaInfo{
			Name:  m["name"],
			Addr:  m["ip"] + ":" + m["port"],
			Flags: m["flags"],
		})
	}
	return out, nil
}

// Sentinels lists peer sentinels.
func (s *SentinelClient) Sentinels(ctx context.Context) ([]SentinelInfo, error) {
	res, err := s.sent.Sentinels(ctx, s.masterName).Result()
	if err != nil {
		return nil, err
	}
	out := make([]SentinelInfo, 0, len(res))
	for _, m := range res {
		out = append(out, SentinelInfo{
			Name:  m["name"],
			Addr:  m["ip"] + ":" + m["port"],
			Flags: m["flags"],
		})
	}
	return out, nil
}

// WatchEvents subscribes to sentinel pub/sub channels and publishes each
// message as a `sentinel_event` SSE Event. Auto-reconnects with backoff.
// Returns when ctx is canceled.
func WatchSentinelEvents(ctx context.Context, addr, lab string, bc *events.Broadcaster) {
	log := slog.With("addr", addr, "lab", lab)
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		cli := redis.NewClient(&redis.Options{
			Addr:        addr,
			DialTimeout: 2 * time.Second,
			ReadTimeout: 0, // pub/sub blocks indefinitely
		})
		pubsub := cli.PSubscribe(ctx,
			"+sdown", "-sdown",
			"+odown", "-odown",
			"+switch-master",
			"+failover-state-*",
			"+failover-*",
			"+slave-reconf-*",
			"+reset-master",
		)
		ch := pubsub.Channel()
		log.Info("sentinel pubsub started")
		backoff = time.Second
	loop:
		for {
			select {
			case <-ctx.Done():
				_ = pubsub.Close()
				_ = cli.Close()
				return
			case msg, ok := <-ch:
				if !ok {
					log.Warn("pubsub channel closed")
					break loop
				}
				bc.Publish(events.Event{
					Lab:  lab,
					Type: "sentinel_event",
					Payload: map[string]any{
						"channel": msg.Channel,
						"payload": msg.Payload,
					},
				})
				// Heuristic: also flag a synthesized role_changed when sentinel
				// switches master.
				if strings.HasPrefix(msg.Channel, "+switch-master") {
					bc.Publish(events.Event{
						Lab:  lab,
						Type: "role_changed",
						Payload: map[string]any{
							"channel": msg.Channel,
							"payload": msg.Payload,
						},
					})
				}
			}
		}
		_ = pubsub.Close()
		_ = cli.Close()
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return
		}
		if backoff < 16*time.Second {
			backoff *= 2
		}
	}
}
