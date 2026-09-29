package kube

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

func podSignals(body []byte, label string, now time.Time) []domain.Signal {
	var list struct {
		Items []struct {
			Metadata struct {
				Name      string            `json:"name"`
				Namespace string            `json:"namespace"`
				Labels    map[string]string `json:"labels"`
			} `json:"metadata"`
			Status struct {
				Phase             string `json:"phase"`
				ContainerStatuses []struct {
					RestartCount int `json:"restartCount"`
					State        struct {
						Waiting *struct {
							Reason string `json:"reason"`
						} `json:"waiting"`
						Terminated *struct {
							Reason string `json:"reason"`
						} `json:"terminated"`
					} `json:"state"`
				} `json:"containerStatuses"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &list); err != nil {
		return nil
	}
	var out []domain.Signal
	for _, pod := range list.Items {
		svc := pod.Metadata.Name
		if label != "" {
			if v := pod.Metadata.Labels[label]; v != "" {
				svc = v
			}
		}
		for _, cs := range pod.Status.ContainerStatuses {
			reason := ""
			if cs.State.Waiting != nil {
				reason = cs.State.Waiting.Reason
			}
			if cs.State.Terminated != nil && cs.State.Terminated.Reason == "OOMKilled" {
				reason = "OOMKilled"
			}
			if reason == "" {
				continue
			}
			if reason != "CrashLoopBackOff" && reason != "ImagePullBackOff" && reason != "OOMKilled" && reason != "ErrImagePull" {
				continue
			}
			out = append(out, domain.Signal{
				ID:   "pod-" + pod.Metadata.Namespace + "-" + pod.Metadata.Name + "-" + reason,
				Type: domain.SignalK8s, Service: svc, Severity: "warning",
				Summary:     reason + " on " + pod.Metadata.Name,
				Fingerprint: "k8s|pod|" + pod.Metadata.Namespace + "|" + pod.Metadata.Name + "|" + reason,
				OccurredAt:  now,
				Attributes:  map[string]string{"reason": reason, "namespace": pod.Metadata.Namespace, "pod": pod.Metadata.Name},
			})
		}
	}
	return out
}

func nodeSignals(body []byte, now time.Time) []domain.Signal {
	var list struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Status struct {
				Conditions []struct {
					Type   string `json:"type"`
					Status string `json:"status"`
				} `json:"conditions"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &list); err != nil {
		return nil
	}
	var out []domain.Signal
	for _, node := range list.Items {
		for _, cond := range node.Status.Conditions {
			bad := (cond.Type == "Ready" && cond.Status != "True") ||
				((cond.Type == "MemoryPressure" || cond.Type == "DiskPressure" || cond.Type == "PIDPressure") && cond.Status == "True")
			if !bad {
				continue
			}
			reason := "Node" + cond.Type
			if cond.Type == "Ready" {
				reason = "NodeNotReady"
			}
			out = append(out, domain.Signal{
				ID: "node-" + node.Metadata.Name + "-" + reason, Type: domain.SignalK8s, Service: node.Metadata.Name,
				Severity: "critical", Summary: node.Metadata.Name + " " + reason,
				Fingerprint: "k8s|node|" + node.Metadata.Name + "|" + reason, OccurredAt: now,
				Attributes: map[string]string{"reason": reason, "node": node.Metadata.Name},
			})
		}
	}
	return out
}

func rolloutChanges(body []byte, now time.Time) []domain.Change {
	var list struct {
		Items []struct {
			Metadata struct {
				Name              string            `json:"name"`
				Namespace         string            `json:"namespace"`
				Annotations       map[string]string `json:"annotations"`
				CreationTimestamp string            `json:"creationTimestamp"`
			} `json:"metadata"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &list); err != nil {
		return nil
	}
	var out []domain.Change
	for _, item := range list.Items {
		rev := item.Metadata.Annotations["deployment.kubernetes.io/revision"]
		if rev == "" {
			continue
		}
		at, err := time.Parse(time.RFC3339, item.Metadata.CreationTimestamp)
		if err != nil {
			at = now
		}
		owner := item.Metadata.Name
		if i := strings.LastIndex(owner, "-"); i > 0 {
			owner = owner[:i]
		}
		out = append(out, domain.Change{
			ID: "rs-" + item.Metadata.Namespace + "-" + item.Metadata.Name, Kind: "rollout", Source: "kubernetes",
			Target: owner, OccurredAt: at, Attributes: map[string]string{"revision": rev, "namespace": item.Metadata.Namespace},
		})
	}
	return out
}
