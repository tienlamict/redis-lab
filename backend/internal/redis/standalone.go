// Connects to the standalone+replication lab and reports the current role of
// each node. Bitnami's pod labels are static and don't reflect post-failover
// state, so we always ask redis itself via the ROLE command.
package redis

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// NodeRole is what we surface in the topology API for a single redis instance.
type NodeRole struct {
	Pod        string `json:"pod"`
	Addr       string `json:"addr"`
	Role       string `json:"role"`       // master | slave | unknown
	Status     string `json:"status"`     // ok | error
	ReplOffset int64  `json:"replOffset"` // master_repl_offset or slave_repl_offset
	Err        string `json:"error,omitempty"`
}

// StandaloneClient owns a single pooled client to the master service.
type StandaloneClient struct {
	cli *redis.Client
}

// NewStandalone dials addr (host:port). Auth disabled in lab.
func NewStandalone(addr string) *StandaloneClient {
	return &StandaloneClient{cli: redis.NewClient(&redis.Options{
		Addr:        addr,
		DialTimeout: 2 * time.Second,
		ReadTimeout: 2 * time.Second,
	})}
}

// Close releases pool resources.
func (s *StandaloneClient) Close() error { return s.cli.Close() }

// Cmd executes a raw command on the master.
func (s *StandaloneClient) Cmd(ctx context.Context, args ...any) (any, error) {
	return s.cli.Do(ctx, args...).Result()
}

// QueryRole asks redis at addr for its role + replication offset.
// Caller supplies pod name so we can echo it back to the UI.
func QueryRole(ctx context.Context, addr, pod string) NodeRole {
	r := NodeRole{Pod: pod, Addr: addr}
	c := redis.NewClient(&redis.Options{
		Addr:        addr,
		DialTimeout: 1500 * time.Millisecond,
		ReadTimeout: 1500 * time.Millisecond,
	})
	defer c.Close()

	res, err := c.Do(ctx, "ROLE").Result()
	if err != nil {
		r.Status = "error"
		r.Err = err.Error()
		return r
	}
	r.Status = "ok"
	if arr, ok := res.([]any); ok && len(arr) > 0 {
		if role, ok := arr[0].(string); ok {
			r.Role = role
		}
		if len(arr) > 1 {
			if off, ok := arr[1].(int64); ok {
				r.ReplOffset = off
			}
		}
	}
	return r
}

// ParseInfoReplication returns key=value pairs from INFO replication output.
func ParseInfoReplication(s string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		i := strings.IndexByte(line, ':')
		if i <= 0 {
			continue
		}
		out[line[:i]] = line[i+1:]
	}
	return out
}

// FmtErr is a tiny helper used by callers wanting a concise error string.
func FmtErr(err error) string { return fmt.Sprintf("%v", err) }
