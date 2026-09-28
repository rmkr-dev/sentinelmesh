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

	"github.com/rmkr-dev/sentinelmesh/internal/ai"
	"github.com/rmkr-dev/sentinelmesh/internal/api"
	"github.com/rmkr-dev/sentinelmesh/internal/config"
	"github.com/rmkr-dev/sentinelmesh/internal/domain"
	"github.com/rmkr-dev/sentinelmesh/internal/engine"
	"github.com/rmkr-dev/sentinelmesh/internal/remediation"
	"github.com/rmkr-dev/sentinelmesh/internal/runbook"
	"github.com/rmkr-dev/sentinelmesh/internal/store"
	"github.com/rmkr-dev/sentinelmesh/internal/telemetryquery"
	"github.com/rmkr-dev/sentinelmesh/internal/version"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	dir := os.Getenv("PLATFORM_CONFIG")
	if dir == "" {
		dir = "config"
	}
	cfg, err := config.Load(dir, os.Getenv("PLATFORM_ENV"))
	if err != nil {
		slog.Error("config", "error", err.Error())
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	st, closeStore, err := openStore(ctx, cfg)
	if err != nil {
		slog.Error("store", "error", err.Error())
		os.Exit(1)
	}
	defer closeStore()

	if err := seed(ctx, st, cfg, dir); err != nil {
		slog.Error("seed", "error", err.Error())
		os.Exit(1)
	}
	books, err := runbook.LoadDir(filepath.Join(dir, "..", cfg.Paths.Runbooks))
	if err != nil {
		// Runbooks may live at the repository root rather than under config/.
		books, err = runbook.LoadDir(env("RUNBOOK_DIR", "runbooks"))
	}
	if err != nil {
		slog.Error("runbooks", "error", err.Error())
		os.Exit(1)
	}
	if env("RUNBOOK_DIR", "") == "" {
		if extra, err := runbook.LoadDir("/app/runbooks"); err == nil && len(extra) > 0 {
			books = extra
		}
	}

	prompt := ""
	if b, err := os.ReadFile(filepath.Join(env("PROMPT_DIR", "prompts"), "root-cause-analysis", "system.md")); err == nil {
		prompt = string(b)
	}
	provider := buildAI(cfg.AI, prompt)
	eng := &engine.Engine{
		Store: st,
		Deps: engine.Dependencies{
			Metrics: telemetryquery.PrometheusClient{BaseURL: cfg.Telemetry.PrometheusURL},
			Traces:  &telemetryquery.JaegerClient{BaseURL: cfg.Telemetry.JaegerURL},
			Logs:    &telemetryquery.LokiClient{BaseURL: cfg.Telemetry.LokiURL},
		},
		Conventions: telemetryquery.Conventions{
			RequestMetric: cfg.Telemetry.RequestMetric,
			ServiceLabel:  cfg.Telemetry.ServiceLabel,
			StatusLabel:   cfg.Telemetry.StatusLabel,
		},
		Runbooks:  books,
		Window:    cfg.Engine.CorrelationWindow,
		Lookback:  cfg.Engine.DeploymentLookback,
		Log:       logger,
		AI:        provider,
		AIEnabled: cfg.AI.Enabled && provider != nil,
	}
	if cfg.Telemetry.PrometheusURL == "" {
		eng.Deps.Metrics = nil
	}

	allowed := map[string]bool{}
	for _, a := range cfg.Remediation.AllowedActions {
		allowed[a] = true
	}
	gate := remediation.Gate{Policy: remediation.Policy{
		Enabled:         cfg.Remediation.Enabled,
		RequireApproval: cfg.Remediation.RequireApproval,
		Allowed:         allowed,
	}}
	exec := remediation.DemoExecutor{Apply: demoApply(st)}

	srv := &api.Server{
		Store:      st,
		Engine:     eng,
		Runbooks:   books,
		Gate:       gate,
		Executor:   exec,
		Demo:       cfg.Demo.Enabled,
		Token:      cfg.Auth.Token,
		WebDir:     env("WEB_DIR", "web"),
		GrafanaURL: cfg.UI.GrafanaURL,
		JaegerURL:  cfg.UI.JaegerURL,
		Log:        logger,
	}
	httpServer := &http.Server{
		Addr:              cfg.HTTP.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		ticker := time.NewTicker(cfg.Engine.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := eng.Tick(ctx); err != nil {
					slog.Warn("engine tick", "error", err.Error())
				}
			}
		}
	}()
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(c)
	}()
	slog.Info("platform listening", "addr", cfg.HTTP.Addr, "version", version.Version, "env", cfg.Environment, "ai", cfg.AI.Provider)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("http", "error", err.Error())
		os.Exit(1)
	}
}

func openStore(ctx context.Context, cfg config.Config) (store.Store, func(), error) {
	if cfg.Store == "postgres" {
		if cfg.DatabaseURL == "" {
			return nil, nil, errString("DATABASE_URL is required when store=postgres")
		}
		pg, err := store.NewPostgres(ctx, cfg.DatabaseURL)
		if err != nil {
			return nil, nil, err
		}
		return pg, pg.Close, nil
	}
	slog.Warn("using in-memory store; incidents disappear when the process exits")
	return store.NewMemory(), func() {}, nil
}

func seed(ctx context.Context, st store.Store, cfg config.Config, dir string) error {
	services, err := config.LoadServices(filepath.Join(dir, cfg.Paths.Services), cfg.Environment)
	if err != nil {
		return err
	}
	for _, svc := range services {
		svc.Environment = cfg.Environment
		if err := st.UpsertService(ctx, svc); err != nil {
			return err
		}
	}
	defs, err := config.LoadSLOs(filepath.Join(dir, cfg.Paths.SLOs), nil)
	if err != nil {
		return err
	}
	for _, def := range defs {
		if err := st.UpsertSLO(ctx, def); err != nil {
			return err
		}
	}
	return nil
}

func buildAI(cfg config.AIConfig, prompt string) ai.Provider {
	if !cfg.Enabled {
		return nil
	}
	switch cfg.Provider {
	case "", "mock":
		return ai.MockProvider{}
	case "openai-compatible", "local":
		if cfg.BaseURL == "" || cfg.Model == "" {
			slog.Warn("ai provider is missing base_url or model; analysis stays deterministic")
			return nil
		}
		return ai.NewChatProvider(ai.ChatConfig{
			Name: cfg.Provider, BaseURL: cfg.BaseURL, APIKey: cfg.APIKey, Model: cfg.Model,
			Temperature: cfg.Temperature, MaxTokens: cfg.MaxTokens, SystemPrompt: prompt,
		})
	case "azure-openai":
		if cfg.BaseURL == "" || cfg.Model == "" {
			slog.Warn("azure openai is missing base_url or deployment; analysis stays deterministic")
			return nil
		}
		return ai.NewChatProvider(ai.ChatConfig{
			Name: "azure-openai", BaseURL: cfg.BaseURL, APIKey: cfg.APIKey, Model: cfg.Model,
			Temperature: cfg.Temperature, MaxTokens: cfg.MaxTokens, APIVersion: cfg.APIVersion,
			Azure: true, SystemPrompt: prompt,
		})
	default:
		slog.Warn("unknown ai provider; analysis stays deterministic", "provider", cfg.Provider)
		return nil
	}
}

func demoApply(st store.Store) func(context.Context, domain.RemediationRequest) (map[string]any, map[string]any, error) {
	return func(ctx context.Context, req domain.RemediationRequest) (map[string]any, map[string]any, error) {
		before := map[string]any{"action": req.Action, "target": req.Target}
		switch req.Action {
		case "rollback_deployment":
			_ = st.DeleteFault(ctx, "deployment-regression")
			now := time.Now().UTC()
			_ = st.CreateDeployment(ctx, domain.Deployment{
				ID: "rollback-" + req.ID, Service: req.Target, Version: "v1.8.3", Environment: "local",
				GitSHA: "good123", Repository: "github.com/rmkr-dev/sentinelmesh", Author: req.Approver, Timestamp: now,
				Metadata: map[string]string{"change": "approved-rollback"},
			})
			return before, map[string]any{"version": "v1.8.3", "fault": "deployment-regression disabled"}, nil
		case "restart_pod":
			for _, name := range []string{"cpu-pressure", "memory-pressure"} {
				_ = st.DeleteFault(ctx, name)
			}
			return before, map[string]any{"restarted": req.Target}, nil
		case "scale_deployment":
			return before, map[string]any{"replicas": req.Replicas, "note": "recorded for the local demo executor"}, nil
		default:
			return before, nil, errString("unsupported demo action")
		}
	}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

type errString string

func (e errString) Error() string { return string(e) }
