package remediation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

// AzureExecutor calls ARM. Actions stay behind the same approval gate as Kubernetes.
type AzureExecutor struct {
	Token     string
	HTTP      *http.Client
	Allowlist map[string]bool
	BaseURL   string
}

func (a AzureExecutor) client() *http.Client {
	if a.HTTP != nil {
		return a.HTTP
	}
	return &http.Client{Timeout: 20 * time.Second}
}

func (a AzureExecutor) Execute(ctx context.Context, req domain.RemediationRequest) (map[string]any, map[string]any, error) {
	key := req.Action + ":" + req.Target
	if len(a.Allowlist) == 0 || (!a.Allowlist[req.Action] && !a.Allowlist[key]) {
		return nil, nil, fmt.Errorf("azure action %s is not allowlisted", req.Action)
	}
	if !strings.HasPrefix(req.Target, "/subscriptions/") {
		return nil, nil, fmt.Errorf("azure target must be a resource id")
	}
	before := map[string]any{"action": req.Action, "target": req.Target}
	var path string
	var body any
	switch req.Action {
	case "restart_web_app":
		path = req.Target + "/restart?api-version=2024-04-01"
	case "restart_container_app":
		path = req.Target + "/restart?api-version=2024-03-01"
	case "scale_container_app":
		path = req.Target + "?api-version=2024-03-01"
		body = map[string]any{"properties": map[string]any{"template": map[string]any{"scale": map[string]any{"minReplicas": req.Replicas}}}}
	default:
		return before, nil, fmt.Errorf("unsupported azure action %s", req.Action)
	}
	base := a.BaseURL
	if base == "" {
		base = "https://management.azure.com"
	}
	var payload io.Reader
	method := http.MethodPost
	if body != nil {
		b, _ := json.Marshal(body)
		payload = bytes.NewReader(b)
		method = http.MethodPatch
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(base, "/")+path, payload)
	if err != nil {
		return before, nil, err
	}
	if a.Token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+a.Token)
	}
	if body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.client().Do(httpReq)
	if err != nil {
		return before, nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return before, nil, fmt.Errorf("arm %d", resp.StatusCode)
	}
	return before, map[string]any{"status": resp.StatusCode, "body": string(raw)}, nil
}

// Keep a reference so the file stays used if Execute is inlined by tests.
var _ = time.Second
