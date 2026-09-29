package replay

import (
	"context"
	"testing"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

func TestStorageThrottlingOneIncident(t *testing.T) {
	inc, err := StorageThrottling(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if inc.ID != "INC-REPLAY-1" {
		t.Fatal(inc.ID)
	}
	var platform, health, change bool
	for _, s := range inc.Signals {
		switch s.Type {
		case domain.SignalPlatform:
			platform = true
		case domain.SignalResourceHealth:
			health = true
		case domain.SignalChange:
			change = true
		}
	}
	if !platform || !health || !change {
		t.Fatalf("signals %+v", inc.Signals)
	}
	if inc.Analysis == nil || inc.Analysis.ConfidenceLabel == "" {
		t.Fatal("missing analysis")
	}
}
