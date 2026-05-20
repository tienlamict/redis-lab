// Topology endpoints: one per lab. The JSON shape is fixed by the prompt spec
// and the UI depends on it. If you rename a field, update ui/src/types.ts.
package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	redislab "github.com/redis-lab/backend/internal/redis"
)

// Node is the per-instance shape used by every topology endpoint.
// Fields are a superset of the spec — extras (replOffset on replicas, etc.)
// stay omitempty so consumers only see what they asked for.
type nodeView struct {
	Pod        string `json:"pod"`
	IP         string `json:"ip"`
	Role       string `json:"role,omitempty"`
	Status     string `json:"status"`
	ReplOffset int64  `json:"replOffset,omitempty"`
}

type sentinelView struct {
	Pod        string `json:"pod"`
	Status     string `json:"status"`
	Monitoring string `json:"monitoring"`
}

type shardView struct {
	MasterID   string     `json:"masterId"`
	MasterPod  string     `json:"masterPod"`
	SlotRanges [][]int    `json:"slotRanges"`
	Replicas   []nodeRef  `json:"replicas"`
}

type nodeRef struct {
	ID  string `json:"id"`
	Pod string `json:"pod"`
}

type podSummary struct {
	Name   string `json:"name"`
	IP     string `json:"ip"`
	Status string `json:"status"`
	Ready  bool   `json:"ready"`
	Node   string `json:"node"`
}

// podNameFromFQDN extracts the leading pod label from a Bitnami in-cluster
// hostname like "redis-sentinel-node-0.redis-sentinel-headless.redis-sentinel.svc.cluster.local"
// or "redis-cluster-2.redis-cluster-headless...". An ip:port survives unchanged
// when there are no dots (we strip the port first).
func podNameFromFQDN(addr string) string {
	host := addr
	if i := strings.IndexByte(host, ':'); i > 0 {
		host = host[:i]
	}
	if i := strings.IndexByte(host, '.'); i > 0 {
		host = host[:i]
	}
	return host
}

// podForIP looks up a pod name by its IP from the pod list. Used by cluster
// topology since CLUSTER NODES returns IPs, not pod FQDNs.
func podForIP(pods []podSummary, ip string) string {
	host := ip
	if i := strings.IndexByte(host, ':'); i > 0 {
		host = host[:i]
	}
	for _, p := range pods {
		if p.IP == host {
			return p.Name
		}
	}
	return ""
}

func (d *Deps) handleStandaloneTopology(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5e9)
	defer cancel()

	rawPods, err := d.K8s.ListPods(ctx, d.NSStandalone)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	pods := make([]podSummary, 0, len(rawPods))
	for _, p := range rawPods {
		pods = append(pods, podSummary{Name: p.Name, IP: p.IP, Status: p.Status, Ready: p.Ready, Node: p.Node})
	}

	// Ask the master for INFO replication — gives us role + repl offsets.
	res, err := d.Standalone.Cmd(ctx, "INFO", "replication")
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"master":   nodeView{Status: "error"},
			"replicas": []nodeView{},
			"pods":     pods,
			"error":    err.Error(),
		})
		return
	}
	info := redislab.ParseInfoReplication(asString(res))

	master := nodeView{Role: info["role"], Status: "ok"}
	for _, p := range pods {
		if strings.Contains(p.Name, "master") {
			master.Pod = p.Name
			master.IP = p.IP
			break
		}
	}
	if v, ok := info["master_repl_offset"]; ok {
		master.ReplOffset = atoi64(v)
	}

	replicas := []nodeView{}
	for _, p := range pods {
		if strings.Contains(p.Name, "replicas") {
			replicas = append(replicas, nodeView{
				Pod:    p.Name,
				IP:     p.IP,
				Role:   "slave",
				Status: pickStatus(p.Ready),
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"master":   master,
		"replicas": replicas,
		"pods":     pods,
		"info":     info,
	})
}

func (d *Deps) handleSentinelTopology(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5e9)
	defer cancel()

	rawPods, err := d.K8s.ListPods(ctx, d.NSSentinel)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	pods := make([]podSummary, 0, len(rawPods))
	for _, p := range rawPods {
		pods = append(pods, podSummary{Name: p.Name, IP: p.IP, Status: p.Status, Ready: p.Ready, Node: p.Node})
	}

	mi, mErr := d.Sentinel.Master(ctx)
	reps, rErr := d.Sentinel.Replicas(ctx)
	sents, sErr := d.Sentinel.Sentinels(ctx)
	masterAddr, _ := d.Sentinel.MasterAddr(ctx)

	masterView := nodeView{
		Pod:    podNameFromFQDN(mi.Addr),
		Role:   "master",
		Status: flagStatus(mi.Flags),
	}
	// Resolve IP from pod list when possible.
	for _, p := range pods {
		if p.Name == masterView.Pod {
			masterView.IP = p.IP
			break
		}
	}

	replicas := make([]nodeView, 0, len(reps))
	for _, r := range reps {
		pod := podNameFromFQDN(r.Addr)
		ip := ""
		for _, p := range pods {
			if p.Name == pod {
				ip = p.IP
				break
			}
		}
		replicas = append(replicas, nodeView{
			Pod:    pod,
			IP:     ip,
			Role:   "slave",
			Status: flagStatus(r.Flags),
		})
	}

	sentinels := make([]sentinelView, 0, len(sents)+1)
	// First add ourselves (the sentinel responding) — `sentinel sentinels` lists
	// peers only, so we synthesize one entry for the master pod sentinel as a
	// best-effort if we know currentMaster. The actual peer list comes from
	// sents below.
	for _, s := range sents {
		sentinels = append(sentinels, sentinelView{
			Pod:        podNameFromFQDN(s.Addr),
			Status:     flagStatus(s.Flags),
			Monitoring: "mymaster",
		})
	}

	body := gin.H{
		"pods":          pods,
		"master":        masterView,
		"replicas":      replicas,
		"sentinels":     sentinels,
		"currentMaster": podNameFromFQDN(masterAddr),
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

	rawPods, err := d.K8s.ListPods(ctx, d.NSCluster)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	pods := make([]podSummary, 0, len(rawPods))
	for _, p := range rawPods {
		pods = append(pods, podSummary{Name: p.Name, IP: p.IP, Status: p.Status, Ready: p.Ready, Node: p.Node})
	}

	shards, nodes, sErr := d.Cluster.GetShards(ctx)
	info, _ := d.Cluster.ClusterInfo(ctx)

	views := make([]shardView, 0, len(shards))
	for _, sh := range shards {
		sv := shardView{
			MasterID:   sh.MasterID,
			MasterPod:  podForIP(pods, sh.MasterAddr),
			SlotRanges: sh.SlotRanges,
			Replicas:   []nodeRef{},
		}
		for _, r := range sh.Replicas {
			sv.Replicas = append(sv.Replicas, nodeRef{ID: r.ID, Pod: podForIP(pods, r.Addr)})
		}
		views = append(views, sv)
	}

	body := gin.H{
		"pods":   pods,
		"shards": views,
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

// flagStatus collapses sentinel/replica flag strings into the simpler
// "ok | sdown | odown | disconnected | unknown" used by the UI.
func flagStatus(flags string) string {
	if flags == "" {
		return "unknown"
	}
	if strings.Contains(flags, "o_down") {
		return "odown"
	}
	if strings.Contains(flags, "s_down") {
		return "sdown"
	}
	if strings.Contains(flags, "disconnected") {
		return "disconnected"
	}
	return "ok"
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

func atoi64(s string) int64 {
	var n int64
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return n
		}
		n = n*10 + int64(c-'0')
	}
	return n
}
