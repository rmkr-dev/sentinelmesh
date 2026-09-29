package remediation

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

func TestAzureExecutorAllowlist(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test" {
			http.Error(w, "auth", 401)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	exec := AzureExecutor{
		Token: "test", BaseURL: srv.URL, HTTP: srv.Client(),
		Allowlist: map[string]bool{"restart_web_app": true},
	}
	_, _, err := exec.Execute(context.Background(), domain.RemediationRequest{
		Action: "scale_container_app", Target: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/demo/providers/Microsoft.App/containerApps/shop",
	})
	if err == nil {
		t.Fatal("expected allowlist rejection")
	}
	before, after, err := exec.Execute(context.Background(), domain.RemediationRequest{
		Action: "restart_web_app", Target: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/demo/providers/Microsoft.Web/sites/fn",
	})
	if err != nil || before["target"] == "" || after["status"] != http.StatusOK {
		t.Fatalf("%v %v %v", before, after, err)
	}
}
