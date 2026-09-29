package auth

import "testing"

func TestRoleMatrix(t *testing.T) {
	if Allow([]string{RoleViewer}, "transition") {
		t.Fatal("viewer transition")
	}
	if !Allow([]string{RoleResponder}, "analyze") {
		t.Fatal("responder analyze")
	}
	if Allow([]string{RoleResponder}, "approve") {
		t.Fatal("responder approve")
	}
	if !Allow([]string{RoleApprover}, "approve") {
		t.Fatal("approver")
	}
	if !Allow([]string{RoleAdmin}, "catalog") {
		t.Fatal("admin")
	}
}
