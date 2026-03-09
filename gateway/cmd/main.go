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
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	slog.SetDefault(logger)
	cfg := loadConfig()

	ctx := context.Background()

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

	router := api.NewRouter(pool, deployer)

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

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("forced shutdown", "error", err)
	}
	slog.Info("gateway stopped")
}

type GatewayConfig struct {
	Port           string
	Env            string
	DatabaseURL    string
	K8sInCluster   bool
	K8sKubeconfig  string
	K8sNamespace   string
}

func loadConfig() GatewayConfig {
	home, _ := os.UserHomeDir()
	defaultKubeconfig := filepath.Join(home, ".kube", "config")

	return GatewayConfig{
		Port:          envOr("BF_PORT", "8080"),
		Env:           envOr("BF_ENV", "dev"),
		DatabaseURL:   envOr("BF_DATABASE_URL", "postgres://bifrost:localdev@localhost:5432/bifrost?sslmode=disable"),
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
