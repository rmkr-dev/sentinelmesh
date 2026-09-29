//go:build integration

package store

import (
	"context"
	"os"
	"testing"
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
