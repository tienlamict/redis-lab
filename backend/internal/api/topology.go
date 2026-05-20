// Topology endpoints: one per lab. The shape is fixed by the prompt and the
// UI depends on it — don't rename fields without updating both sides.
package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	redislab "github.com/redis-lab/backend/internal/redis"
)

type standaloneTopology struct {
	Master   redislab.NodeRole   `json:"master"`
	Replicas []redislab.NodeRole `json:"replicas"`
	Pods     []podSummary        `json:"pods"`
}

type podSummary struct {
	Name   string `json:"name"`
	IP     string `json:"ip"`
	Status string `json:"status"`
	Ready  bool   `json:"ready"`
	Node   string `json:"node"`
}

func (d *Deps) handleStandaloneTopology(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5e9)
	defer cancel()

	pods, err := d.K8s.ListPods(ctx, d.NSStandalone)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := standaloneTopology{}
	for _, p := range pods {
		out.Pods = append(out.Pods, podSummary{Name: p.Name, IP: p.IP, Status: p.Status, Ready: p.Ready, Node: p.Node})
		if !p.Ready || p.IP == "" {
			continue
		}
		// Connect to each pod via its IP through the in-cluster service is not
		// reachable from host — but kubectl port-forward already opened the
		// master service. Easiest: ask the master client for INFO replication.
	}
	// Use the master client to enumerate role + replicas via INFO.
	res, err := d.Standalone.Cmd(ctx, "INFO", "replication")
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"master":   redislab.NodeRole{Status: "error", Err: err.Error()},
			"replicas": []any{},
			"pods":     out.Pods,
		})
		return
	}
	info := redislab.ParseInfoReplication(asString(res))
	master := redislab.NodeRole{
		Role:   info["role"],
		Status: "ok",
	}
	// Best-effort match to a pod by IP from master_host (won't always populate on master).
	for _, p := range out.Pods {
		if strings.Contains(p.Name, "master") {
			master.Pod = p.Name
			master.Addr = p.IP
			break
		}
	}
	replicas := []redislab.NodeRole{}
	for _, p := range out.Pods {
		if strings.Contains(p.Name, "replicas") {
			replicas = append(replicas, redislab.NodeRole{Pod: p.Name, Addr: p.IP, Role: "slave", Status: pickStatus(p.Ready)})
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"master":   master,
		"replicas": replicas,
		"pods":     out.Pods,
		"info":     info,
	})
}

func (d *Deps) handleSentinelTopology(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5e9)
	defer cancel()

	pods, err := d.K8s.ListPods(ctx, d.NSSentinel)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	mi, mErr := d.Sentinel.Master(ctx)
	reps, rErr := d.Sentinel.Replicas(ctx)
	sents, sErr := d.Sentinel.Sentinels(ctx)
	masterAddr, _ := d.Sentinel.MasterAddr(ctx)

	body := gin.H{
		"pods":         pods,
		"master":       mi,
		"replicas":     reps,
		"sentinels":    sents,
		"currentMaster": masterAddr,
	}
	if mErr != nil {
		body["masterError"] = mErr.Error()
	}
	if rErr != nil {
		body["replicasError"] = rErr.Error()
	}
	if sErr != nil {
		body["sentinelsError"] = sErr.Error()
	}
	c.JSON(http.StatusOK, body)
}

func (d *Deps) handleClusterTopology(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5e9)
	defer cancel()

	pods, err := d.K8s.ListPods(ctx, d.NSCluster)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	shards, nodes, sErr := d.Cluster.GetShards(ctx)
	info, _ := d.Cluster.ClusterInfo(ctx)

	body := gin.H{
		"pods":   pods,
		"shards": shards,
		"nodes":  nodes,
		"info":   info,
	}
	if sErr != nil {
		body["error"] = sErr.Error()
	}
	c.JSON(http.StatusOK, body)
}

func pickStatus(ready bool) string {
	if ready {
		return "ok"
	}
	return "pending"
}

func asString(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	default:
		return ""
	}
}
