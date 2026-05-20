// redis-lab backend. Boots K8s client, redis clients for 3 labs, an event
// broadcaster fed by pod watchers + sentinel pub/sub, and an HTTP server.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/redis-lab/backend/internal/api"
	"github.com/redis-lab/backend/internal/events"
	"github.com/redis-lab/backend/internal/k8s"
	redislab "github.com/redis-lab/backend/internal/redis"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg := loadConfig()
	slog.Info("starting redis-lab backend",
		"addr", cfg.HTTPAddr,
		"standaloneRedis", cfg.StandaloneAddr,
		"sentinelAddr", cfg.SentinelAddr,
		"clusterAddr", cfg.ClusterAddr)

	kc, err := k8s.NewClient()
	if err != nil {
		slog.Error("kube client", "err", err)
		os.Exit(1)
	}

	bc := events.New()
	rs := redislab.NewStandalone(cfg.StandaloneAddr)
	rsen := redislab.NewSentinel(cfg.SentinelAddr, cfg.SentinelMasterName)
	rsen.ExecFn = func(ctx context.Context, pod string, args []string) (string, string, error) {
		// Bitnami sentinel pods have two containers: "redis" + "sentinel".
		return kc.Exec(ctx, "redis-sentinel", pod, "redis", args)
	}
	rc := redislab.NewCluster([]string{cfg.ClusterAddr})
	rc.PodName = "redis-cluster-0"
	rc.ExecFn = func(ctx context.Context, args []string) (string, string, error) {
		return kc.Exec(ctx, "redis-cluster", rc.PodName, "", args)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Watchers
	go kc.WatchPods(ctx, "redis-standalone", "standalone", bc)
	go kc.WatchPods(ctx, "redis-sentinel", "sentinel", bc)
	go kc.WatchPods(ctx, "redis-cluster", "cluster", bc)
	go redislab.WatchSentinelEvents(ctx, cfg.SentinelAddr, "sentinel", bc)

	deps := &api.Deps{
		K8s:               kc,
		Broadcaster:       bc,
		Standalone:        rs,
		Sentinel:          rsen,
		Cluster:           rc,
		NSStandalone:      "redis-standalone",
		NSSentinel:        "redis-sentinel",
		NSCluster:         "redis-cluster",
		ReleaseStandalone: "redis-standalone",
		ReleaseSentinel:   "redis-sentinel",
		ReleaseCluster:    "redis-cluster",
		ValuesStandalone:  cfg.ValuesStandalone,
		ValuesSentinel:    cfg.ValuesSentinel,
		ValuesCluster:     cfg.ValuesCluster,
	}

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           api.NewRouter(deps),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		slog.Info("http listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http serve", "err", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	slog.Info("shutting down")
	shutdownCtx, scancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer scancel()
	_ = srv.Shutdown(shutdownCtx)
	_ = rs.Close()
	_ = rsen.Close()
	_ = rc.Close()
}

type config struct {
	HTTPAddr           string
	StandaloneAddr     string
	SentinelAddr       string
	SentinelMasterName string
	ClusterAddr        string
	ValuesStandalone   string
	ValuesSentinel     string
	ValuesCluster      string
}

func loadConfig() config {
	repoRoot := envOr("REPO_ROOT", "..")
	if abs, err := filepath.Abs(repoRoot); err == nil {
		repoRoot = abs
	}
	return config{
		HTTPAddr:           envOr("HTTP_ADDR", ":8080"),
		StandaloneAddr:     envOr("REDIS_STANDALONE_ADDR", "localhost:6379"),
		SentinelAddr:       envOr("REDIS_SENTINEL_ADDR", "localhost:26379"),
		SentinelMasterName: envOr("REDIS_SENTINEL_MASTER", "mymaster"),
		ClusterAddr:        envOr("REDIS_CLUSTER_ADDR", "localhost:6381"),
		ValuesStandalone:   envOr("VALUES_STANDALONE", filepath.Join(repoRoot, "helm/values-standalone.yaml")),
		ValuesSentinel:     envOr("VALUES_SENTINEL", filepath.Join(repoRoot, "helm/values-sentinel.yaml")),
		ValuesCluster:      envOr("VALUES_CLUSTER", filepath.Join(repoRoot, "helm/values-cluster.yaml")),
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
