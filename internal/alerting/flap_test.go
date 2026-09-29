package alerting

import (
	"testing"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

func TestFourTransitionsSuppress(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	a := domain.Alert{
		Name: "Flap", Service: "payment-service", Fingerprint: "fp", Status: "firing",
		History: []domain.AlertTransition{
			{Status: "firing", At: now.Add(-25 * time.Minute)},
			{Status: "resolved", At: now.Add(-20 * time.Minute)},
			{Status: "firing", At: now.Add(-15 * time.Minute)},
			{Status: "resolved", At: now.Add(-10 * time.Minute)},
			{Status: "firing", At: now.Add(-5 * time.Minute)},
		},
	}
	if !Flapping([]domain.Alert{a}, NormalizeFingerprint(a), 4, 30*time.Minute, now) {
		t.Fatal("expected suppression")
	}
	a.History = a.History[:2]
	if Flapping([]domain.Alert{a}, NormalizeFingerprint(a), 4, 30*time.Minute, now) {
		t.Fatal("two states are not four transitions")
	}
}
