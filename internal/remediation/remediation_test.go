package remediation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

func TestPolicyDisabledByDefault(t *testing.T) {
	g := Gate{Policy: DefaultPolicy()}
	_, err := g.NewRequest("rollback_deployment", "payment-service", "error spike", "alice", "INC-1", time.Now())
	if err == nil {
		t.Fatal("expected disabled")
	}
}

func TestApprovalSeparation(t *testing.T) {
	g := Gate{Policy: Policy{Enabled: true, RequireApproval: true, Allowed: DefaultPolicy().Allowed}}
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	req, err := g.NewRequest("scale_deployment", "payment-service", "saturation", "alice", "INC-1", now)
	if err != nil {
		t.Fatal(err)
	}
	if req.Status != StatusPending {
		t.Fatal(req.Status)
	}
	if _, err := Approve(req, "alice", true, now); err == nil {
		t.Fatal("self approval must fail")
	}
	approved, err := Approve(req, "bob", true, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	exec := DemoExecutor{Apply: func(ctx context.Context, r domain.RemediationRequest) (map[string]any, map[string]any, error) {
		return map[string]any{"replicas": 2}, map[string]any{"replicas": 4}, nil
	}}
	done, err := Run(context.Background(), exec, approved, now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if done.Status != StatusExecuted || done.After["replicas"] != 4 {
		t.Fatalf("%+v", done)
	}
}

func TestKubernetesRollback(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /apis/apps/v1/namespaces/shop/replicasets", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"metadata": map[string]any{"annotations": map[string]string{"deployment.kubernetes.io/revision": "2"}},
					"spec":     map[string]any{"template": map[string]any{"metadata": map[string]any{"labels": map[string]string{"version": "bad"}}}},
				},
				{
					"metadata": map[string]any{"annotations": map[string]string{"deployment.kubernetes.io/revision": "1"}},
					"spec":     map[string]any{"template": map[string]any{"metadata": map[string]any{"labels": map[string]string{"version": "good"}}}},
				},
			},
		})
	})
	mux.HandleFunc("PATCH /apis/apps/v1/namespaces/shop/deployments/payment-service", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	ex := KubernetesExecutor{BaseURL: srv.URL, Token: "t", Namespace: "shop"}
	before, after, err := ex.Execute(context.Background(), domain.RemediationRequest{
		Action: "rollback_deployment", Target: "payment-service", Namespace: "shop",
	})
	if err != nil {
		t.Fatal(err)
	}
	if before["revision"] != 2 || after["revision"] != 1 {
		t.Fatalf("before=%v after=%v", before, after)
	}
}

func TestRejectBadNames(t *testing.T) {
	ex := KubernetesExecutor{BaseURL: "http://example", Namespace: "shop"}
	_, _, err := ex.Execute(context.Background(), domain.RemediationRequest{Action: "restart_pod", Target: "Bad_Name"})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
