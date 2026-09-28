// Package config loads base configuration plus an environment overlay.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
	"github.com/rmkr-dev/sentinelmesh/internal/slo"
	"gopkg.in/yaml.v3"
)

// Config is the process configuration.
type Config struct {
	Environment string            `yaml:"environment"`
	HTTP        HTTPConfig        `yaml:"http"`
	DatabaseURL string            `yaml:"database_url"`
	Store       string            `yaml:"store"`
	Telemetry   TelemetryConfig   `yaml:"telemetry"`
	Engine      EngineConfig      `yaml:"engine"`
	AI          AIConfig          `yaml:"ai"`
	Remediation RemediationConfig `yaml:"remediation"`
	Demo        DemoConfig        `yaml:"demo"`
	Auth        AuthConfig        `yaml:"auth"`
	UI          UIConfig          `yaml:"ui"`
	Paths       PathsConfig       `yaml:"paths"`
}

type HTTPConfig struct {
	Addr string `yaml:"addr"`
}

type TelemetryConfig struct {
	PrometheusURL  string   `yaml:"prometheus_url"`
	JaegerURL      string   `yaml:"jaeger_url"`
	LokiURL        string   `yaml:"loki_url"`
	RequestMetric  string   `yaml:"request_metric"`
	ServiceLabel   string   `yaml:"service_label"`
	StatusLabel    string   `yaml:"status_label"`
	RedactEmails   bool     `yaml:"redact_emails"`
	HeaderDenylist []string `yaml:"header_denylist"`
	QueryDenylist  []string `yaml:"query_denylist"`
}

type EngineConfig struct {
	Interval           time.Duration `yaml:"interval"`
	CorrelationWindow  time.Duration `yaml:"correlation_window"`
	DeploymentLookback time.Duration `yaml:"deployment_lookback"`
}

type AIConfig struct {
	Enabled     bool    `yaml:"enabled"`
	Provider    string  `yaml:"provider"`
	Model       string  `yaml:"model"`
	BaseURL     string  `yaml:"base_url"`
	APIKey      string  `yaml:"api_key"`
	APIVersion  string  `yaml:"api_version"`
	Temperature float64 `yaml:"temperature"`
	MaxTokens   int     `yaml:"max_tokens"`
}

type RemediationConfig struct {
	Enabled         bool     `yaml:"enabled"`
	RequireApproval bool     `yaml:"require_approval"`
	Executor        string   `yaml:"executor"`
	AllowedActions  []string `yaml:"allowed_actions"`
	Namespace       string   `yaml:"namespace"`
}

type DemoConfig struct {
	Enabled bool `yaml:"enabled"`
}

type AuthConfig struct {
	Token string `yaml:"token"`
}

type UIConfig struct {
	GrafanaURL string `yaml:"grafana_url"`
	JaegerURL  string `yaml:"jaeger_url"`
}

type PathsConfig struct {
	Services string `yaml:"services"`
	SLOs     string `yaml:"slos"`
	Runbooks string `yaml:"runbooks"`
	Prompts  string `yaml:"prompts"`
	Web      string `yaml:"web"`
}

// ServiceFile is the onboarding document for one service.
type ServiceFile struct {
	Name         string            `yaml:"name"`
	Team         string            `yaml:"team"`
	Owner        string            `yaml:"owner"`
	Criticality  string            `yaml:"criticality"`
	Environment  string            `yaml:"environment"`
	Version      string            `yaml:"version"`
	Repository   string            `yaml:"repository"`
	Dependencies []string          `yaml:"dependencies"`
	Attributes   map[string]string `yaml:"attributes"`
}

// SLOFile is the YAML SLO document.
type SLOFile struct {
	Service string             `yaml:"service"`
	SLOs    map[string]SLOSpec `yaml:"slos"`
}

// SLOSpec matches the documented SLO template.
type SLOSpec struct {
	Target      string   `yaml:"target"`
	ThresholdMS float64  `yaml:"threshold_ms"`
	Windows     []string `yaml:"windows"`
	Description string   `yaml:"description"`
}

// Load reads base.yaml and overlays environments/<env>.yaml.
// Environment variables override secrets and endpoints.
func Load(dir, environment string) (Config, error) {
	if environment == "" {
		environment = os.Getenv("PLATFORM_ENV")
	}
	if environment == "" {
		environment = "local"
	}
	cfg := defaults(environment)
	base, err := readMap(filepath.Join(dir, "base.yaml"))
	if err != nil && !os.IsNotExist(err) {
		return Config{}, err
	}
	overlay, err := readMap(filepath.Join(dir, "environments", environment+".yaml"))
	if err != nil && !os.IsNotExist(err) {
		return Config{}, err
	}
	merged := deepMerge(base, overlay)
	raw, err := yaml.Marshal(merged)
	if err != nil {
		return Config{}, err
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return Config{}, err
	}
	cfg.Environment = environment
	applyEnv(&cfg)
	if cfg.HTTP.Addr == "" {
		cfg.HTTP.Addr = ":8080"
	}
	if cfg.Engine.Interval == 0 {
		cfg.Engine.Interval = 15 * time.Second
	}
	if cfg.Engine.CorrelationWindow == 0 {
		cfg.Engine.CorrelationWindow = 5 * time.Minute
	}
	if cfg.Engine.DeploymentLookback == 0 {
		cfg.Engine.DeploymentLookback = 30 * time.Minute
	}
	return cfg, nil
}

func defaults(env string) Config {
	return Config{
		Environment: env,
		HTTP:        HTTPConfig{Addr: ":8080"},
		Store:       "memory",
		Telemetry: TelemetryConfig{
			RequestMetric: "http_server_request_duration_seconds",
			ServiceLabel:  "service_name",
			StatusLabel:   "http_response_status_code",
			RedactEmails:  true,
		},
		Engine: EngineConfig{
			Interval:           15 * time.Second,
			CorrelationWindow:  5 * time.Minute,
			DeploymentLookback: 30 * time.Minute,
		},
		AI: AIConfig{Enabled: true, Provider: "mock", Temperature: 0, MaxTokens: 1200},
		Remediation: RemediationConfig{
			Enabled:         false,
			RequireApproval: true,
			Executor:        "demo",
			AllowedActions:  []string{"restart_pod", "scale_deployment", "rollback_deployment"},
		},
		Demo: DemoConfig{Enabled: env == "local"},
		Paths: PathsConfig{
			Services: "services",
			SLOs:     "slo",
			Runbooks: "runbooks",
			Prompts:  "prompts",
			Web:      "web",
		},
	}
}

func applyEnv(cfg *Config) {
	if v := os.Getenv("DATABASE_URL"); v != "" {
		cfg.DatabaseURL = v
		if os.Getenv("STORE") == "" && cfg.Store == "memory" {
			cfg.Store = "postgres"
		}
	}
	if v := os.Getenv("STORE"); v != "" {
		cfg.Store = v
	}
	if v := os.Getenv("HTTP_ADDR"); v != "" {
		cfg.HTTP.Addr = v
	}
	if v := os.Getenv("PROMETHEUS_URL"); v != "" {
		cfg.Telemetry.PrometheusURL = v
	}
	if v := os.Getenv("JAEGER_URL"); v != "" {
		cfg.Telemetry.JaegerURL = v
	}
	if v := os.Getenv("LOKI_URL"); v != "" {
		cfg.Telemetry.LokiURL = v
	}
	if v := os.Getenv("PLATFORM_API_TOKEN"); v != "" {
		cfg.Auth.Token = v
	}
	if v := os.Getenv("AI_PROVIDER"); v != "" {
		cfg.AI.Provider = v
	}
	if v := os.Getenv("AI_MODEL"); v != "" {
		cfg.AI.Model = v
	}
	if v := os.Getenv("AI_BASE_URL"); v != "" {
		cfg.AI.BaseURL = v
	}
	if v := os.Getenv("AI_API_KEY"); v != "" {
		cfg.AI.APIKey = v
	}
	if v := os.Getenv("AI_API_VERSION"); v != "" {
		cfg.AI.APIVersion = v
	}
	if v := os.Getenv("AI_ENABLED"); v != "" {
		cfg.AI.Enabled = v == "true" || v == "1"
	}
	if v := os.Getenv("DEMO_ENABLED"); v != "" {
		cfg.Demo.Enabled = v == "true" || v == "1"
	}
	if v := os.Getenv("GRAFANA_URL"); v != "" {
		cfg.UI.GrafanaURL = v
	}
	if v := os.Getenv("JAEGER_UI_URL"); v != "" {
		cfg.UI.JaegerURL = v
	}
}

// LoadServices reads service catalog files.
func LoadServices(dir, environment string) ([]domain.Service, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []domain.Service
	for _, e := range entries {
		if e.IsDir() || !(strings.HasSuffix(e.Name(), ".yaml") || strings.HasSuffix(e.Name(), ".yml")) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var sf ServiceFile
		if err := yaml.Unmarshal(b, &sf); err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		if sf.Name == "" {
			return nil, fmt.Errorf("%s: service name is required", e.Name())
		}
		env := sf.Environment
		if env == "" {
			env = environment
		}
		out = append(out, domain.Service{
			Name:         sf.Name,
			Team:         sf.Team,
			Owner:        sf.Owner,
			Criticality:  sf.Criticality,
			Environment:  env,
			Version:      sf.Version,
			Repository:   sf.Repository,
			Dependencies: sf.Dependencies,
			Attributes:   sf.Attributes,
			UpdatedAt:    time.Now().UTC(),
		})
	}
	return out, nil
}

// LoadSLOs reads SLO documents and expands windows.
func LoadSLOs(dir string, windowDefaults map[string]time.Duration) ([]domain.SLODefinition, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if windowDefaults == nil {
		windowDefaults = map[string]time.Duration{
			"1m":  time.Minute,
			"5m":  5 * time.Minute,
			"30m": 30 * time.Minute,
			"1h":  time.Hour,
			"6h":  6 * time.Hour,
			"30d": 30 * 24 * time.Hour,
		}
	}
	var out []domain.SLODefinition
	for _, e := range entries {
		if e.IsDir() || !(strings.HasSuffix(e.Name(), ".yaml") || strings.HasSuffix(e.Name(), ".yml")) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var doc SLOFile
		if err := yaml.Unmarshal(b, &doc); err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		for name, spec := range doc.SLOs {
			objective, err := slo.ParseObjective(spec.Target)
			if err != nil {
				return nil, fmt.Errorf("%s %s: %w", e.Name(), name, err)
			}
			indicator := name
			windows := spec.Windows
			if len(windows) == 0 {
				windows = []string{"5m", "1h", "30d"}
			}
			var parsed []domain.SLOWindow
			for _, w := range windows {
				dur, ok := windowDefaults[w]
				if !ok {
					dur, err = time.ParseDuration(w)
					if err != nil {
						return nil, fmt.Errorf("unknown window %s", w)
					}
				}
				parsed = append(parsed, domain.SLOWindow{
					Name:       w,
					Duration:   dur,
					Compliance: w == "30d" || w == windows[len(windows)-1],
					BurnAlert:  burnFor(w),
				})
			}
			out = append(out, domain.SLODefinition{
				ID:          doc.Service + ":" + name,
				Service:     doc.Service,
				Name:        name,
				Objective:   objective,
				Indicator:   indicator,
				ThresholdMS: spec.ThresholdMS,
				Windows:     parsed,
				Description: spec.Description,
			})
		}
	}
	return out, nil
}

func burnFor(window string) float64 {
	switch window {
	case "1m", "5m":
		return 14.4
	case "30m", "1h":
		return 6
	default:
		return 1
	}
}

func readMap(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := yaml.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}

func deepMerge(base, overlay map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overlay {
		if bv, ok := out[k].(map[string]any); ok {
			if ov, ok := v.(map[string]any); ok {
				out[k] = deepMerge(bv, ov)
				continue
			}
		}
		out[k] = v
	}
	return out
}

// EnvBool parses a boolean environment variable.
func EnvBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}
