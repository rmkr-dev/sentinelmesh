package auth

import "testing"

func TestProductionRefusesCLI(t *testing.T) {
	_, err := New("azure_cli", "production")
	if err == nil {
		t.Fatal("expected refusal")
	}
}

func TestUnknownMode(t *testing.T) {
	_, err := New("certificate", "local")
	if err == nil {
		t.Fatal("expected error")
	}
}
