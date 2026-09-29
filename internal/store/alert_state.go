package store

import (
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

// mergeAlert keeps one row per fingerprint and appends a transition when status changes.
func mergeAlert(prev *domain.Alert, next domain.Alert) domain.Alert {
	if prev == nil {
		if len(next.History) == 0 && next.Status != "" {
			at := next.StartsAt
			if at.IsZero() {
				at = time.Now().UTC()
			}
			next.History = []domain.AlertTransition{{Status: next.Status, At: at}}
		}
		return next
	}
	next.History = append([]domain.AlertTransition{}, prev.History...)
	if prev.Status != next.Status {
		at := time.Now().UTC()
		if next.Status == "resolved" && next.EndsAt != nil && !next.EndsAt.IsZero() {
			at = next.EndsAt.UTC()
		}
		next.History = append(next.History, domain.AlertTransition{Status: next.Status, At: at})
	}
	if prev.ID != "" {
		next.ID = prev.ID
	}
	if next.Fingerprint == "" {
		next.Fingerprint = prev.Fingerprint
	}
	if next.StartsAt.IsZero() {
		next.StartsAt = prev.StartsAt
	}
	return next
}

func alertActive(a domain.Alert, since time.Time) bool {
	if a.Status == "firing" {
		return true
	}
	if a.Status != "resolved" {
		return false
	}
	at := a.StartsAt
	if a.EndsAt != nil {
		at = *a.EndsAt
	}
	return since.IsZero() || !at.Before(since)
}
