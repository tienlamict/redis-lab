// Package api wires HTTP routes and bundles shared dependencies needed by
// each handler (k8s client, redis clients per lab, event broadcaster).
package api

import (
	"io/fs"
	"net/http"

	backend "github.com/redis-lab/backend"
	"github.com/redis-lab/backend/internal/events"
	"github.com/redis-lab/backend/internal/k8s"
	redislab "github.com/redis-lab/backend/internal/redis"

	"github.com/gin-gonic/gin"
)

// Deps groups everything handlers need. Created once in main and passed in.
type Deps struct {
	K8s         *k8s.Client
	Broadcaster *events.Broadcaster
	Standalone  *redislab.StandaloneClient
	Sentinel    *redislab.SentinelClient
	Cluster     *redislab.ClusterClient

	// Namespaces per lab (fixed by deploy script).
	NSStandalone string
	NSSentinel   string
	NSCluster    string

	// Helm release names per lab (same as namespace by convention).
	ReleaseStandalone string
	ReleaseSentinel   string
	ReleaseCluster    string

	// Helm values file paths (resolved at startup).
	ValuesStandalone string
	ValuesSentinel   string
	ValuesCluster    string
}

// NewRouter wires every route + serves the embedded UI as SPA fallback.
func NewRouter(d *Deps) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery(), gin.Logger())

	// REST
	g := r.Group("/api/labs")
	{
		g.GET("/standalone/topology", d.handleStandaloneTopology)
		g.GET("/sentinel/topology", d.handleSentinelTopology)
		g.GET("/cluster/topology", d.handleClusterTopology)

		g.POST("/:lab/command", d.handleCommand)
		g.POST("/:lab/fault/kill", d.handleKill)
		g.POST("/:lab/reset", d.handleReset)
	}
	r.GET("/sse/:lab", d.handleSSE)
	r.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	// Embedded SPA — fall through to index.html for unknown paths.
	uifs, err := fs.Sub(backend.UIFiles, "ui/dist")
	if err == nil {
		r.NoRoute(func(c *gin.Context) {
			p := c.Request.URL.Path
			if p == "/" || p == "" {
				p = "/index.html"
			}
			f, err := uifs.Open(p[1:])
			if err != nil {
				// SPA fallback
				idx, err := uifs.Open("index.html")
				if err != nil {
					c.Status(http.StatusNotFound)
					return
				}
				defer idx.Close()
				c.Status(http.StatusOK)
				c.Header("Content-Type", "text/html; charset=utf-8")
				_, _ = copyFS(c.Writer, idx)
				return
			}
			defer f.Close()
			c.Status(http.StatusOK)
			c.Header("Content-Type", contentTypeFor(p))
			_, _ = copyFS(c.Writer, f)
		})
	}
	return r
}
