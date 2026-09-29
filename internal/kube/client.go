// Package kube is a read-only Kubernetes adapter. It uses the same raw REST style as the remediation executor.
package kube

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

// Client lists events, pods, and nodes.
type Client struct {
	BaseURL      string
	Token        string
	Namespaces   []string
	ServiceLabel string
	HTTP         *http.Client
}

func (c Client) client() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 10 * time.Second}
}

// Snapshot is one read of the cluster.
type Snapshot struct {
	Events  []domain.K8sEvent
	Signals []domain.Signal
	Changes []domain.Change
}

// Collect returns warning events, workload symptoms, and rollout changes.
func (c Client) Collect(ctx context.Context, now time.Time) (signals []domain.Signal, events []domain.K8sEvent, changes []domain.Change, err error) {
	snap, err := c.collect(ctx, now)
	if err != nil {
		return nil, nil, nil, err
	}
	return snap.Signals, snap.Events, snap.Changes, nil
}

func (c Client) collect(ctx context.Context, now time.Time) (Snapshot, error) {
	if c.BaseURL == "" {
		return Snapshot{}, fmt.Errorf("kubernetes api url is empty")
	}
	var snap Snapshot
	events, err := c.events(ctx)
	if err != nil {
		return snap, err
	}
	snap.Events = events
	for _, ev := range events {
		if !symptomReason(ev.Reason) {
			continue
		}
		snap.Signals = append(snap.Signals, domain.Signal{
			ID:   "k8s-" + ev.Namespace + "-" + ev.Object + "-" + ev.Reason,
			Type: domain.SignalK8s, Service: serviceFromObject(ev.Object, c.ServiceLabel),
			Severity: "warning", Summary: ev.Reason + ": " + ev.Message,
			Fingerprint: "k8s|" + ev.Namespace + "|" + ev.Object + "|" + ev.Reason,
			OccurredAt:  ev.At, Attributes: map[string]string{"reason": ev.Reason, "namespace": ev.Namespace, "object": ev.Object},
		})
	}
	pods, err := c.get(ctx, "/api/v1/pods")
	if err != nil {
		return snap, err
	}
	snap.Signals = append(snap.Signals, podSignals(pods, c.ServiceLabel, now)...)
	nodes, err := c.get(ctx, "/api/v1/nodes")
	if err != nil {
		return snap, err
	}
	snap.Signals = append(snap.Signals, nodeSignals(nodes, now)...)
	sets, err := c.get(ctx, "/apis/apps/v1/replicasets")
	if err != nil {
		return snap, err
	}
	snap.Changes = rolloutChanges(sets, now)
	return snap, nil
}

func (c Client) events(ctx context.Context) ([]domain.K8sEvent, error) {
	body, err := c.get(ctx, "/api/v1/events")
	if err != nil {
		return nil, err
	}
	var list struct {
		Items []struct {
			Reason   string `json:"reason"`
			Message  string `json:"message"`
			Type     string `json:"type"`
			Count    int    `json:"count"`
			Metadata struct {
				Namespace string `json:"namespace"`
			} `json:"metadata"`
			InvolvedObject struct {
				Kind string `json:"kind"`
				Name string `json:"name"`
			} `json:"involvedObject"`
			LastTimestamp string `json:"lastTimestamp"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, err
	}
	var out []domain.K8sEvent
	for _, item := range list.Items {
		if item.Type != "" && !strings.EqualFold(item.Type, "Warning") {
			continue
		}
		if len(c.Namespaces) > 0 && !contains(c.Namespaces, item.Metadata.Namespace) {
			continue
		}
		at, _ := time.Parse(time.RFC3339, item.LastTimestamp)
		out = append(out, domain.K8sEvent{
			Reason: item.Reason, Message: item.Message, Type: item.Type,
			Object:    item.InvolvedObject.Kind + "/" + item.InvolvedObject.Name,
			Namespace: item.Metadata.Namespace, Count: item.Count, At: at,
		})
	}
	return out, nil
}

func (c Client) get(ctx context.Context, path string) ([]byte, error) {
	u, err := url.Parse(strings.TrimRight(c.BaseURL, "/") + path)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("kubernetes %s: %d", path, resp.StatusCode)
	}
	return b, nil
}

func symptomReason(reason string) bool {
	switch reason {
	case "CrashLoopBackOff", "OOMKilled", "BackOff", "FailedScheduling", "Failed", "Unhealthy", "NodeNotReady", "Evicted", "ImagePullBackOff", "ErrImagePull":
		return true
	default:
		return false
	}
}

func serviceFromObject(object, label string) string {
	name := object
	if i := strings.LastIndex(object, "/"); i >= 0 {
		name = object[i+1:]
	}
	if label == "" {
		return name
	}
	return name
}

func contains(ss []string, v string) bool {
	for _, s := range ss {
		if s == v {
			return true
		}
	}
	return false
}
