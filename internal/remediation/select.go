package remediation

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

// SelectOptions chooses the remediation executor from configuration.
type SelectOptions struct {
	Executor           string
	Namespace          string
	DemoEnabled        bool
	RemediationEnabled bool
	DemoApply          func(context.Context, domain.RemediationRequest) (map[string]any, map[string]any, error)
	Token              string
	APIBase            string
	TokenPath          string
	ServiceHost        string
	ServicePort        string
}

// Select builds the executor named by configuration.
// The demo executor is refused when remediation is on and the demo is off.
// The Kubernetes executor reads the in-cluster service account token. It does not accept a token from configuration.
func Select(opts SelectOptions) (Executor, error) {
	kind := opts.Executor
	if kind == "" {
		kind = "demo"
	}
	switch kind {
	case "demo":
		if opts.RemediationEnabled && !opts.DemoEnabled {
			return nil, fmt.Errorf("remediation executor demo is refused when demo is disabled and remediation is enabled")
		}
		return DemoExecutor{Apply: opts.DemoApply}, nil
	case "kubernetes":
		token := opts.Token
		if token == "" {
			path := opts.TokenPath
			if path == "" {
				path = "/var/run/secrets/kubernetes.io/serviceaccount/token"
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("kubernetes executor token: %w", err)
			}
			token = strings.TrimSpace(string(raw))
			if token == "" {
				return nil, fmt.Errorf("kubernetes executor token file is empty")
			}
		}
		base := opts.APIBase
		if base == "" {
			host := opts.ServiceHost
			if host == "" {
				host = os.Getenv("KUBERNETES_SERVICE_HOST")
			}
			port := opts.ServicePort
			if port == "" {
				port = os.Getenv("KUBERNETES_SERVICE_PORT")
			}
			if port == "" {
				port = "443"
			}
			if host == "" {
				return nil, fmt.Errorf("kubernetes executor requires the in-cluster API host")
			}
			base = "https://" + host + ":" + port
		}
		ns := opts.Namespace
		if ns == "" {
			ns = "default"
		}
		return KubernetesExecutor{BaseURL: base, Token: token, Namespace: ns}, nil
	default:
		return nil, fmt.Errorf("unknown remediation executor %q", kind)
	}
}
