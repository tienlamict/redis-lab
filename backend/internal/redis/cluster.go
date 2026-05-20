// Redis Cluster helpers: shard inspection, KEYSLOT lookup, and a command
// executor that records the MOVED/ASK redirect chain so the UI can display
// "how the request actually flowed".
package redis

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// ClusterClient holds a single direct connection to one cluster node. We don't
// use go-redis's ClusterClient because it auto-discovers pod IPs from
// CLUSTER SLOTS, and those IPs aren't reachable from the host across
// kubectl port-forward. CLUSTER NODES / INFO are admin commands that work
// from any node, so a plain Client is enough for topology.
// For command execution with MOVED tracking, see Exec() — backed by
// kubectl exec into a pod.
type ClusterClient struct {
	cli *redis.Client
	// ExecFn runs `redis-cli -c <args>` inside a cluster pod and returns
	// (stdout, stderr, err). Wired by main from the k8s.Client.
	ExecFn func(ctx context.Context, args []string) (string, string, error)
	// PodName is the pod we exec into (e.g. "redis-cluster-0").
	PodName string
}

// NewCluster dials a single node (host:port) — typically the port-forwarded
// endpoint.
func NewCluster(addrs []string) *ClusterClient {
	addr := "localhost:6379"
	if len(addrs) > 0 {
		addr = addrs[0]
	}
	return &ClusterClient{cli: redis.NewClient(&redis.Options{
		Addr:        addr,
		DialTimeout: 2 * time.Second,
		ReadTimeout: 2 * time.Second,
	})}
}

// Close releases all pooled connections.
func (c *ClusterClient) Close() error { return c.cli.Close() }

// NodeInfo is a row parsed from CLUSTER NODES.
type NodeInfo struct {
	ID         string  `json:"id"`
	Addr       string  `json:"addr"`
	Flags      string  `json:"flags"`
	MasterID   string  `json:"masterId,omitempty"`
	SlotRanges [][]int `json:"slotRanges,omitempty"`
}

// ShardInfo is what the UI consumes — one master + its replicas + slot ranges.
type ShardInfo struct {
	MasterID   string     `json:"masterId"`
	MasterAddr string     `json:"masterAddr"`
	SlotRanges [][]int    `json:"slotRanges"`
	Replicas   []NodeInfo `json:"replicas"`
}

// GetClusterNodes parses `CLUSTER NODES`.
func (c *ClusterClient) GetClusterNodes(ctx context.Context) ([]NodeInfo, error) {
	raw, err := c.cli.Do(ctx, "CLUSTER", "NODES").Text()
	if err != nil {
		return nil, err
	}
	return parseClusterNodes(raw), nil
}

func parseClusterNodes(raw string) []NodeInfo {
	out := []NodeInfo{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// id ip:port@cport flags master ping pong epoch state slot...
		f := strings.Fields(line)
		if len(f) < 8 {
			continue
		}
		addr := f[1]
		if i := strings.IndexByte(addr, '@'); i > 0 {
			addr = addr[:i]
		}
		ni := NodeInfo{ID: f[0], Addr: addr, Flags: f[2]}
		if f[3] != "-" {
			ni.MasterID = f[3]
		}
		// slot tokens start at index 8
		for _, s := range f[8:] {
			if strings.HasPrefix(s, "[") {
				// migration markers like [12345-<-nodeID] — ignore
				continue
			}
			if i := strings.IndexByte(s, '-'); i > 0 {
				lo, err1 := strconv.Atoi(s[:i])
				hi, err2 := strconv.Atoi(s[i+1:])
				if err1 == nil && err2 == nil {
					ni.SlotRanges = append(ni.SlotRanges, []int{lo, hi})
				}
			} else {
				n, err := strconv.Atoi(s)
				if err == nil {
					ni.SlotRanges = append(ni.SlotRanges, []int{n, n})
				}
			}
		}
		out = append(out, ni)
	}
	return out
}

// GetShards groups nodes into shards (one master + its replicas).
func (c *ClusterClient) GetShards(ctx context.Context) ([]ShardInfo, []NodeInfo, error) {
	nodes, err := c.GetClusterNodes(ctx)
	if err != nil {
		return nil, nil, err
	}
	byID := map[string]NodeInfo{}
	for _, n := range nodes {
		byID[n.ID] = n
	}
	shards := []ShardInfo{}
	for _, n := range nodes {
		if !strings.Contains(n.Flags, "master") {
			continue
		}
		sh := ShardInfo{
			MasterID:   n.ID,
			MasterAddr: n.Addr,
			SlotRanges: n.SlotRanges,
		}
		for _, m := range nodes {
			if m.MasterID == n.ID {
				sh.Replicas = append(sh.Replicas, m)
			}
		}
		shards = append(shards, sh)
	}
	return shards, nodes, nil
}

// GetSlotForKey is a thin wrapper over CLUSTER KEYSLOT.
func (c *ClusterClient) GetSlotForKey(ctx context.Context, key string) (int, error) {
	res, err := c.cli.Do(ctx, "CLUSTER", "KEYSLOT", key).Int64()
	if err != nil {
		return 0, err
	}
	return int(res), nil
}

// Redirect represents one MOVED/ASK hop.
type Redirect struct {
	Kind string `json:"kind"` // MOVED or ASK
	Slot int    `json:"slot"`
	To   string `json:"to"`
}

// ExecResult is the response shape for /api/labs/cluster/command.
type ExecResult struct {
	Result     any        `json:"result"`
	ExecutedOn string     `json:"executedOn,omitempty"`
	Slot       int        `json:"slot,omitempty"`
	Redirects  []Redirect `json:"redirects,omitempty"`
}

// ExecuteWithRedirectLog runs args inside a cluster pod via `redis-cli -c`,
// which natively follows MOVED/ASK and prints "-> Redirected to slot ..." for
// each hop. We parse those lines into Redirect entries so the UI can show the
// chain. Falls back to a direct connection if exec is not wired.
func (c *ClusterClient) ExecuteWithRedirectLog(ctx context.Context, args []any) (ExecResult, error) {
	out := ExecResult{Redirects: []Redirect{}}

	// Surface the key's slot for the UI when args look like CMD KEY ...
	if len(args) >= 2 {
		if k, ok := args[1].(string); ok && k != "" {
			if slot, err := c.GetSlotForKey(ctx, k); err == nil {
				out.Slot = slot
			}
		}
	}

	if c.ExecFn == nil {
		// Fallback path: hit the local port-forwarded node; redirects show as
		// errors (no auto-follow without ClusterClient).
		res, err := c.cli.Do(ctx, args...).Result()
		if err != nil {
			var r redirectErr
			if parseRedirect(err.Error(), &r) {
				out.Redirects = append(out.Redirects, Redirect{Kind: r.kind, Slot: r.slot, To: r.addr})
			}
			return out, err
		}
		out.Result = res
		if out.Slot > 0 {
			if n, e := c.NodeForSlot(ctx, out.Slot); e == nil {
				out.ExecutedOn = n
			}
		}
		return out, nil
	}

	// Exec path: first run WITHOUT -c so we see explicit MOVED/ASK responses
	// (redis-cli -c follows redirects silently in non-interactive mode). Then
	// rerun WITH -c to deliver the actual result to the user.
	strArgsRaw := []string{"redis-cli"}
	for _, a := range args {
		strArgsRaw = append(strArgsRaw, fmt.Sprint(a))
	}
	rawOut, _, _ := c.ExecFn(ctx, strArgsRaw)
	for _, line := range strings.Split(rawOut, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "(error)") {
			t = strings.TrimSpace(strings.TrimPrefix(t, "(error)"))
		}
		var r redirectErr
		if parseRedirect(t, &r) {
			out.Redirects = append(out.Redirects, Redirect{Kind: r.kind, Slot: r.slot, To: r.addr})
		}
	}

	strArgs := append([]string{"redis-cli", "-c"}, strArgsRaw[1:]...)
	stdout, stderr, err := c.ExecFn(ctx, strArgs)
	if err != nil && stdout == "" {
		return out, fmt.Errorf("exec: %w (stderr=%s)", err, strings.TrimSpace(stderr))
	}
	res, moreRedirects := parseRedisCliOutput(stdout)
	out.Result = res
	out.Redirects = append(out.Redirects, moreRedirects...)
	if out.Slot > 0 {
		if n, e := c.NodeForSlot(ctx, out.Slot); e == nil {
			out.ExecutedOn = n
		}
	}
	return out, nil
}

// parseRedisCliOutput extracts the final result and any "-> Redirected to
// slot [N] located at host:port" lines.
func parseRedisCliOutput(s string) (string, []Redirect) {
	redirects := []Redirect{}
	var resultLines []string
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if strings.HasPrefix(t, "-> Redirected") {
			// "-> Redirected to slot [7000] located at 10.244.x.x:6379"
			f := strings.Fields(t)
			if len(f) >= 8 {
				slot := 0
				if s := strings.Trim(f[4], "[]"); s != "" {
					slot, _ = strconv.Atoi(s)
				}
				redirects = append(redirects, Redirect{Kind: "MOVED", Slot: slot, To: f[7]})
			}
			continue
		}
		resultLines = append(resultLines, t)
	}
	return strings.Join(resultLines, "\n"), redirects
}

type redirectErr struct {
	kind string
	slot int
	addr string
}

// parseRedirect detects "MOVED 7000 1.2.3.4:6379" or "ASK ..." error strings.
func parseRedirect(s string, out *redirectErr) bool {
	parts := strings.Fields(s)
	if len(parts) < 3 {
		return false
	}
	kind := strings.ToUpper(parts[0])
	if kind != "MOVED" && kind != "ASK" {
		return false
	}
	slot, err := strconv.Atoi(parts[1])
	if err != nil {
		return false
	}
	out.kind = kind
	out.slot = slot
	out.addr = parts[2]
	return true
}

// NodeForSlot returns the address of the master currently owning slot.
func (c *ClusterClient) NodeForSlot(ctx context.Context, slot int) (string, error) {
	shards, _, err := c.GetShards(ctx)
	if err != nil {
		return "", err
	}
	for _, sh := range shards {
		for _, r := range sh.SlotRanges {
			if slot >= r[0] && slot <= r[1] {
				return sh.MasterAddr, nil
			}
		}
	}
	return "", errors.New("slot not assigned")
}

// ClusterInfo returns the parsed `CLUSTER INFO` map.
func (c *ClusterClient) ClusterInfo(ctx context.Context) (map[string]string, error) {
	raw, err := c.cli.Do(ctx, "CLUSTER", "INFO").Text()
	if err != nil {
		return nil, err
	}
	return ParseInfoReplication(raw), nil
}

// Unused helper kept to silence the linter on `fmt` import in some builds.
var _ = fmt.Sprintf
