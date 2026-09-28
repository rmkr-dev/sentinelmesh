package ai

import (
	"context"
	"strings"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

// MockProvider rewrites the deterministic analysis in plain language.
// It does not add causes, traces, or numbers that are absent from the pack.
type MockProvider struct{}

func (MockProvider) Name() string { return "mock" }

func (MockProvider) Analyze(_ context.Context, pack domain.EvidencePack) (domain.Analysis, error) {
	base := domain.Analysis{}
	if pack.Analysis != nil {
		base = *pack.Analysis
	}
	label := base.ConfidenceLabel
	if label == "" {
		label = domain.GradeInsufficient
	}
	var b strings.Builder
	b.WriteString("AI-assisted summary (mock provider, not a model inference). ")
	b.WriteString("Grade: ")
	b.WriteString(strings.ReplaceAll(label, "_", " "))
	b.WriteString(". ")
	if base.Summary != "" {
		b.WriteString(base.Summary)
	} else {
		b.WriteString("No deterministic summary was available.")
	}
	if label != domain.GradeConfirmed {
		b.WriteString(" The assistant does not treat correlation as a confirmed cause.")
	}
	out := domain.Analysis{
		Summary:             b.String(),
		Impact:              base.Impact,
		Timeline:            nonNilTimeline(base.Timeline),
		Evidence:            nonNilEvidence(base.Evidence),
		Hypotheses:          cloneHypotheses(base.Hypotheses),
		Confidence:          base.Confidence,
		ConfidenceLabel:     label,
		RecommendedActions:  nonNilActions(base.RecommendedActions),
		RollbackRecommended: base.RollbackRecommended,
		MissingEvidence:     nonNilStrings(base.MissingEvidence),
		AIGenerated:         true,
		GeneratedAt:         time.Now().UTC(),
	}
	return out, nil
}

func cloneHypotheses(in []domain.Hypothesis) []domain.Hypothesis {
	if in == nil {
		return []domain.Hypothesis{}
	}
	out := make([]domain.Hypothesis, len(in))
	copy(out, in)
	return out
}

func nonNilTimeline(in []domain.TimelineEntry) []domain.TimelineEntry {
	if in == nil {
		return []domain.TimelineEntry{}
	}
	return in
}

func nonNilEvidence(in []domain.Evidence) []domain.Evidence {
	if in == nil {
		return []domain.Evidence{}
	}
	return in
}

func nonNilActions(in []domain.RecommendedAction) []domain.RecommendedAction {
	if in == nil {
		return []domain.RecommendedAction{}
	}
	return in
}

func nonNilStrings(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}
