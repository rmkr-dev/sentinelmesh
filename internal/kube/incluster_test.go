package kube

import (
	"context"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHTTPClientTrustsProvidedCA(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer cluster-token" {
			http.Error(w, "auth", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "token")
	if err := os.WriteFile(tokenPath, []byte("cluster-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ca := pemCert(t, srv)
	client, err := HTTPClient(ca, &TokenFile{Path: tokenPath, TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatal(resp.Status)
	}
	plain := &http.Client{Timeout: 5 * time.Second}
	if _, err := plain.Get(srv.URL); err == nil {
		t.Fatal("default client trusted the test CA")
	}
}

func TestTokenRefreshOn401(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "token")
	if err := os.WriteFile(tokenPath, []byte("expired"), 0o600); err != nil {
		t.Fatal(err)
	}
	var saw []string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get("Authorization")
		saw = append(saw, got)
		if got == "Bearer expired" {
			if err := os.WriteFile(tokenPath, []byte("fresh"), 0o600); err != nil {
				t.Error(err)
			}
			http.Error(w, "expired", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	client, err := HTTPClient(pemCert(t, srv), &TokenFile{Path: tokenPath, TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatal(resp.Status)
	}
	if len(saw) != 2 || saw[1] != "Bearer fresh" {
		t.Fatalf("attempts %v", saw)
	}
}

func pemCert(t *testing.T, srv *httptest.Server) []byte {
	t.Helper()
	cert := srv.Certificate()
	if cert == nil {
		t.Fatal("no certificate")
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
}

func TestListPages(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("continue") == "" {
			_, _ = io.WriteString(w, `{"items":[{"type":"Warning","reason":"Evicted","message":"low disk","metadata":{"namespace":"shop"},"involvedObject":{"kind":"Pod","name":"pay"}}],"metadata":{"continue":"next"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"items":[{"type":"Warning","reason":"OOMKilled","message":"oom","metadata":{"namespace":"shop"},"involvedObject":{"kind":"Pod","name":"pay"}}]}`)
	}))
	defer srv.Close()
	events, err := (Client{BaseURL: srv.URL}).events(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(events) != 2 {
		t.Fatalf("calls=%d events=%d", calls, len(events))
	}
}
