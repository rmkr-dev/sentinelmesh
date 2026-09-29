// Package remediation authorizes and records actions. Execution is disabled
// unless policy explicitly allows the action and, by default, a human approves it.
package remediation

import (
	"context"
	"fmt"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

const (
	StatusPending  = "pending"
	StatusApproved = "approved"
	StatusRejected = "rejected"
	StatusExecuted = "executed"
	StatusFailed   = "failed"
)

// Policy is the remediation gate.
type Policy struct {
	Enabled         bool
	RequireApproval bool
	Allowed         map[string]bool
}

// Executor performs an already-approved action.
type Executor interface {
	Execute(ctx context.Context, req domain.RemediationRequest) (before, after map[string]any, err error)
}

// Gate decides whether a request may be created or executed.
type Gate struct {
	Policy Policy
}

func (g Gate) Authorize(action string) error {
	if !g.Policy.Enabled {
		return fmt.Errorf("remediation is disabled by policy")
	}
	if !g.Policy.Allowed[action] {
		return fmt.Errorf("action %q is not allowed", action)
	}
	return nil
}

// NewRequest creates a pending or immediately approved request according to policy.
func (g Gate) NewRequest(action, target, reason, actor, incidentID string, now time.Time) (domain.RemediationRequest, error) {
	if err := g.Authorize(action); err != nil {
		return domain.RemediationRequest{}, err
	}
	if actor == "" {
		return domain.RemediationRequest{}, fmt.Errorf("actor is required")
	}
	if reason == "" {
		return domain.RemediationRequest{}, fmt.Errorf("reason is required")
	}
	status := StatusPending
	approver := ""
	if !g.Policy.RequireApproval {
		status = StatusApproved
		approver = actor
	}
	return domain.RemediationRequest{
		Action:     action,
		Target:     target,
		IncidentID: incidentID,
		Status:     status,
		Reason:     reason,
		Actor:      actor,
		Approver:   approver,
		CreatedAt:  now.UTC(),
		UpdatedAt:  now.UTC(),
	}, nil
}

// Approve records a human decision. The requester cannot approve their own action
// when approval is required.
func Approve(req domain.RemediationRequest, approver string, accept bool, now time.Time) (domain.RemediationRequest, error) {
	if req.Status != StatusPending {
		return req, fmt.Errorf("request is %s", req.Status)
	}
	if approver == "" {
		return req, fmt.Errorf("approver is required")
	}
	if approver == req.Actor {
		return req, fmt.Errorf("requester cannot approve their own remediation")
	}
	req.Approver = approver
	req.UpdatedAt = now.UTC()
	if accept {
		req.Status = StatusApproved
	} else {
		req.Status = StatusRejected
	}
	return req, nil
}

// Run executes an approved request and stores before/after state.
func Run(ctx context.Context, exec Executor, req domain.RemediationRequest, now time.Time) (domain.RemediationRequest, error) {
	if req.Status != StatusApproved {
		return req, fmt.Errorf("only approved requests can execute, status is %s", req.Status)
	}
	if exec == nil {
		return req, fmt.Errorf("no executor configured")
	}
	before, after, err := exec.Execute(ctx, req)
	req.Before = before
	req.After = after
	req.UpdatedAt = now.UTC()
	if err != nil {
		req.Status = StatusFailed
		req.Error = err.Error()
		return req, err
	}
	req.Status = StatusExecuted
	return req, nil
}
