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

	"crypto/rand"
	"encoding/hex"

	"duolok/bifrost/gateway/internal/api"
	"duolok/bifrost/gateway/internal/db"
	"duolok/bifrost/gateway/internal/events"
	"duolok/bifrost/gateway/internal/k8s"
	"duolok/bifrost/gateway/internal/notify"
	"duolok/bifrost/gateway/internal/pubsub"
	"duolok/bifrost/gateway/internal/rules"
	"duolok/bifrost/gateway/internal/validator"
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

	var emitter *events.Emitter
	if cfg.RealtimeURL != "" {
		var emitErr error
		emitter, emitErr = events.NewEmitter(cfg.RealtimeURL)
		if emitErr != nil {
			slog.Warn("event emitter unavailable", "error", emitErr)
			emitter = nil
		} else {
			defer emitter.Close()
			slog.Info("event emitter configured", "url", cfg.RealtimeURL)
		}
	}

	var deployer *k8s.Deployer
	deployer, err = k8s.NewDeployer(k8s.DeployConfig{
		Namespace:  cfg.K8sNamespace,
		InCluster:  cfg.K8sInCluster,
		Kubeconfig: cfg.K8sKubeconfig,
	}, pool, emitter)
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
		sub, err := pubsub.NewSubscriber(ctx, pool, deployer, emitter, cfg.GCPProject, "gateway-build-complete")
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

	var validatorClient *validator.Client
	if cfg.ValidatorURL != "" {
		validatorClient = validator.NewClient(cfg.ValidatorURL)
		slog.Info("validator configured", "url", cfg.ValidatorURL)
	} else {
		slog.Warn("validator not configured, config validation will be skipped")
	}

	var notifier *notify.Publisher
	if cfg.RabbitMqURL != "" {
		var notifyErr error
		notifier, notifyErr = notify.NewPublisher(cfg.RabbitMqURL)
		if notifyErr != nil {
			slog.Warn("rabbitmq unavailable, notifications disabled", "error", notifyErr)
			notifier = nil
		} else {
			defer notifier.Close()
		}
	}

	// JWT secret: use env var or generate a random one for dev
	jwtSecret := []byte(cfg.JWTSecret)
	if len(jwtSecret) == 0 {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			slog.Error("failed to generate JWT secret", "error", err)
			os.Exit(1)
		}
		jwtSecret = []byte(hex.EncodeToString(b))
		slog.Warn("BF_JWT_SECRET not set, using random secret (tokens won't survive restarts)")
	}

	oauthCfg := &api.OAuthConfig{
		GoogleClientID:     cfg.GoogleClientID,
		GoogleClientSecret: cfg.GoogleClientSecret,
		GitHubClientID:     cfg.GitHubClientID,
		GitHubClientSecret: cfg.GitHubClientSecret,
		CallbackBaseURL:    cfg.AuthCallbackURL,
	}

	if oauthCfg.GoogleEnabled() {
		slog.Info("Google OAuth configured")
	}

	if oauthCfg.GitHubEnabled() {
		slog.Info("GitHub OAuth configured")
	}

	rulesEngine := rules.NewEngine(notifier)
	router := api.NewRouter(api.HandlerDeps{
		Pool:      pool,
		Deployer:  deployer,
		Publisher: publisher,
		Validator: validatorClient,
		Emitter:   emitter,
		Notifier:  notifier,
		Rules:     rulesEngine,
		JWTSecret: jwtSecret,
		OAuth:     oauthCfg,
	}, jwtSecret, pool)

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

	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("forced shutdown", "error", err)
	}
	slog.Info("gateway stopped")
}

type GatewayConfig struct {
	Port               string
	Env                string
	DatabaseURL        string
	GCPProject         string
	ARRepo             string
	K8sInCluster       bool
	K8sKubeconfig      string
	K8sNamespace       string
	RealtimeURL        string
	ValidatorURL       string
	RabbitMqURL        string
	JWTSecret          string
	GoogleClientID     string
	GoogleClientSecret string
	GitHubClientID     string
	GitHubClientSecret string
	AuthCallbackURL    string
}

func loadConfig() GatewayConfig {
	home, _ := os.UserHomeDir()
	defaultKubeconfig := filepath.Join(home, ".kube", "config")

	return GatewayConfig{
		Port:               envOr("BF_PORT", "8080"),
		Env:                envOr("BF_ENV", "dev"),
		DatabaseURL:        envOr("BF_DATABASE_URL", "postgres://bifrost:localdev@localhost:5432/bifrost?sslmode=disable"),
		GCPProject:         os.Getenv("BF_GCP_PROJECT"),
		ARRepo:             os.Getenv("BF_AR_REPO"),
		K8sInCluster:       os.Getenv("BF_K8S_IN_CLUSTER") == "true",
		K8sKubeconfig:      envOr("BF_KUBECONFIG", defaultKubeconfig),
		K8sNamespace:       envOr("BF_K8S_NAMESPACE", "bifrost-apps"),
		RealtimeURL:        os.Getenv("BF_REALTIME_URL"),
		ValidatorURL:       os.Getenv("BF_VALIDATOR_URL"),
		RabbitMqURL:        envOr("BF_RABBITMQ_URL", "amqp://bifrost:localdev@localhost:5672/"),
		JWTSecret:          os.Getenv("BF_JWT_SECRET"),
		GoogleClientID:     os.Getenv("BF_GOOGLE_CLIENT_ID"),
		GoogleClientSecret: os.Getenv("BF_GOOGLE_CLIENT_SECRET"),
		GitHubClientID:     os.Getenv("BF_GITHUB_CLIENT_ID"),
		GitHubClientSecret: os.Getenv("BF_GITHUB_CLIENT_SECRET"),
		AuthCallbackURL:    envOr("BF_AUTH_CALLBACK_URL", "http://localhost:8080"),
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
