package remediation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
	"github.com/rmkr-dev/sentinelmesh/internal/kube"
)

var nameRE = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// KubernetesExecutor talks to the Kubernetes API. It supports restart (delete pod),
// scale, and rollback to the previous ReplicaSet template.
type KubernetesExecutor struct {
	BaseURL   string
	Token     string
	Tokens    *kube.TokenFile
	Namespace string
	HTTP      *http.Client
}

func (k KubernetesExecutor) client() *http.Client {
	if k.HTTP != nil {
		return k.HTTP
	}
	return &http.Client{Timeout: 20 * time.Second}
}

func (k KubernetesExecutor) Execute(ctx context.Context, req domain.RemediationRequest) (map[string]any, map[string]any, error) {
	ns := req.Namespace
	if ns == "" {
		ns = k.Namespace
	}
	if ns == "" {
		ns = "default"
	}
	if !nameRE.MatchString(ns) || !nameRE.MatchString(req.Target) {
		return nil, nil, fmt.Errorf("namespace and target must be DNS label names")
	}
	switch req.Action {
	case "restart_pod":
		before := map[string]any{"pod": req.Target, "namespace": ns}
		if err := k.do(ctx, http.MethodDelete, "/api/v1/namespaces/"+ns+"/pods/"+req.Target, nil); err != nil {
			return before, nil, err
		}
		return before, map[string]any{"deleted": req.Target}, nil
	case "scale_deployment":
		if req.Replicas < 0 || req.Replicas > 100 {
			return nil, nil, fmt.Errorf("replicas must be between 0 and 100")
		}
		path := "/apis/apps/v1/namespaces/" + ns + "/deployments/" + req.Target + "/scale"
		body := map[string]any{
			"apiVersion": "autoscaling/v1",
			"kind":       "Scale",
			"metadata":   map[string]any{"name": req.Target, "namespace": ns},
			"spec":       map[string]any{"replicas": req.Replicas},
		}
		before := map[string]any{"deployment": req.Target}
		if err := k.do(ctx, http.MethodPut, path, body); err != nil {
			return before, nil, err
		}
		return before, map[string]any{"replicas": req.Replicas}, nil
	case "rollback_deployment":
		return k.rollback(ctx, ns, req.Target)
	default:
		return nil, nil, fmt.Errorf("unsupported action %s", req.Action)
	}
}

func (k KubernetesExecutor) rollback(ctx context.Context, ns, name string) (map[string]any, map[string]any, error) {
	var list struct {
		Items []struct {
			Metadata struct {
				Annotations map[string]string `json:"annotations"`
			} `json:"metadata"`
			Spec struct {
				Template json.RawMessage `json:"template"`
			} `json:"spec"`
		} `json:"items"`
	}
	path := "/apis/apps/v1/namespaces/" + ns + "/replicasets?labelSelector=" + "app%3D" + name
	raw, err := k.get(ctx, path)
	if err != nil {
		return nil, nil, err
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, nil, err
	}
	type rev struct {
		n        int
		template json.RawMessage
	}
	var revs []rev
	for _, item := range list.Items {
		var n int
		fmt.Sscanf(item.Metadata.Annotations["deployment.kubernetes.io/revision"], "%d", &n)
		revs = append(revs, rev{n: n, template: item.Spec.Template})
	}
	if len(revs) < 2 {
		return nil, nil, fmt.Errorf("no previous revision for deployment %s", name)
	}
	current, previous := revs[0], revs[0]
	for _, r := range revs {
		if r.n > current.n {
			previous = current
			current = r
		} else if r.n > previous.n && r.n < current.n {
			previous = r
		} else if r.n < current.n && previous.n == current.n {
			previous = r
		}
	}
	if previous.n == 0 || previous.n == current.n {
		return nil, nil, fmt.Errorf("could not identify previous revision")
	}
	patch := map[string]any{"spec": map[string]any{"template": json.RawMessage(previous.template)}}
	before := map[string]any{"revision": current.n}
	if err := k.do(ctx, http.MethodPatch, "/apis/apps/v1/namespaces/"+ns+"/deployments/"+name, patch); err != nil {
		return before, nil, err
	}
	return before, map[string]any{"revision": previous.n}, nil
}

func (k KubernetesExecutor) get(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(k.BaseURL, "/")+path, nil)
	if err != nil {
		return nil, err
	}
	k.auth(req)
	resp, err := k.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("kubernetes GET %s: %d %s", path, resp.StatusCode, bytes.TrimSpace(body))
	}
	return body, nil
}

func (k KubernetesExecutor) do(ctx context.Context, method, path string, payload any) error {
	var buf io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		buf = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(k.BaseURL, "/")+path, buf)
	if err != nil {
		return err
	}
	k.auth(req)
	if payload != nil {
		if method == http.MethodPatch {
			req.Header.Set("Content-Type", "application/strategic-merge-patch+json")
		} else {
			req.Header.Set("Content-Type", "application/json")
		}
	}
	resp, err := k.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("kubernetes %s %s: %d %s", method, path, resp.StatusCode, bytes.TrimSpace(body))
	}
	return nil
}

func (k KubernetesExecutor) auth(req *http.Request) {
	token := k.Token
	if k.Tokens != nil {
		if fresh, err := k.Tokens.Token(); err == nil && fresh != "" {
			token = fresh
		}
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}
