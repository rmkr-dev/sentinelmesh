// Package ai is a provider-agnostic interface for narrative assistance.
// The deterministic RCA engine remains authoritative. Providers may rephrase
// evidence; they may not invent telemetry or upgrade a correlation to a cause.
package ai

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

// Provider analyzes an evidence pack. Implementations must not be required
// for incident creation, SLO math, or alerting.
type Provider interface {
	Name() string
	Analyze(ctx context.Context, pack domain.EvidencePack) (domain.Analysis, error)
}

// ErrUnavailable is returned when the provider cannot be reached.
var ErrUnavailable = errors.New("ai provider unavailable")

// Merge keeps the deterministic evidence set and attaches model narrative
// only when every cited evidence ID exists. Confirmation is downgraded unless
// the deterministic analysis already confirmed it.
func Merge(base domain.Analysis, model domain.Analysis, provider string) domain.Analysis {
	out := base
	out.AIStatus = "completed"
	out.Deterministic = true
	known := map[string]domain.Evidence{}
	confirmed := false
	for _, e := range base.Evidence {
		known[e.ID] = e
		if e.Grade == domain.GradeConfirmed {
			confirmed = true
		}
	}
	if strings.TrimSpace(model.Summary) != "" {
		out.Summary = model.Summary
	}
	if strings.TrimSpace(model.Impact) != "" && !looksInvented(model.Impact) {
		out.Impact = model.Impact
	}
	for _, h := range model.Hypotheses {
		if len(h.EvidenceIDs) == 0 || !allKnown(h.EvidenceIDs, known) {
			continue
		}
		h.AIGenerated = true
		if h.Grade == domain.GradeConfirmed && !confirmed {
			h.Grade = domain.GradeStronglyCorrelated
			h.Statement = strings.TrimSpace(h.Statement + " Downgraded from confirmed because the evidence pack has no confirmed fact.")
		}
		h.ID = "ai-" + slug(h.ID)
		out.Hypotheses = append(out.Hypotheses, h)
	}
	if model.RollbackRecommended && !base.RollbackRecommended {
		// A model cannot introduce a rollback recommendation without a deployment hypothesis already present.
		if hasGrade(base.Hypotheses, domain.GradeStronglyCorrelated) {
			out.RollbackRecommended = true
		}
	}
	out.AIGenerated = false
	out.MissingEvidence = base.MissingEvidence
	for _, m := range model.MissingEvidence {
		if strings.TrimSpace(m) != "" {
			out.MissingEvidence = append(out.MissingEvidence, m)
		}
	}
	out.ConfidenceLabel = strongest(out.Hypotheses)
	if out.ConfidenceLabel == domain.GradeConfirmed && !confirmed {
		out.ConfidenceLabel = domain.GradeStronglyCorrelated
	}
	out.Confidence = score(out.ConfidenceLabel)
	out.GeneratedAt = time.Now().UTC()
	if provider != "" {
		if out.RecommendedActions == nil {
			out.RecommendedActions = []domain.RecommendedAction{}
		}
	}
	return out
}

// Degraded returns the deterministic analysis with the provider failure recorded.
func Degraded(base domain.Analysis, err error) domain.Analysis {
	out := base
	out.AIStatus = "degraded"
	out.AIGenerated = false
	out.Deterministic = true
	if err != nil {
		out.MissingEvidence = append(out.MissingEvidence, "AI provider unavailable: "+err.Error()+". Deterministic analysis is unchanged.")
	}
	return out
}

func allKnown(ids []string, known map[string]domain.Evidence) bool {
	for _, id := range ids {
		if _, ok := known[id]; !ok {
			return false
		}
	}
	return true
}

func looksInvented(impact string) bool {
	l := strings.ToLower(impact)
	for _, phrase := range []string{"all customers", "entire region", "data loss confirmed", "100% of users"} {
		if strings.Contains(l, phrase) {
			return true
		}
	}
	return false
}

func hasGrade(hs []domain.Hypothesis, grade string) bool {
	for _, h := range hs {
		if h.Grade == grade {
			return true
		}
	}
	return false
}

func strongest(hs []domain.Hypothesis) string {
	best := domain.GradeInsufficient
	for _, h := range hs {
		if score(h.Grade) > score(best) {
			best = h.Grade
		}
	}
	return best
}

func score(g string) float64 {
	switch g {
	case domain.GradeConfirmed:
		return 0.95
	case domain.GradeStronglyCorrelated:
		return 0.75
	case domain.GradeProbable:
		return 0.55
	case domain.GradePossible:
		return 0.35
	default:
		return 0.15
	}
}

func slug(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "hypothesis"
	}
	return strings.ReplaceAll(s, " ", "-")
}

// ValidateRequiredFields reports schema problems in a model response.
func ValidateRequiredFields(a domain.Analysis) error {
	if strings.TrimSpace(a.Summary) == "" {
		return fmt.Errorf("summary is required")
	}
	if a.Hypotheses == nil || a.Evidence == nil || a.RecommendedActions == nil || a.Timeline == nil || a.MissingEvidence == nil {
		return fmt.Errorf("arrays must be present, use empty arrays")
	}
	switch a.ConfidenceLabel {
	case domain.GradeConfirmed, domain.GradeStronglyCorrelated, domain.GradeProbable, domain.GradePossible, domain.GradeInsufficient:
	default:
		return fmt.Errorf("invalid confidence_label %q", a.ConfidenceLabel)
	}
	return nil
}
