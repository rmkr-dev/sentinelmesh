package remediation

import (
	"context"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

// DemoExecutor applies an allowlisted action to the local fault catalog.
// It is used for the local demonstration, not for production clusters.
type DemoExecutor struct {
	Apply func(ctx context.Context, req domain.RemediationRequest) (before, after map[string]any, err error)
}

func (d DemoExecutor) Execute(ctx context.Context, req domain.RemediationRequest) (map[string]any, map[string]any, error) {
	if d.Apply == nil {
		return nil, nil, errNoApply
	}
	return d.Apply(ctx, req)
}

var errNoApply = errString("demo executor is not configured")

type errString string

func (e errString) Error() string { return string(e) }
