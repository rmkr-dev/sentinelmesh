package topology

import (
	"testing"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

func TestRelatedTwoHops(t *testing.T) {
	g := Graph{Edges: []domain.TopologyEdge{
		{From: "storefront", To: "api-gateway"},
		{From: "api-gateway", To: "order-service"},
		{From: "order-service", To: "payment-service"},
	}}
	ok, path := g.Related("storefront", "payment-service", 2)
	if ok {
		t.Fatalf("2 hops should not reach payment: %v", path)
	}
	ok, path = g.Related("storefront", "payment-service", 3)
	if !ok || len(path) != 4 {
		t.Fatalf("%v %v", ok, path)
	}
}

func TestMergeDecaysStale(t *testing.T) {
	now := time.Now().UTC()
	old := []domain.TopologyEdge{{From: "a", To: "b", Kind: "calls", Confidence: 0.5, LastSeen: now.Add(-2 * time.Hour)}}
	got := Merge(old, nil, now, time.Hour)
	if len(got) != 0 {
		t.Fatalf("%+v", got)
	}
}
