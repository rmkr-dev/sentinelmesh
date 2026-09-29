// Package alerting normalizes, deduplicates, and suppresses alerts.
package alerting

import (
	"sort"
	"strings"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

// NormalizeFingerprint drops volatile labels so one problem stays one alert.
func NormalizeFingerprint(a domain.Alert) string {
	resourceID := ""
	if a.Labels != nil {
		resourceID = a.Labels["resource_id"]
	}
	parts := []string{a.Name, a.Service, resourceID}
	var extra []string
	for k, v := range a.Labels {
		switch k {
		case "pod", "instance", "replica", "resource_id", "service", "alertname", "severity":
			continue
		default:
			extra = append(extra, k+"="+v)
		}
	}
	sort.Strings(extra)
	return strings.Join(append(parts, extra...), "|")
}

// Flapping is true when the same fingerprint changed state at least n times in the window.
// Transitions recorded on the alert history count, because one fingerprint is one row.
func Flapping(history []domain.Alert, fingerprint string, n int, window time.Duration, now time.Time) bool {
	if n <= 0 {
		n = 3
	}
	var transitions []domain.AlertTransition
	var rows []domain.Alert
	for _, a := range history {
		if NormalizeFingerprint(a) != fingerprint && a.Fingerprint != fingerprint {
			continue
		}
		if len(a.History) > 0 {
			transitions = append(transitions, a.History...)
			continue
		}
		if now.Sub(a.StartsAt) > window && (a.EndsAt == nil || now.Sub(*a.EndsAt) > window) {
			continue
		}
		rows = append(rows, a)
	}
	if len(transitions) == 0 {
		sort.Slice(rows, func(i, j int) bool { return rows[i].StartsAt.Before(rows[j].StartsAt) })
		for _, a := range rows {
			at := a.StartsAt
			if a.Status == "resolved" && a.EndsAt != nil {
				at = *a.EndsAt
			}
			transitions = append(transitions, domain.AlertTransition{Status: a.Status, At: at})
		}
	}
	sort.Slice(transitions, func(i, j int) bool { return transitions[i].At.Before(transitions[j].At) })
	var changes int
	var prev string
	for _, t := range transitions {
		if !t.At.IsZero() && now.Sub(t.At) > window {
			continue
		}
		if prev != "" && prev != t.Status {
			changes++
		}
		prev = t.Status
	}
	return changes >= n
}

// Silenced reports whether any active silence matches the alert labels.
func Silenced(silences []domain.Silence, labels map[string]string, now time.Time) bool {
	for _, s := range silences {
		if now.Before(s.StartsAt) || !now.Before(s.EndsAt) {
			continue
		}
		if match(s.Matchers, labels) {
			return true
		}
	}
	return false
}

// InMaintenance reports whether a maintenance window matches.
func InMaintenance(windows []domain.MaintenanceWindow, labels map[string]string, now time.Time) bool {
	for _, w := range windows {
		if now.Before(w.StartsAt) || !now.Before(w.EndsAt) {
			continue
		}
		if match(w.Matchers, labels) {
			return true
		}
	}
	return false
}

func match(want, got map[string]string) bool {
	if len(want) == 0 {
		return false
	}
	for k, v := range want {
		if got[k] != v {
			return false
		}
	}
	return true
}
