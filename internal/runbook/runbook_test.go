package runbook

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

func TestMatchAndActions(t *testing.T) {
	dir := t.TempDir()
	body := []byte(`
id: payment-service-high-error-rate
title: Payment error rate
service: payment-service
trigger:
  signal_types: [slo, alert]
  metric: http_server_error_rate
  threshold: 0.05
steps:
  - id: inspect_recent_deployment
    action: read
    description: Compare the last deployment timestamp with the symptom start.
  - id: evaluate_rollback
    action: recommend
    description: If the deployment correlation is strong, request approval to roll back.
    remediation: rollback_deployment
`)
	if err := os.WriteFile(filepath.Join(dir, "payment.yaml"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	books, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	inc := domain.Incident{
		Service: "payment-service",
		Signals: []domain.Signal{{Type: domain.SignalSLO, Service: "payment-service"}},
	}
	matched := Match(books, inc)
	if len(matched) != 1 {
		t.Fatalf("matched %d", len(matched))
	}
	actions := Actions(matched)
	if len(actions) != 2 {
		t.Fatalf("actions %d", len(actions))
	}
	if !actions[1].RequiresApproval || actions[1].Remediation != "rollback_deployment" {
		t.Fatalf("%+v", actions[1])
	}
	other := Match(books, domain.Incident{Service: "inventory-service"})
	if len(other) != 0 {
		t.Fatal("unexpected match")
	}
}
