//go:build integration

package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

func TestAdvisoryLockSingleLeader(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL is not set")
	}
	ctx := context.Background()
	a, err := NewPostgres(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := NewPostgres(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	ok, err := a.TryLock(ctx, 424242)
	if err != nil || !ok {
		t.Fatalf("leader a: %v %v", ok, err)
	}
	ok, err = b.TryLock(ctx, 424242)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("second engine took the leader lock")
	}
}

func TestPostgresActiveAlerts(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL is not set")
	}
	ctx := context.Background()
	db, err := NewPostgres(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	start := now.Add(-6 * time.Minute)
	alert := domain.Alert{
		ID: "alrt-ci-firing", Fingerprint: "ci-fp", Name: "OrderErrors", Service: "orders",
		Status: "firing", StartsAt: start,
	}
	if err := db.SaveAlert(ctx, alert); err != nil {
		t.Fatal(err)
	}
	got, err := db.ListActiveAlerts(ctx, now.Add(-5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, a := range got {
		if a.Fingerprint == "ci-fp" && a.Status == "firing" {
			found = true
		}
	}
	if !found {
		t.Fatalf("long-running firing alert missing: %+v", got)
	}
	ended := now.Add(-time.Minute)
	alert.Status = "resolved"
	alert.EndsAt = &ended
	if err := db.SaveAlert(ctx, alert); err != nil {
		t.Fatal(err)
	}
	got, err = db.ListActiveAlerts(ctx, now.Add(-5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range got {
		if a.Fingerprint == "ci-fp" && a.Status != "resolved" {
			t.Fatalf("resolved alert not updated: %+v", a)
		}
	}
}
