// Package azure holds Azure control-plane adapters. Domain types stay in internal/domain.
package azure

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

// Client calls Azure REST APIs with a bearer token.
type Client struct {
	Credential azcore.TokenCredential
	HTTP       *http.Client
	Scope      string
	// BaseURL rewrites the request host. Production leaves it empty.
	BaseURL string
}

func (c Client) absolute(raw string) string {
	if c.BaseURL == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	base, err := url.Parse(c.BaseURL)
	if err != nil {
		return raw
	}
	u.Scheme = base.Scheme
	u.Host = base.Host
	return u.String()
}

func (c Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 20 * time.Second}
}

// Do sends a request with an Entra token for c.Scope.
func (c Client) Do(ctx context.Context, req *http.Request) ([]byte, int, error) {
	if c.Credential == nil {
		return nil, 0, errString("azure credential is not configured")
	}
	scope := c.Scope
	if scope == "" {
		scope = "https://management.azure.com/.default"
	}
	tok, err := c.Credential.GetToken(ctx, policy.TokenRequestOptions{Scopes: []string{scope}})
	if err != nil {
		return nil, 0, err
	}
	req = req.Clone(ctx)
	req.Header.Set("Authorization", "Bearer "+tok.Token)
	resp, err := c.http().Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

type errString string

func (e errString) Error() string { return string(e) }
