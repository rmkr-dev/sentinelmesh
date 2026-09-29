// Package rca builds an evidence-based analysis without calling a model.
// Correlation is labeled as correlation. Confirmation requires a recorded fact
// such as a control-plane fault or a human verification, not a statistical match.
package rca

import (
	"fmt"
	"strings"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

// Pack is the deterministic input. The AI layer may receive the resulting analysis
// but is not required to produce it.
type Pack struct {
	Incident     domain.Incident
	Deployments  []domain.Deployment
	Changes      []domain.Signal
	SLO          []domain.SLOResult
	Dependencies []domain.DependencyEdge
	Logs         []domain.LogExcerpt
	Traces       []domain.TraceSample
	K8s          []domain.K8sEvent
	Now          time.Time
}

// Analyze returns a schema-stable analysis. Missing inputs are listed, not invented.
func Analyze(pack Pack) domain.Analysis {
	now := pack.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	a := domain.Analysis{
		Deterministic:   true,
		AIGenerated:     false,
		AIStatus:        "not_requested",
		GeneratedAt:     now.UTC(),
		Evidence:        []domain.Evidence{},
		Hypotheses:      []domain.Hypothesis{},
		MissingEvidence: []string{},
		Timeline:        append([]domain.TimelineEntry{}, pack.Incident.Events...),
	}

	var evidence []domain.Evidence
	next := 1
	add := func(e domain.Evidence) string {
		e.ID = fmt.Sprintf("ev-%d", next)
		next++
		evidence = append(evidence, e)
		return e.ID
	}

	start := pack.Incident.StartedAt
	if start.IsZero() {
		start = symptomTime(pack.Incident.Signals)
	}

	var deployEvidence []string
	var deploy domain.Deployment
	var deployDelta time.Duration
	foundDeploy := false
	for _, d := range pack.Deployments {
		if d.Service != pack.Incident.Service && !contains(pack.Incident.RelatedServices, d.Service) {
			continue
		}
		if start.IsZero() || d.Timestamp.After(start.Add(2*time.Minute)) {
			continue
		}
		if start.Sub(d.Timestamp) > 30*time.Minute {
			continue
		}
		if foundDeploy && d.Timestamp.Before(deploy.Timestamp) {
			continue
		}
		deploy = d
		deployDelta = start.Sub(d.Timestamp)
		foundDeploy = true
	}
	if foundDeploy {
		id := add(domain.Evidence{
			Kind:      "deployment",
			Source:    "deployment-registry",
			Grade:     domain.GradeStronglyCorrelated,
			Timestamp: deploy.Timestamp,
			Summary: fmt.Sprintf("Deployment %s:%s (%s) completed at %s, %s before the symptom start.",
				deploy.Service, deploy.Version, shortSHA(deploy.GitSHA), deploy.Timestamp.UTC().Format(time.RFC3339), humanDelta(deployDelta)),
			Attributes: map[string]string{
				"service":       deploy.Service,
				"version":       deploy.Version,
				"git_sha":       deploy.GitSHA,
				"deployment_id": deploy.ID,
			},
		})
		deployEvidence = append(deployEvidence, id)
	} else {
		a.MissingEvidence = append(a.MissingEvidence, "No deployment for the affected service in the 30 minutes before symptom start.")
	}

	var faultEvidence []string
	var faultSummary string
	for _, c := range pack.Changes {
		if c.Type != domain.SignalFault && c.Type != domain.SignalChange {
			continue
		}
		grade := domain.GradePossible
		if c.Attributes["observed"] == "true" {
			grade = domain.GradeConfirmed
		}
		id := add(domain.Evidence{
			Kind:       "change",
			Source:     "control-plane",
			Grade:      grade,
			Timestamp:  c.OccurredAt,
			Summary:    c.Summary,
			Attributes: c.Attributes,
		})
		if grade == domain.GradeConfirmed {
			faultEvidence = append(faultEvidence, id)
			faultSummary = c.Summary
		}
	}

	var sloEvidence []string
	for _, s := range pack.SLO {
		if s.Service != "" && s.Service != pack.Incident.Service && !contains(pack.Incident.RelatedServices, s.Service) {
			continue
		}
		if s.Status != domain.SLOBreached && s.Status != domain.SLOAtRisk {
			continue
		}
		id := add(domain.Evidence{
			Kind:      "slo",
			Source:    "slo-engine",
			Grade:     domain.GradeProbable,
			Timestamp: s.EvaluatedAt,
			Summary: fmt.Sprintf("%s %s window %s is %s: SLI %.4f%% vs target %.4f%%, burn rate %.2f, error budget remaining %.2f%%.",
				s.Service, s.SLO, s.Window, s.Status, s.Current, s.Target, s.BurnRate, s.ErrorBudgetRemaining),
			Attributes: map[string]string{"status": s.Status, "slo": s.SLO, "window": s.Window},
		})
		sloEvidence = append(sloEvidence, id)
	}
	if len(sloEvidence) == 0 {
		a.MissingEvidence = append(a.MissingEvidence, "No SLO window is currently at risk or breached for this service.")
	}

	var traceEvidence []string
	peerCounts := map[string]int{}
	for _, tr := range pack.Traces {
		if tr.Status != "error" && tr.DurationMS < 500 {
			continue
		}
		grade := domain.GradePossible
		if tr.Status == "error" {
			grade = domain.GradeProbable
		}
		id := add(domain.Evidence{
			Kind:      "trace",
			Source:    "tracing-backend",
			Grade:     grade,
			Timestamp: tr.StartedAt,
			Summary: fmt.Sprintf("Trace %s %s %s duration %.0fms peer %s %s",
				tr.TraceID, tr.Service, tr.Operation, tr.DurationMS, emptyAs(tr.Peer, "n/a"), tr.Error),
			Attributes: map[string]string{"trace_id": tr.TraceID, "peer": tr.Peer},
		})
		traceEvidence = append(traceEvidence, id)
		if tr.Peer != "" && tr.Status == "error" {
			peerCounts[tr.Peer]++
		}
	}
	if len(pack.Traces) == 0 {
		a.MissingEvidence = append(a.MissingEvidence, "No trace samples were available for the incident window.")
	}

	if len(pack.Logs) == 0 {
		a.MissingEvidence = append(a.MissingEvidence, "No log excerpts were available for the incident window.")
	} else {
		for i, lg := range pack.Logs {
			if i >= 5 {
				break
			}
			add(domain.Evidence{
				Kind:       "log",
				Source:     "log-backend",
				Grade:      domain.GradePossible,
				Timestamp:  lg.Timestamp,
				Summary:    fmt.Sprintf("%s %s: %s", lg.Service, lg.Severity, lg.Body),
				Attributes: map[string]string{"trace_id": lg.TraceID},
			})
		}
	}

	var k8sEvidence []string
	for _, ev := range pack.K8s {
		grade := domain.GradePossible
		statement := ""
		switch {
		case strings.Contains(ev.Reason, "OOM") || strings.Contains(ev.Message, "OOMKilled"):
			grade = domain.GradeProbable
			statement = "Container OOMKilled after memory growth."
		case strings.Contains(ev.Reason, "NodeNotReady") || strings.Contains(ev.Message, "NodeNotReady"):
			grade = domain.GradeProbable
			statement = "A node hosting affected pods is NotReady."
		case strings.Contains(ev.Reason, "ImagePull") || strings.Contains(ev.Message, "ImagePull"):
			grade = domain.GradeStronglyCorrelated
			statement = "Image pull failed after a rollout. This is a strong correlation with the rollout, not a confirmed application defect."
		}
		id := add(domain.Evidence{
			Kind:      "kubernetes",
			Source:    "kubernetes",
			Grade:     grade,
			Timestamp: ev.At,
			Summary:   fmt.Sprintf("%s %s/%s: %s", ev.Type, ev.Namespace, ev.Object, ev.Message),
		})
		if statement != "" {
			k8sEvidence = append(k8sEvidence, id)
			a.Hypotheses = append(a.Hypotheses, domain.Hypothesis{
				ID:    "h-k8s-" + slug(ev.Reason) + "-" + slug(ev.Namespace) + "-" + slug(ev.Object),
				Grade: grade, EvidenceIDs: []string{id}, Statement: statement,
			})
		}
	}
	_ = k8sEvidence

	for _, sig := range pack.Incident.Signals {
		add(domain.Evidence{
			Kind:       sig.Type,
			Source:     "signal",
			Grade:      domain.GradePossible,
			Timestamp:  sig.OccurredAt,
			Summary:    sig.Summary,
			Attributes: sig.Attributes,
		})
	}

	if len(faultEvidence) > 0 {
		a.Hypotheses = append(a.Hypotheses, domain.Hypothesis{
			ID:          "h-control-plane",
			Grade:       domain.GradeConfirmed,
			EvidenceIDs: faultEvidence,
			Statement:   "Confirmed by a control-plane record, not by inference: " + faultSummary,
		})
	}

	if foundDeploy {
		a.Hypotheses = append(a.Hypotheses, domain.Hypothesis{
			ID:          "h-deployment",
			Grade:       domain.GradeStronglyCorrelated,
			EvidenceIDs: deployEvidence,
			Statement: fmt.Sprintf("Strong correlation detected. The symptom start is %s after deployment %s:%s. This is a correlation, not a confirmed cause.",
				humanDelta(deployDelta), deploy.Service, deploy.Version),
		})
		a.RollbackRecommended = true
		a.RecommendedActions = append(a.RecommendedActions, domain.RecommendedAction{
			ID:               "rollback",
			Title:            "Evaluate rollback of " + deploy.Service,
			Detail:           fmt.Sprintf("Compare %s with the previous version. Rollback requires human approval and is not executed by this analysis.", deploy.Version),
			Remediation:      "rollback_deployment",
			RequiresApproval: true,
		})
	}

	var depPeer string
	depN := 0
	for peer, n := range peerCounts {
		if n > depN {
			depPeer, depN = peer, n
		}
	}
	if depPeer != "" && depN >= 1 {
		ids := traceEvidence
		grade := domain.GradeProbable
		if depN < 2 && !foundDeploy {
			grade = domain.GradePossible
		}
		a.Hypotheses = append(a.Hypotheses, domain.Hypothesis{
			ID:          "h-dependency",
			Grade:       grade,
			EvidenceIDs: ids,
			Statement:   fmt.Sprintf("Failing or slow calls to dependency %q appear in sampled traces. Treat this as %s until more traces corroborate it.", depPeer, strings.ReplaceAll(grade, "_", " ")),
		})
		a.RecommendedActions = append(a.RecommendedActions, domain.RecommendedAction{
			ID:     "inspect-dependency",
			Title:  "Inspect dependency " + depPeer,
			Detail: "Open the cited traces and the dependency's own error and latency SLIs before changing the caller.",
		})
	}

	for _, edge := range pack.Dependencies {
		if edge.Status == "unhealthy" || edge.Status == "down" {
			id := add(domain.Evidence{
				Kind:    "dependency",
				Source:  "service-catalog",
				Grade:   domain.GradeProbable,
				Summary: fmt.Sprintf("Dependency %s → %s is %s. %s", edge.From, edge.To, edge.Status, edge.Detail),
			})
			a.Hypotheses = append(a.Hypotheses, domain.Hypothesis{
				ID:          "h-dep-" + edge.To,
				Grade:       domain.GradeProbable,
				EvidenceIDs: []string{id},
				Statement:   fmt.Sprintf("Dependency %s is reported %s. This supports a dependency-related hypothesis and does not by itself confirm the root cause.", edge.To, edge.Status),
			})
		}
	}

	if len(a.Hypotheses) == 0 {
		a.Hypotheses = append(a.Hypotheses, domain.Hypothesis{
			ID:          "h-insufficient",
			Grade:       domain.GradeInsufficient,
			EvidenceIDs: evidenceIDs(evidence),
			Statement:   "Insufficient evidence to propose a cause. Collect traces, logs, deployment history, and dependency health before acting.",
		})
	}

	a.Evidence = evidence
	a.ConfidenceLabel = strongest(a.Hypotheses)
	a.Confidence = gradeScore(a.ConfidenceLabel)
	a.Summary = summary(pack, a, foundDeploy, deploy, deployDelta)
	a.Impact = impact(pack)
	if len(a.RecommendedActions) == 0 {
		a.RecommendedActions = append(a.RecommendedActions, domain.RecommendedAction{
			ID:     "investigate",
			Title:  "Continue investigation from the timeline",
			Detail: "Review the cited evidence and fill the missing-evidence list before changing production.",
		})
	}
	a.Hypotheses = Rank(a.Hypotheses, a.Evidence)
	return a
}

func summary(pack Pack, a domain.Analysis, found bool, d domain.Deployment, delta time.Duration) string {
	base := fmt.Sprintf("Incident %s on %s is %s.", pack.Incident.ID, pack.Incident.Service, pack.Incident.Status)
	if found {
		return fmt.Sprintf("%s Strong correlation detected. Symptoms began %s after deployment %s:%s. This is a correlation, not a confirmed cause. Leading grade: %s.",
			base, humanDelta(delta), d.Service, d.Version, strings.ReplaceAll(a.ConfidenceLabel, "_", " "))
	}
	return fmt.Sprintf("%s Leading grade: %s. %s", base, strings.ReplaceAll(a.ConfidenceLabel, "_", " "), a.Hypotheses[0].Statement)
}

func impact(pack Pack) string {
	if pack.Incident.Impact != "" {
		return pack.Incident.Impact
	}
	services := append([]string{pack.Incident.Service}, pack.Incident.RelatedServices...)
	return fmt.Sprintf("Symptoms involve %s. Customer impact is not recorded; do not infer user counts from telemetry volume.", strings.Join(unique(services), ", "))
}

func strongest(hs []domain.Hypothesis) string {
	best := domain.GradeInsufficient
	bestScore := gradeScore(best)
	for _, h := range hs {
		if s := gradeScore(h.Grade); s > bestScore {
			best, bestScore = h.Grade, s
		}
	}
	return best
}

func gradeScore(g string) float64 {
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

func slug(v string) string {
	v = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		default:
			return '-'
		}
	}, v)
	return strings.Trim(v, "-")
}

func symptomTime(signals []domain.Signal) time.Time {
	var t time.Time
	for _, s := range signals {
		if s.Type == domain.SignalDeployment || s.Type == domain.SignalFault {
			continue
		}
		if t.IsZero() || s.OccurredAt.Before(t) {
			t = s.OccurredAt
		}
	}
	return t
}

func humanDelta(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	if d < time.Minute {
		return fmt.Sprintf("%d seconds", int(d.Seconds()))
	}
	mins := int(d.Round(time.Minute) / time.Minute)
	if mins == 1 {
		return "approximately 1 minute"
	}
	return fmt.Sprintf("approximately %d minutes", mins)
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	if sha == "" {
		return "unknown-sha"
	}
	return sha
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

func unique(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func evidenceIDs(ev []domain.Evidence) []string {
	ids := make([]string, 0, len(ev))
	for _, e := range ev {
		ids = append(ids, e.ID)
	}
	return ids
}

func emptyAs(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
