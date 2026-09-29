package kube

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	defaultTokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	defaultCAPath    = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
)

// TokenFile re-reads a projected service account token.
type TokenFile struct {
	Path string
	TTL  time.Duration

	mu     sync.Mutex
	token  string
	readAt time.Time
}

// Token returns the cached token until TTL elapses.
func (t *TokenFile) Token() (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	ttl := t.TTL
	if ttl <= 0 {
		ttl = time.Minute
	}
	if t.token != "" && time.Since(t.readAt) < ttl {
		return t.token, nil
	}
	path := t.Path
	if path == "" {
		path = defaultTokenPath
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(raw))
	if token == "" {
		return "", fmt.Errorf("kubernetes token file is empty")
	}
	t.token = token
	t.readAt = time.Now()
	return token, nil
}

// Invalidate forces the next Token call to read the file.
func (t *TokenFile) Invalidate() {
	t.mu.Lock()
	t.readAt = time.Time{}
	t.token = ""
	t.mu.Unlock()
}

// HTTPClient trusts caPEM and attaches a bearer token, refreshing it after 401.
func HTTPClient(caPEM []byte, tokens *TokenFile) (*http.Client, error) {
	pool := x509.NewCertPool()
	if len(caPEM) > 0 && !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("kubernetes CA bundle is not valid PEM")
	}
	base := &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
	}
	return &http.Client{
		Timeout:   15 * time.Second,
		Transport: &bearerTransport{base: base, tokens: tokens},
	}, nil
}

type bearerTransport struct {
	base   http.RoundTripper
	tokens *TokenFile
}

func (b *bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := b.roundTrip(req, false)
	if err != nil || resp == nil || resp.StatusCode != http.StatusUnauthorized || b.tokens == nil {
		return resp, err
	}
	resp.Body.Close()
	b.tokens.Invalidate()
	return b.roundTrip(req, true)
}

func (b *bearerTransport) roundTrip(req *http.Request, retry bool) (*http.Response, error) {
	clone := req.Clone(req.Context())
	if b.tokens != nil {
		token, err := b.tokens.Token()
		if err != nil {
			return nil, err
		}
		clone.Header.Set("Authorization", "Bearer "+token)
	}
	_ = retry
	return b.base.RoundTrip(clone)
}

// InClusterClient builds a client from the service account directory.
func InClusterClient(tokenPath, caPath string) (*http.Client, *TokenFile, error) {
	if tokenPath == "" {
		tokenPath = defaultTokenPath
	}
	if caPath == "" {
		caPath = defaultCAPath
	}
	ca, err := os.ReadFile(caPath)
	if err != nil {
		return nil, nil, fmt.Errorf("kubernetes CA: %w", err)
	}
	tokens := &TokenFile{Path: tokenPath}
	client, err := HTTPClient(ca, tokens)
	if err != nil {
		return nil, nil, err
	}
	return client, tokens, nil
}
