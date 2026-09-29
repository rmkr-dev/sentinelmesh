// Package auth validates callers and maps them to roles.
package auth

import "strings"

const (
	RoleViewer    = "viewer"
	RoleResponder = "responder"
	RoleApprover  = "approver"
	RoleAdmin     = "admin"
)

// Allow reports whether any role may perform the action.
func Allow(roles []string, action string) bool {
	need := map[string]string{
		"read":       RoleViewer,
		"analyze":    RoleResponder,
		"transition": RoleResponder,
		"silence":    RoleResponder,
		"approve":    RoleApprover,
		"catalog":    RoleAdmin,
		"admin":      RoleAdmin,
	}
	rank := map[string]int{RoleViewer: 1, RoleResponder: 2, RoleApprover: 3, RoleAdmin: 4}
	min := rank[need[action]]
	if min == 0 {
		min = rank[RoleAdmin]
	}
	for _, role := range roles {
		if rank[strings.ToLower(role)] >= min {
			return true
		}
	}
	return false
}
