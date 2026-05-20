// Fault-injection endpoints. Kill = pod delete; Reset = helm uninstall +
// PVC wipe + helm install.
package api

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis-lab/backend/internal/fault"
	"github.com/redis-lab/backend/internal/k8s"
)

type killReq struct {
	Target string `json:"target"`
}

func (d *Deps) nsForLab(lab string) string {
	switch lab {
	case "standalone":
		return d.NSStandalone
	case "sentinel":
		return d.NSSentinel
	case "cluster":
		return d.NSCluster
	}
	return ""
}

func (d *Deps) handleKill(c *gin.Context) {
	lab := c.Param("lab")
	ns := d.nsForLab(lab)
	if ns == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown lab"})
		return
	}
	var body killReq
	if err := c.BindJSON(&body); err != nil || body.Target == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing target"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	if err := fault.Kill(ctx, d.K8s, ns, body.Target); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "Pod " + body.Target + " deleted"})
}

func (d *Deps) handleReset(c *gin.Context) {
	lab := c.Param("lab")
	ns := d.nsForLab(lab)
	if ns == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown lab"})
		return
	}
	var (
		release, values string
		chart           string
	)
	switch lab {
	case "standalone":
		release = d.ReleaseStandalone
		values = d.ValuesStandalone
		chart = "bitnami/redis"
	case "sentinel":
		release = d.ReleaseSentinel
		values = d.ValuesSentinel
		chart = "bitnami/redis"
	case "cluster":
		release = d.ReleaseCluster
		values = d.ValuesCluster
		chart = "bitnami/redis-cluster"
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Minute)
	defer cancel()
	if err := k8s.HelmUninstall(ctx, release, ns); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "uninstall: " + err.Error()})
		return
	}
	_ = k8s.KubectlDeletePVCs(ctx, ns)
	time.Sleep(3 * time.Second)
	if err := k8s.HelmInstall(ctx, release, chart, ns, values); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "install: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "Lab " + lab + " reset"})
}
