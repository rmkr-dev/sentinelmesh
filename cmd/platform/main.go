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
	"github.com/rmkr-dev/sentinelmesh/internal/anomaly"
	"github.com/rmkr-dev/sentinelmesh/internal/api"
	"github.com/rmkr-dev/sentinelmesh/internal/auth"
	"github.com/rmkr-dev/sentinelmesh/internal/azure"
	azauth "github.com/rmkr-dev/sentinelmesh/internal/azure/auth"
	"github.com/rmkr-dev/sentinelmesh/internal/config"
	"github.com/rmkr-dev/sentinelmesh/internal/domain"
	"github.com/rmkr-dev/sentinelmesh/internal/engine"
	"github.com/rmkr-dev/sentinelmesh/internal/kube"
	"github.com/rmkr-dev/sentinelmesh/internal/remediation"
	"github.com/rmkr-dev/sentinelmesh/internal/runbook"
	"github.com/rmkr-dev/sentinelmesh/internal/store"
	"github.com/rmkr-dev/sentinelmesh/internal/telemetry"
	"github.com/rmkr-dev/sentinelmesh/internal/telemetryquery"
	"github.com/rmkr-dev/sentinelmesh/internal/version"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
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
	tel, err := telemetry.Setup(ctx, telemetry.Options{ServiceName: "sentinelmesh"})
	if err != nil {
		slog.Error("telemetry", "error", err.Error())
		os.Exit(1)
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tel.Shutdown(c)
	}()
	logger = tel.Logger
	slog.SetDefault(logger)

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
	runbookDir, err := config.ResolveDir("RUNBOOK_DIR", cfg.Paths.Runbooks, "/app/runbooks")
	if err != nil {
		slog.Error("runbooks", "error", err.Error())
		os.Exit(1)
	}
	books, err := runbook.LoadDir(runbookDir)
	if err != nil {
		slog.Error("runbooks", "error", err.Error())
		os.Exit(1)
	}
	webDir, err := config.ResolveDir("WEB_DIR", cfg.Paths.Web, "/app/web")
	if err != nil {
		slog.Error("web", "error", err.Error())
		os.Exit(1)
	}
	promptDir, err := config.ResolveDir("PROMPT_DIR", cfg.Paths.Prompts, "/app/prompts")
	if err != nil {
		slog.Error("prompts", "error", err.Error())
		os.Exit(1)
	}
	prompt := ""
	if b, err := os.ReadFile(filepath.Join(promptDir, "root-cause-analysis", "system.md")); err == nil {
		prompt = string(b)
	}
	provider, err := buildAI(cfg.AI, prompt)
	if err != nil {
		slog.Error("ai", "error", err.Error())
		os.Exit(1)
	}
	deps := engine.Dependencies{}
	if cfg.Telemetry.Backend == "azure" && cfg.Azure.MonitorEndpoint != "" {
		cred, err := azauth.New(azauth.ModeFromEnv(cfg.Azure.AuthMode), cfg.Environment)
		if err != nil {
			slog.Error("azure metrics", "error", err.Error())
			os.Exit(1)
		}
		deps.Metrics = telemetryquery.AzureMetrics{Client: azure.Client{Credential: cred}, Endpoint: cfg.Azure.MonitorEndpoint}
	} else if cfg.Telemetry.PrometheusURL != "" {
		deps.Metrics = telemetryquery.PrometheusClient{BaseURL: cfg.Telemetry.PrometheusURL}
	}
	if cfg.Telemetry.JaegerURL != "" {
		deps.Traces = telemetryquery.JaegerClient{BaseURL: cfg.Telemetry.JaegerURL}
	}
	if cfg.Telemetry.LokiURL != "" {
		deps.Logs = telemetryquery.LokiClient{BaseURL: cfg.Telemetry.LokiURL}
	}
	eng := &engine.Engine{
		Store: st,
		Deps:  deps,
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
		Redaction: cfg.Telemetry.Policy(),
		Retention: cfg.Engine.AnomalyRetention,
		Detectors: detectorSpecs(cfg),
	}
	if cfg.Kubernetes.Enabled {
		host := os.Getenv("KUBERNETES_SERVICE_HOST")
		port := os.Getenv("KUBERNETES_SERVICE_PORT")
		if port == "" {
			port = "443"
		}
		httpClient, _, err := kube.InClusterClient("", "")
		if err != nil {
			slog.Error("kubernetes client", "error", err.Error())
			os.Exit(1)
		}
		eng.Cluster = kube.Client{
			BaseURL: "https://" + host + ":" + port, HTTP: httpClient,
			ServiceLabel: firstNonEmpty(cfg.Kubernetes.ServiceLabel, "app.kubernetes.io/name"),
		}
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
	exec, err := remediation.Select(remediation.SelectOptions{
		Executor:           cfg.Remediation.Executor,
		Namespace:          cfg.Remediation.Namespace,
		DemoEnabled:        cfg.Demo.Enabled,
		RemediationEnabled: cfg.Remediation.Enabled,
		DemoApply:          demoApply(st),
		TokenPath:          "/var/run/secrets/kubernetes.io/serviceaccount/token",
		CAPath:             "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt",
		AzureAllow:         allowed,
	})
	if err != nil {
		slog.Error("remediation executor", "error", err.Error())
		os.Exit(1)
	}

	var verifier *auth.Verifier
	if cfg.Auth.OIDC.Issuer != "" {
		verifier, err = auth.NewVerifier(ctx, auth.Config{
			Issuer: cfg.Auth.OIDC.Issuer, Audience: cfg.Auth.OIDC.Audience, RolesClaim: cfg.Auth.OIDC.RolesClaim,
		})
		if err != nil {
			slog.Error("oidc", "error", err.Error())
			os.Exit(1)
		}
	}
	srv := &api.Server{
		Store:             st,
		Engine:            eng,
		Runbooks:          books,
		Gate:              gate,
		Executor:          exec,
		Demo:              cfg.Demo.Enabled,
		Environment:       cfg.Environment,
		HonorActorHeader:  cfg.Demo.Enabled && (cfg.Environment == "local" || cfg.Environment == "dev"),
		Token:             cfg.Auth.Token,
		WebhookToken:      cfg.Auth.WebhookToken,
		Principals:        principalsFrom(cfg),
		OIDC:              verifier,
		CorrelationWindow: cfg.Engine.CorrelationWindow,
		Redaction:         cfg.Telemetry.Policy(),
		WebDir:            webDir,
		GrafanaURL:        cfg.UI.GrafanaURL,
		JaegerURL:         cfg.UI.JaegerURL,
		Log:               logger,
	}
	eng.OnSLO = srv.PublishSLO
	if cfg.Azure.Enabled {
		go runAzurePoller(ctx, cfg, st, logger)
	}
	httpServer := &http.Server{
		Addr:              cfg.HTTP.Addr,
		Handler:           otelhttp.NewHandler(srv.Handler(), "http"),
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
				if l, ok := st.(interface {
					TryLock(context.Context, int64) (bool, error)
				}); ok {
					got, err := l.TryLock(ctx, 424242)
					if err != nil {
						slog.Warn("leader election", "error", err.Error())
						continue
					}
					if !got {
						continue
					}
				}
				if err := eng.Tick(ctx); err != nil {
					slog.Warn("engine tick", "error", err.Error())
				}
			}
		}
	}()
	go func() {
		<-ctx.Done()
		if pg, ok := st.(*store.Postgres); ok {
			pg.ReleaseLock()
		}
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

func buildAI(cfg config.AIConfig, prompt string) (ai.Provider, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	switch cfg.Provider {
	case "openai-compatible", "local":
		if cfg.BaseURL == "" || cfg.Model == "" {
			return nil, errString("ai.provider requires base_url and model when enabled")
		}
		return ai.NewChatProvider(ai.ChatConfig{
			Name: cfg.Provider, BaseURL: cfg.BaseURL, APIKey: cfg.APIKey, Model: cfg.Model,
			Temperature: cfg.Temperature, MaxTokens: cfg.MaxTokens, SystemPrompt: prompt,
		}), nil
	case "azure-openai":
		if cfg.BaseURL == "" || cfg.Model == "" {
			return nil, errString("azure openai requires base_url and model when enabled")
		}
		return ai.NewChatProvider(ai.ChatConfig{
			Name: "azure-openai", BaseURL: cfg.BaseURL, APIKey: cfg.APIKey, Model: cfg.Model,
			Temperature: cfg.Temperature, MaxTokens: cfg.MaxTokens, APIVersion: cfg.APIVersion,
			Azure: true, SystemPrompt: prompt,
		}), nil
	default:
		return nil, errString("ai.provider is required when ai.enabled is true")
	}
}

func principalsFrom(cfg config.Config) []api.Principal {
	var out []api.Principal
	if cfg.Auth.Token != "" {
		out = append(out, api.Principal{Name: "api", Token: cfg.Auth.Token, Role: auth.RoleAdmin})
	}
	for _, p := range cfg.Auth.Principals {
		if p.Token == "" {
			continue
		}
		out = append(out, api.Principal{Name: p.Name, Token: p.Token, Role: p.Role})
	}
	if cfg.Auth.WebhookToken != "" {
		out = append(out, api.Principal{Name: "alertmanager", Token: cfg.Auth.WebhookToken, Role: auth.RoleResponder, WebhookOnly: true})
	}
	return out
}

func detectorSpecs(cfg config.Config) []engine.DetectorSpec {
	var out []engine.DetectorSpec
	for _, d := range cfg.Engine.Detectors {
		out = append(out, engine.DetectorSpec{
			Name: d.Name, Metric: d.Metric,
			Params: anomaly.Params{Threshold: d.Threshold, Window: d.Window, MinPoints: d.MinPoints},
		})
	}
	return out
}

func runAzurePoller(ctx context.Context, cfg config.Config, st store.Store, logger *slog.Logger) {
	cred, err := azauth.New(azauth.ModeFromEnv(cfg.Azure.AuthMode), cfg.Environment)
	if err != nil {
		logger.Error("azure poller", "error", err.Error())
		return
	}
	dir := cfg.Azure.ResourceTypes
	if dir == "" {
		dir = "config/azure/resource-types"
	}
	types, err := azure.LoadResourceTypes(dir)
	if err != nil {
		logger.Error("azure resource types", "error", err.Error())
		return
	}
	poller := azure.Poller{
		Client:       azure.Client{Credential: cred},
		Subscription: cfg.Azure.Subscription,
		WorkspaceID:  cfg.Azure.WorkspaceID,
		Types:        types,
	}
	tick := func() {
		if err := poller.Poll(ctx, st); err != nil {
			logger.Warn("azure poll", "error", err.Error())
		}
	}
	tick()
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tick()
		}
	}
}

func firstNonEmpty(v, def string) string {
	if v != "" {
		return v
	}
	return def
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

type errString string

func (e errString) Error() string { return string(e) }
