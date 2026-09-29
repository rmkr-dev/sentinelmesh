package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

func TestMergeDropsInventedEvidenceAndConfirmation(t *testing.T) {
	base := domain.Analysis{
		Summary:         "deterministic",
		ConfidenceLabel: domain.GradeStronglyCorrelated,
		Evidence: []domain.Evidence{{
			ID: "ev-1", Grade: domain.GradeStronglyCorrelated, Summary: "deploy",
		}},
		Hypotheses: []domain.Hypothesis{{
			ID: "h-deployment", Grade: domain.GradeStronglyCorrelated, EvidenceIDs: []string{"ev-1"},
			Statement: "correlation",
		}},
		RecommendedActions: []domain.RecommendedAction{},
		MissingEvidence:    []string{},
		Timeline:           []domain.TimelineEntry{},
	}
	model := domain.Analysis{
		Summary:         "The model agrees there is a correlation.",
		Impact:          "All customers in the entire region are down",
		ConfidenceLabel: domain.GradeConfirmed,
		Evidence:        []domain.Evidence{},
		Hypotheses: []domain.Hypothesis{
			{ID: "guess", Grade: domain.GradeConfirmed, EvidenceIDs: []string{"ev-999"}, Statement: "invented"},
			{ID: "ok", Grade: domain.GradeConfirmed, EvidenceIDs: []string{"ev-1"}, Statement: "restate"},
		},
		RecommendedActions:  []domain.RecommendedAction{},
		MissingEvidence:     []string{},
		Timeline:            []domain.TimelineEntry{},
		RollbackRecommended: true,
	}
	got := Merge(base, model, "mock")
	if got.ConfidenceLabel == domain.GradeConfirmed {
		t.Fatal("confirmation must be downgraded")
	}
	if strings.Contains(got.Impact, "entire region") {
		t.Fatal("invented impact should be ignored")
	}
	for _, h := range got.Hypotheses {
		if strings.Contains(h.ID, "guess") {
			t.Fatal("unknown evidence hypothesis kept")
		}
	}
	var saw bool
	for _, h := range got.Hypotheses {
		if strings.Contains(h.ID, "ok") {
			saw = true
			if !h.AIGenerated || h.Grade == domain.GradeConfirmed {
				t.Fatalf("%+v", h)
			}
		}
	}
	if !saw {
		t.Fatal("expected merged hypothesis")
	}
}

func TestMockDoesNotInvent(t *testing.T) {
	base := domain.Analysis{
		Summary: "Strong correlation detected.", ConfidenceLabel: domain.GradeStronglyCorrelated,
		Evidence: []domain.Evidence{}, Hypotheses: []domain.Hypothesis{},
		RecommendedActions: []domain.RecommendedAction{}, MissingEvidence: []string{"traces"},
		Timeline: []domain.TimelineEntry{},
	}
	got, err := MockProvider{}.Analyze(context.Background(), domain.EvidencePack{Analysis: &base})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateRequiredFields(got); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Summary, "Strong correlation detected.") {
		t.Fatal(got.Summary)
	}
}

func TestChatProviderParsesSchema(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Fatal("missing auth")
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		content := `{"summary":"restated","impact":"checkout errors observed","timeline":[],"evidence":[],"hypotheses":[{"id":"h","statement":"correlation","grade":"strongly_correlated","evidence_ids":["ev-1"],"ai_generated":true}],"confidence":0.75,"confidence_label":"strongly_correlated","recommended_actions":[],"rollback_recommended":false,"missing_evidence":[]}`
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": content}}},
		})
	}))
	defer srv.Close()
	p := NewChatProvider(ChatConfig{BaseURL: srv.URL, APIKey: "test", Model: "test-model"})
	got, err := p.Analyze(context.Background(), domain.EvidencePack{})
	if err != nil {
		t.Fatal(err)
	}
	if got.ConfidenceLabel != domain.GradeStronglyCorrelated {
		t.Fatal(got.ConfidenceLabel)
	}
}

func TestChatProviderUnavailable(t *testing.T) {
	p := NewChatProvider(ChatConfig{BaseURL: "http://127.0.0.1:1", Model: "m", HTTP: &http.Client{Timeout: time.Millisecond * 200}})
	_, err := p.Analyze(context.Background(), domain.EvidencePack{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDegradedKeepsDeterministicSummary(t *testing.T) {
	base := domain.Analysis{Summary: "kept", ConfidenceLabel: domain.GradePossible, Hypotheses: []domain.Hypothesis{{Grade: domain.GradePossible}}}
	got := Degraded(base, ErrUnavailable)
	if got.AIStatus != "degraded" || got.Summary != "kept" {
		t.Fatalf("%+v", got)
	}
}
