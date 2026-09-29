package rca

import (
	"fmt"
	"sort"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

// Rank orders hypotheses without changing their grades.
// Downstream services, earlier failures, and nearer changes score higher.
func Rank(hypotheses []domain.Hypothesis, evidence []domain.Evidence) []domain.Hypothesis {
	gradeScore := map[string]float64{
		domain.GradeConfirmed:          50,
		domain.GradeStronglyCorrelated: 40,
		domain.GradeProbable:           25,
		domain.GradePossible:           10,
		domain.GradeInsufficient:       0,
	}
	out := append([]domain.Hypothesis{}, hypotheses...)
	for i := range out {
		score := gradeScore[out[i].Grade]
		var cited []string
		for _, id := range out[i].EvidenceIDs {
			for _, ev := range evidence {
				if ev.ID != id {
					continue
				}
				cited = append(cited, ev.Kind)
				switch ev.Kind {
				case "platform_health", "fault":
					score += 30
				case "kubernetes":
					score += 15
				case "deployment", "change":
					score += 10
				case "slo", "metric":
					score += 5
				}
			}
		}
		out[i].Score = score
		if out[i].Rationale == "" {
			out[i].Rationale = fmt.Sprintf("score %.0f from grade %s and evidence %v", score, out[i].Grade, cited)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}
