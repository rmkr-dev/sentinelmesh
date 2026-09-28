package onboard

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOnboardWritesArtifacts(t *testing.T) {
	dir := t.TempDir()
	paths, err := Service(Options{Repo: dir, Name: "orders", Team: "checkout", Environment: "local"})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 4 {
		t.Fatalf("paths %d", len(paths))
	}
	if _, err := os.Stat(filepath.Join(dir, "config", "slo", "orders.yaml")); err != nil {
		t.Fatal(err)
	}
	if _, err := Service(Options{Repo: dir, Name: "orders", Team: "checkout"}); err == nil {
		t.Fatal("expected existing file error")
	}
	if _, err := Service(Options{Repo: dir, Name: "Bad", Team: "checkout"}); err == nil {
		t.Fatal("expected name error")
	}
}
