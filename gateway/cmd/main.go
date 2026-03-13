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

	"duolok/bifrost/gateway/internal/api"
	"duolok/bifrost/gateway/internal/db"
	"duolok/bifrost/gateway/internal/k8s"
	"duolok/bifrost/gateway/internal/pubsub"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	slog.SetDefault(logger)
	cfg := loadConfig()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	// K8s deployer is optional — gateway works without it for local dev
	var deployer *k8s.Deployer
	deployer, err = k8s.NewDeployer(k8s.DeployConfig{
		Namespace:  cfg.K8sNamespace,
		InCluster:  cfg.K8sInCluster,
		Kubeconfig: cfg.K8sKubeconfig,
	}, pool)
	if err != nil {
		slog.Warn("k8s deployer unavailable, deploy endpoints disabled", "error", err)
		deployer = nil
	}

	var publisher *pubsub.Publisher
	if cfg.GCPProject != "" {
		publisher, err = pubsub.NewPublisher(ctx, cfg.GCPProject, "build-requests", cfg.ARRepo)
		if err != nil {
			slog.Warn("pubsub publisher unavailable, build requests won't be published", "error", err)
			publisher = nil
		} else {
			defer publisher.Stop()
		}
	}

	if cfg.GCPProject != "" {
		sub, err := pubsub.NewSubscriber(ctx, pool, deployer, cfg.GCPProject, "gateway-build-complete")
		if err != nil {
			slog.Warn("pubsub subscriber unavailable", "error", err)
		} else {
			go func() {
				if err := sub.Start(ctx); err != nil {
					slog.Error("build-complete subscriber stopped", "error", err)
				}
			}()
		}
	}

	router := api.NewRouter(pool, deployer, publisher)

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: router,
	}

	go func() {
		slog.Info("gateway starting", "port", cfg.Port, "env", cfg.Env)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("shutting down gateway")

	cancel() // Stop subscriber

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("forced shutdown", "error", err)
	}
	slog.Info("gateway stopped")
}

type GatewayConfig struct {
	Port          string
	Env           string
	DatabaseURL   string
	GCPProject    string
	ARRepo        string
	K8sInCluster  bool
	K8sKubeconfig string
	K8sNamespace  string
}

func loadConfig() GatewayConfig {
	home, _ := os.UserHomeDir()
	defaultKubeconfig := filepath.Join(home, ".kube", "config")

	return GatewayConfig{
		Port:          envOr("BF_PORT", "8080"),
		Env:           envOr("BF_ENV", "dev"),
		DatabaseURL:   envOr("BF_DATABASE_URL", "postgres://bifrost:localdev@localhost:5432/bifrost?sslmode=disable"),
		GCPProject:    os.Getenv("BF_GCP_PROJECT"),
		ARRepo:        os.Getenv("BF_AR_REPO"),
		K8sInCluster:  os.Getenv("BF_K8S_IN_CLUSTER") == "true",
		K8sKubeconfig: envOr("BF_KUBECONFIG", defaultKubeconfig),
		K8sNamespace:  envOr("BF_K8S_NAMESPACE", "bifrost-apps"),
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
