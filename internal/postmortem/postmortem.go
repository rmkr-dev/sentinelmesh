// Package postmortem renders a markdown postmortem from recorded incident data.
// Fields that were never recorded are marked explicitly and are not invented.
package postmortem

import (
	"fmt"
	"strings"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

const notRecorded = "_Not recorded._"

// Render builds the postmortem markdown.
func Render(inc domain.Incident) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Incident Postmortem\n\n")
	fmt.Fprintf(&b, "- Incident: `%s`\n", inc.ID)
	fmt.Fprintf(&b, "- Service: `%s`\n", inc.Service)
	fmt.Fprintf(&b, "- Severity: `%s`\n", or(inc.Severity, notRecorded))
	fmt.Fprintf(&b, "- Status: `%s`\n", or(inc.Status, notRecorded))
	fmt.Fprintf(&b, "- Environment: `%s`\n\n", or(inc.Environment, notRecorded))

	section(&b, "Summary", textOr(inc.Summary, analysisSummary(inc)))
	section(&b, "Impact", textOr(inc.Impact, analysisImpact(inc)))
	section(&b, "Detection", detection(inc))
	fmt.Fprintf(&b, "## Timeline\n\n")
	if len(inc.Events) == 0 {
		fmt.Fprintf(&b, "%s\n\n", notRecorded)
	} else {
		for _, ev := range inc.Events {
			fmt.Fprintf(&b, "- %s %s — %s\n", ev.At.UTC().Format(time.RFC3339), ev.Kind, ev.Message)
		}
		b.WriteString("\n")
	}
	section(&b, "Root Cause", rootCause(inc))
	section(&b, "Contributing Factors", factors(inc))
	fmt.Fprintf(&b, "## Evidence\n\n")
	ev := inc.Evidence
	if inc.Analysis != nil && len(inc.Analysis.Evidence) > 0 {
		ev = inc.Analysis.Evidence
	}
	if len(ev) == 0 {
		fmt.Fprintf(&b, "%s\n\n", notRecorded)
	} else {
		for _, e := range ev {
			fmt.Fprintf(&b, "- `%s` (%s, %s) %s\n", e.ID, e.Kind, e.Grade, e.Summary)
		}
		b.WriteString("\n")
	}
	section(&b, "Resolution", resolution(inc))
	section(&b, "Recovery", recovery(inc))
	section(&b, "What Went Well", notRecorded)
	section(&b, "What Went Poorly", poor(inc))
	fmt.Fprintf(&b, "## Corrective Actions\n\n%s\n\n", actions(inc))
	section(&b, "Preventive Actions", notRecorded)
	section(&b, "SLO Impact", sloImpact(inc))
	if inc.HumanNotes != "" {
		section(&b, "Human Notes", inc.HumanNotes)
	} else {
		section(&b, "Human Notes", notRecorded)
	}
	fmt.Fprintf(&b, "## Generation\n\nThis postmortem was rendered from the incident record. AI narrative, when present, is labeled in the analysis and is not treated as a confirmed cause.\n")
	return b.String()
}

func section(b *strings.Builder, title, body string) {
	fmt.Fprintf(b, "## %s\n\n%s\n\n", title, strings.TrimSpace(body))
}

func detection(inc domain.Incident) string {
	if inc.DetectedAt.IsZero() {
		return notRecorded
	}
	return fmt.Sprintf("Detected at %s. Started at %s.", inc.DetectedAt.UTC().Format(time.RFC3339), formatTime(inc.StartedAt))
}

func rootCause(inc domain.Incident) string {
	hypotheses := inc.SuspectedCauses
	if inc.Analysis != nil && len(inc.Analysis.Hypotheses) > 0 {
		hypotheses = inc.Analysis.Hypotheses
	}
	if len(hypotheses) == 0 {
		return notRecorded
	}
	top := hypotheses[0]
	if top.Grade != domain.GradeConfirmed {
		return fmt.Sprintf("No confirmed root cause is recorded. Leading hypothesis is graded **%s**:\n\n> %s\n\nThis grade is not a confirmation.", strings.ReplaceAll(top.Grade, "_", " "), top.Statement)
	}
	return fmt.Sprintf("Confirmed cause recorded:\n\n> %s", top.Statement)
}

func factors(inc domain.Incident) string {
	hypotheses := inc.SuspectedCauses
	if inc.Analysis != nil {
		hypotheses = inc.Analysis.Hypotheses
	}
	if len(hypotheses) < 2 {
		return notRecorded
	}
	var lines []string
	for _, h := range hypotheses[1:] {
		lines = append(lines, fmt.Sprintf("- (%s) %s", h.Grade, h.Statement))
	}
	return strings.Join(lines, "\n")
}

func resolution(inc domain.Incident) string {
	if inc.Status != domain.StatusResolved && inc.Status != domain.StatusClosed {
		return notRecorded
	}
	if inc.ResolvedAt == nil {
		return "Status is " + inc.Status + " but the resolution timestamp is not recorded."
	}
	return "Marked " + inc.Status + " at " + inc.ResolvedAt.UTC().Format(time.RFC3339) + "."
}

func recovery(inc domain.Incident) string {
	for _, ev := range inc.Events {
		if ev.Kind == "recovery" || ev.Kind == "resolved" {
			return ev.Message
		}
	}
	return notRecorded
}

func poor(inc domain.Incident) string {
	if inc.Analysis != nil && len(inc.Analysis.MissingEvidence) > 0 {
		return "Evidence gaps recorded during analysis:\n\n- " + strings.Join(inc.Analysis.MissingEvidence, "\n- ")
	}
	return notRecorded
}

func actions(inc domain.Incident) string {
	actions := inc.RecommendedActions
	if inc.Analysis != nil && len(inc.Analysis.RecommendedActions) > 0 {
		actions = inc.Analysis.RecommendedActions
	}
	if len(actions) == 0 {
		return notRecorded
	}
	var lines []string
	for _, a := range actions {
		approval := ""
		if a.RequiresApproval {
			approval = " (requires approval)"
		}
		lines = append(lines, fmt.Sprintf("- %s%s — %s", a.Title, approval, a.Detail))
	}
	return strings.Join(lines, "\n")
}

func sloImpact(inc domain.Incident) string {
	if inc.Analysis == nil {
		return notRecorded
	}
	var lines []string
	for _, e := range inc.Analysis.Evidence {
		if e.Kind == "slo" {
			lines = append(lines, "- "+e.Summary)
		}
	}
	if len(lines) == 0 {
		return notRecorded
	}
	return strings.Join(lines, "\n")
}

func analysisSummary(inc domain.Incident) string {
	if inc.Analysis != nil && inc.Analysis.Summary != "" {
		return inc.Analysis.Summary
	}
	return ""
}

func analysisImpact(inc domain.Incident) string {
	if inc.Analysis != nil && inc.Analysis.Impact != "" {
		return inc.Analysis.Impact
	}
	return ""
}

func textOr(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return primary
	}
	if strings.TrimSpace(fallback) != "" {
		return fallback
	}
	return notRecorded
}

func or(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "not recorded"
	}
	return t.UTC().Format(time.RFC3339)
}
