package rca

import (
	"testing"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

func TestRankDoesNotUpgradeGrade(t *testing.T) {
	evidence := []domain.Evidence{
		{ID: "ev-1", Kind: "deployment", Grade: domain.GradeStronglyCorrelated},
		{ID: "ev-2", Kind: "platform_health", Grade: domain.GradeConfirmed},
	}
	in := []domain.Hypothesis{
		{ID: "h1", Grade: domain.GradeStronglyCorrelated, EvidenceIDs: []string{"ev-1"}, Statement: "deploy"},
		{ID: "h2", Grade: domain.GradeProbable, EvidenceIDs: []string{"ev-2"}, Statement: "health"},
	}
	out := Rank(in, evidence)
	if out[0].ID != "h2" {
		t.Fatalf("order %+v", out)
	}
	if out[0].Grade != domain.GradeProbable || out[1].Grade != domain.GradeStronglyCorrelated {
		t.Fatal("grade changed")
	}
	if out[0].Rationale == "" {
		t.Fatal("missing rationale")
	}
}
