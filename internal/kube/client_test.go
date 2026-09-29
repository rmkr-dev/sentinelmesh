package kube

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCollectCrashLoopAndNode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/events":
			_, _ = w.Write([]byte(`{"items":[{"type":"Warning","reason":"FailedScheduling","message":"0/1 nodes","count":3,"metadata":{"namespace":"shop"},"involvedObject":{"kind":"Pod","name":"payment"},"lastTimestamp":"2026-09-29T00:00:00Z"}]}`))
		case "/api/v1/pods":
			_, _ = w.Write([]byte(`{"items":[{"metadata":{"name":"payment-abc","namespace":"shop","labels":{"app.kubernetes.io/name":"payment-service"}},"status":{"phase":"Running","containerStatuses":[{"restartCount":4,"state":{"waiting":{"reason":"CrashLoopBackOff"}}}]}}]}`))
		case "/api/v1/nodes":
			_, _ = w.Write([]byte(`{"items":[{"metadata":{"name":"node-a"},"status":{"conditions":[{"type":"Ready","status":"False"},{"type":"MemoryPressure","status":"False"}]}}]}`))
		case "/apis/apps/v1/replicasets":
			_, _ = w.Write([]byte(`{"items":[{"metadata":{"name":"payment-service-7","namespace":"shop","creationTimestamp":"2026-09-29T00:01:00Z","annotations":{"deployment.kubernetes.io/revision":"4"}}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	signals, events, changes, err := (Client{BaseURL: srv.URL, ServiceLabel: "app.kubernetes.io/name"}).Collect(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || len(changes) != 1 {
		t.Fatalf("events=%d changes=%d", len(events), len(changes))
	}
	var crash, node bool
	for _, s := range signals {
		if s.Attributes["reason"] == "CrashLoopBackOff" && s.Service == "payment-service" {
			crash = true
		}
		if s.Attributes["reason"] == "NodeNotReady" {
			node = true
		}
	}
	if !crash || !node {
		t.Fatalf("signals %+v", signals)
	}
}
