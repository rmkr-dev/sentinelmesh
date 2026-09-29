package rca

import (
	"strings"
	"testing"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

func TestDeploymentCorrelationIsNotConfirmation(t *testing.T) {
	start := time.Date(2026, 9, 28, 10, 34, 17, 0, time.UTC)
	deployed := time.Date(2026, 9, 28, 10, 31, 22, 0, time.UTC)
	pack := Pack{
		Now: start.Add(5 * time.Minute),
		Incident: domain.Incident{
			ID:          "INC-2026-0001",
			Service:     "payment-service",
			Status:      domain.StatusInvestigating,
			StartedAt:   start,
			Environment: "production",
			Signals: []domain.Signal{
				{Type: domain.SignalAlert, Service: "payment-service", Summary: "error rate increased", OccurredAt: start},
				{Type: domain.SignalAnomaly, Service: "payment-service", Summary: "latency increased", OccurredAt: start.Add(45 * time.Second)},
			},
		},
		Deployments: []domain.Deployment{{
			ID:          "dep-1",
			Service:     "payment-service",
			Version:     "v1.8.2",
			GitSHA:      "abc1234def",
			Timestamp:   deployed,
			Environment: "production",
		}},
		SLO: []domain.SLOResult{{
			Service: "payment-service", SLO: "availability", Window: "5m",
			Target: 99.95, Current: 99.1, BurnRate: 18, ErrorBudgetRemaining: -200,
			Status: domain.SLOBreached, EvaluatedAt: start.Add(time.Minute),
		}},
	}
	got := Analyze(pack)
	if got.ConfidenceLabel == domain.GradeConfirmed {
		t.Fatal("deployment correlation must not be confirmed")
	}
	if got.ConfidenceLabel != domain.GradeStronglyCorrelated {
		t.Fatalf("label=%s", got.ConfidenceLabel)
	}
	if !got.RollbackRecommended {
		t.Fatal("expected rollback recommendation")
	}
	if !strings.Contains(got.Summary, "approximately 3 minutes") {
		t.Fatalf("summary missing delta: %s", got.Summary)
	}
	if !strings.Contains(strings.ToLower(got.Summary), "not a confirmed cause") {
		t.Fatalf("summary should refuse causation: %s", got.Summary)
	}
	if got.AIGenerated {
		t.Fatal("deterministic analysis must not be marked AI-generated")
	}
	var sawDeploy bool
	for _, h := range got.Hypotheses {
		if h.ID == "h-deployment" {
			sawDeploy = true
			if len(h.EvidenceIDs) == 0 {
				t.Fatal("hypothesis missing evidence")
			}
		}
	}
	if !sawDeploy {
		t.Fatal("missing deployment hypothesis")
	}
}

func TestControlPlaneFaultCanBeConfirmed(t *testing.T) {
	now := time.Date(2026, 9, 28, 11, 0, 0, 0, time.UTC)
	pack := Pack{
		Now: now,
		Incident: domain.Incident{
			ID: "INC-2026-0002", Service: "payment-service", Status: domain.StatusDetected, StartedAt: now,
		},
		Changes: []domain.Signal{{
			Type:       domain.SignalFault,
			Service:    "payment-service",
			Summary:    "Fault payment-latency enabled on payment-service",
			OccurredAt: now.Add(-time.Minute),
			Attributes: map[string]string{"observed": "true", "fault": "payment-latency"},
		}},
	}
	got := Analyze(pack)
	if got.ConfidenceLabel != domain.GradeConfirmed {
		t.Fatalf("label=%s", got.ConfidenceLabel)
	}
}

func TestInsufficientEvidence(t *testing.T) {
	got := Analyze(Pack{Incident: domain.Incident{ID: "INC-2026-0003", Service: "order-service", Status: domain.StatusDetected}})
	if got.ConfidenceLabel != domain.GradeInsufficient {
		t.Fatalf("label=%s summary=%s", got.ConfidenceLabel, got.Summary)
	}
	if len(got.MissingEvidence) == 0 {
		t.Fatal("expected missing evidence")
	}
}
