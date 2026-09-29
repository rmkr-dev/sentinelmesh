package faults

import "testing"

func TestCatalogAndFind(t *testing.T) {
	all := Catalog()
	if len(all) < 5 {
		t.Fatalf("catalog %d", len(all))
	}
	seen := map[string]bool{}
	for _, spec := range all {
		if spec.Name == "" || spec.Service == "" || spec.Kind == "" || spec.ExpectedAlert == "" {
			t.Fatalf("%+v", spec)
		}
		seen[spec.Name] = true
		got, ok := Find(spec.Name)
		if !ok || got.Service != spec.Service {
			t.Fatalf("find %s", spec.Name)
		}
	}
	if _, ok := Find("not-a-fault"); ok {
		t.Fatal("unknown fault")
	}
	if !seen["payment-latency"] || !seen["deployment-regression"] {
		t.Fatalf("%v", seen)
	}
}
