// Package incident implements the incident lifecycle state machine.
package incident

import (
	"fmt"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

var allowed = map[string]map[string]bool{
	domain.StatusDetected: {
		domain.StatusTriaged:       true,
		domain.StatusInvestigating: true,
		domain.StatusMitigating:    true,
		domain.StatusResolved:      true,
	},
	domain.StatusTriaged: {
		domain.StatusInvestigating: true,
		domain.StatusMitigating:    true,
		domain.StatusResolved:      true,
	},
	domain.StatusInvestigating: {
		domain.StatusMitigating: true,
		domain.StatusMonitoring: true,
		domain.StatusResolved:   true,
	},
	domain.StatusMitigating: {
		domain.StatusMonitoring:    true,
		domain.StatusInvestigating: true,
		domain.StatusResolved:      true,
	},
	domain.StatusMonitoring: {
		domain.StatusResolved:      true,
		domain.StatusInvestigating: true,
		domain.StatusMitigating:    true,
	},
	domain.StatusResolved: {
		domain.StatusClosed:        true,
		domain.StatusInvestigating: true,
	},
	domain.StatusClosed: {},
}

// Transition validates a status change. The caller records the timeline event.
func Transition(from, to string) error {
	if from == to {
		return fmt.Errorf("incident is already %s", from)
	}
	next, ok := allowed[from]
	if !ok {
		return fmt.Errorf("unknown status %q", from)
	}
	if !next[to] {
		return fmt.Errorf("cannot transition from %s to %s", from, to)
	}
	return nil
}

// Open reports whether the incident still needs attention.
func Open(status string) bool {
	switch status {
	case domain.StatusResolved, domain.StatusClosed:
		return false
	default:
		return true
	}
}
