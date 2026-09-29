package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOverlayAndSLOLoad(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "environments"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "slo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "base.yaml"), []byte("http:\n  addr: \":8080\"\nai:\n  provider: mock\n  temperature: 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "environments", "local.yaml"), []byte("ai:\n  provider: mock\ndemo:\n  enabled: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir, "local")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTP.Addr != ":8080" || !cfg.Demo.Enabled || cfg.AI.Provider != "mock" {
		t.Fatalf("%+v", cfg)
	}
	sloBody := []byte("service: payment-service\nslos:\n  availability:\n    target: 99.95%\n  latency:\n    target: 99%\n    threshold_ms: 500\n    windows: [1m, 5m]\n")
	if err := os.WriteFile(filepath.Join(dir, "slo", "payment.yaml"), sloBody, 0o644); err != nil {
		t.Fatal(err)
	}
	defs, err := LoadSLOs(filepath.Join(dir, "slo"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(defs) != 2 {
		t.Fatalf("defs=%d", len(defs))
	}
	var latency bool
	for _, d := range defs {
		if d.Name == "latency" {
			latency = true
			if d.ThresholdMS != 500 || d.Objective != 99 {
				t.Fatalf("%+v", d)
			}
			if d.Windows[0].Duration != time.Minute {
				t.Fatal(d.Windows[0].Duration)
			}
		}
	}
	if !latency {
		t.Fatal("missing latency")
	}
}
