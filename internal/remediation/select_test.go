package remediation

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

func TestSelectExecutorMatrix(t *testing.T) {
	apply := func(context.Context, domain.RemediationRequest) (map[string]any, map[string]any, error) {
		return map[string]any{"before": true}, map[string]any{"after": true}, nil
	}
	cases := []struct {
		name    string
		opts    SelectOptions
		want    string
		wantErr bool
	}{
		{name: "explicit demo", opts: SelectOptions{Executor: "demo", DemoApply: apply}, want: "demo"},
		{name: "empty executor", opts: SelectOptions{DemoApply: apply}, wantErr: true},
		{name: "demo while remediation off", opts: SelectOptions{Executor: "demo", DemoEnabled: false, RemediationEnabled: false, DemoApply: apply}, want: "demo"},
		{name: "demo refused when remediation on and demo off", opts: SelectOptions{Executor: "demo", DemoEnabled: false, RemediationEnabled: true, DemoApply: apply}, wantErr: true},
		{name: "demo allowed for local", opts: SelectOptions{Executor: "demo", DemoEnabled: true, RemediationEnabled: true, DemoApply: apply}, want: "demo"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			exec, err := Select(tc.opts)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			switch tc.want {
			case "demo":
				if _, ok := exec.(DemoExecutor); !ok {
					t.Fatalf("%T", exec)
				}
			}
		})
	}
}

func TestSelectKubernetesReadsTokenFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	if err := os.WriteFile(path, []byte("in-cluster-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	exec, err := Select(SelectOptions{
		Executor:    "kubernetes",
		Namespace:   "shop",
		TokenPath:   path,
		APIBase:     "https://kubernetes.default.svc",
		DemoEnabled: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	k, ok := exec.(KubernetesExecutor)
	if !ok {
		t.Fatalf("%T", exec)
	}
	if k.Token != "in-cluster-token" || k.Namespace != "shop" || k.BaseURL != "https://kubernetes.default.svc" {
		t.Fatalf("%+v", k)
	}
}

func TestSelectKubernetesMissingToken(t *testing.T) {
	_, err := Select(SelectOptions{Executor: "kubernetes", TokenPath: filepath.Join(t.TempDir(), "missing")})
	if err == nil {
		t.Fatal("expected error")
	}
}
