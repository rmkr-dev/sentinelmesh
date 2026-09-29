package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func isolateConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"DATABASE_URL", "STORE", "PLATFORM_API_TOKEN", "PLATFORM_WEBHOOK_TOKEN",
		"AI_ENABLED", "AI_PROVIDER", "DEMO_ENABLED", "AUTH_MODE",
	} {
		t.Setenv(key, "")
	}
}

func TestProductionRejectsDemoAndMemory(t *testing.T) {
	isolateConfigEnv(t)
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "environments"), 0o755); err != nil {
		t.Fatal(err)
	}
	base := "store: memory\nai:\n  enabled: true\n  provider: \"\"\nremediation:\n  executor: demo\ndemo:\n  enabled: true\n"
	if err := os.WriteFile(filepath.Join(dir, "base.yaml"), []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "environments", "production.yaml"), []byte("store: memory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir, "production")
	if err == nil || !strings.Contains(err.Error(), "store must be postgres") {
		t.Fatal(err)
	}
}

func TestProductionAcceptsRealSettings(t *testing.T) {
	isolateConfigEnv(t)
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "environments"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "base.yaml"), []byte("store: postgres\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	overlay := "demo:\n  enabled: false\nai:\n  enabled: true\n  provider: azure-openai\nremediation:\n  executor: kubernetes\nauth:\n  mode: token\n  token: from-env\n  webhook_token: hook\n"
	if err := os.WriteFile(filepath.Join(dir, "environments", "production.yaml"), []byte(overlay), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir, "production")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Store != "postgres" || cfg.Demo.Enabled || cfg.Remediation.Executor != "kubernetes" {
		t.Fatalf("%+v", cfg)
	}
}

func TestResolveDirPrecedence(t *testing.T) {
	root := t.TempDir()
	configured := filepath.Join(root, "configured")
	fallback := filepath.Join(root, "fallback")
	if err := os.Mkdir(configured, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(fallback, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RUNBOOK_DIR_TEST", "")
	got, err := ResolveDir("RUNBOOK_DIR_TEST", configured, fallback)
	if err != nil || got != configured {
		t.Fatalf("%s %v", got, err)
	}
	envDir := filepath.Join(root, "env")
	if err := os.Mkdir(envDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RUNBOOK_DIR_TEST", envDir)
	got, err = ResolveDir("RUNBOOK_DIR_TEST", configured, fallback)
	if err != nil || got != envDir {
		t.Fatalf("%s %v", got, err)
	}
	t.Setenv("RUNBOOK_DIR_TEST", filepath.Join(root, "missing"))
	if _, err := ResolveDir("RUNBOOK_DIR_TEST", configured, fallback); err == nil {
		t.Fatal("missing directory")
	}
}

func TestExamplesLoad(t *testing.T) {
	root := filepath.Join("..", "..")
	defs, err := LoadSLOs(filepath.Join(root, "examples", "custom-slo"), nil)
	if err != nil || len(defs) == 0 {
		t.Fatalf("%v %v", defs, err)
	}
}
