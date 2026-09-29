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
	if a.Fingerprint != "" && !strings.Contains(a.Fingerprint, "pod") {
		return a.Name + "|" + a.Service
	}
	return a.Name + "|" + a.Service
}

// Flapping is true when the same fingerprint changed state at least n times in the window.
func Flapping(history []domain.Alert, fingerprint string, n int, window time.Duration, now time.Time) bool {
	if n <= 0 {
		n = 3
	}
	var changes int
	var prev string
	var rows []domain.Alert
	for _, a := range history {
		if NormalizeFingerprint(a) != fingerprint {
			continue
		}
		if now.Sub(a.StartsAt) > window && (a.EndsAt == nil || now.Sub(*a.EndsAt) > window) {
			continue
		}
		rows = append(rows, a)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].StartsAt.Before(rows[j].StartsAt) })
	for _, a := range rows {
		if prev != "" && prev != a.Status {
			changes++
		}
		prev = a.Status
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
		if got[k] != v && got["service"] != v && got["alertname"] != v {
			return false
		}
	}
	return true
}
