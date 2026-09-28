package postmortem

import (
	"strings"
	"testing"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

func TestDoesNotFabricateCause(t *testing.T) {
	md := Render(domain.Incident{
		ID: "INC-2026-0001", Service: "payment-service", Severity: "SEV2", Status: "investigating",
		Environment: "local",
		Analysis: &domain.Analysis{
			Summary:         "Strong correlation detected.",
			ConfidenceLabel: domain.GradeStronglyCorrelated,
			Hypotheses: []domain.Hypothesis{{
				Grade: domain.GradeStronglyCorrelated, Statement: "Symptoms followed deployment v1.8.2.",
			}},
			MissingEvidence: []string{"No log excerpts were available."},
		},
	})
	if strings.Contains(md, "root cause is") && !strings.Contains(md, "No confirmed root cause") {
		t.Fatal(md)
	}
	if !strings.Contains(md, "No confirmed root cause") {
		t.Fatal(md)
	}
	if !strings.Contains(md, "_Not recorded._") {
		t.Fatal("expected explicit gaps")
	}
	if !strings.Contains(md, "What Went Well") {
		t.Fatal(md)
	}
}

func TestTimelineRendered(t *testing.T) {
	md := Render(domain.Incident{
		ID: "INC-2026-0002", Service: "order-service", Status: "resolved",
		Events: []domain.TimelineEntry{{
			At: time.Date(2026, 9, 28, 10, 34, 0, 0, time.UTC), Kind: "alert", Message: "error rate increased",
		}},
		ResolvedAt: ptr(time.Date(2026, 9, 28, 10, 47, 0, 0, time.UTC)),
	})
	if !strings.Contains(md, "error rate increased") {
		t.Fatal(md)
	}
	if !strings.Contains(md, "Marked resolved") {
		t.Fatal(md)
	}
}

func ptr(t time.Time) *time.Time { return &t }
