package incident

import "testing"

func TestAllowedPath(t *testing.T) {
	path := []string{"detected", "triaged", "investigating", "mitigating", "monitoring", "resolved", "closed"}
	for i := 0; i < len(path)-1; i++ {
		if err := Transition(path[i], path[i+1]); err != nil {
			t.Fatalf("%s -> %s: %v", path[i], path[i+1], err)
		}
	}
}

func TestRejectSkipToClosed(t *testing.T) {
	if err := Transition("detected", "closed"); err == nil {
		t.Fatal("expected error")
	}
}

func TestReopenFromResolved(t *testing.T) {
	if err := Transition("resolved", "investigating"); err != nil {
		t.Fatal(err)
	}
	if Open("resolved") {
		t.Fatal("resolved is not open")
	}
	if !Open("monitoring") {
		t.Fatal("monitoring is open")
	}
}
