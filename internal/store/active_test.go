package store

import (
	"context"
	"testing"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

func TestActiveAlertsKeepLongRunningFiring(t *testing.T) {
	ctx := context.Background()
	st := NewMemory()
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	start := now.Add(-6 * time.Minute)
	if err := st.SaveAlert(ctx, domain.Alert{
		ID: "alrt-1", Fingerprint: "fp", Name: "OrderErrors", Service: "orders", Status: "firing", StartsAt: start,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := st.ListActiveAlerts(ctx, now.Add(-5*time.Minute))
	if err != nil || len(got) != 1 || got[0].Status != "firing" {
		t.Fatalf("%+v %v", got, err)
	}
	ended := now.Add(-time.Minute)
	if err := st.SaveAlert(ctx, domain.Alert{
		ID: "alrt-1", Fingerprint: "fp", Name: "OrderErrors", Service: "orders", Status: "resolved", StartsAt: start, EndsAt: &ended,
	}); err != nil {
		t.Fatal(err)
	}
	got, err = st.ListAlerts(ctx, time.Time{})
	if err != nil || len(got) != 1 || got[0].Status != "resolved" || len(got[0].History) < 2 {
		t.Fatalf("%+v %v", got, err)
	}
	oldEnd := now.Add(-10 * time.Minute)
	if err := st.SaveAlert(ctx, domain.Alert{
		ID: "alrt-old", Fingerprint: "old", Name: "Old", Status: "resolved", StartsAt: now.Add(-time.Hour), EndsAt: &oldEnd,
	}); err != nil {
		t.Fatal(err)
	}
	got, err = st.ListActiveAlerts(ctx, now.Add(-5*time.Minute))
	if err != nil || len(got) != 1 {
		t.Fatalf("active=%+v %v", got, err)
	}
}
