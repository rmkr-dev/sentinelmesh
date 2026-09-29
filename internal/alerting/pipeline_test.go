package alerting

import (
	"testing"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

func TestFlapAndSilence(t *testing.T) {
	now := time.Now().UTC()
	history := []domain.Alert{
		{Name: "HighErrorRate", Service: "payment-service", Status: "firing", StartsAt: now.Add(-4 * time.Minute)},
		{Name: "HighErrorRate", Service: "payment-service", Status: "resolved", StartsAt: now.Add(-3 * time.Minute)},
		{Name: "HighErrorRate", Service: "payment-service", Status: "firing", StartsAt: now.Add(-2 * time.Minute)},
		{Name: "HighErrorRate", Service: "payment-service", Status: "resolved", StartsAt: now.Add(-time.Minute)},
	}
	if !Flapping(history, "HighErrorRate|payment-service|", 3, 10*time.Minute, now) {
		t.Fatal("expected flap")
	}
	silences := []domain.Silence{{
		Owner: "sre", Matchers: map[string]string{"service": "payment-service"},
		StartsAt: now.Add(-time.Minute), EndsAt: now.Add(time.Hour),
	}}
	if !Silenced(silences, map[string]string{"service": "payment-service"}, now) {
		t.Fatal("expected silence")
	}
}
