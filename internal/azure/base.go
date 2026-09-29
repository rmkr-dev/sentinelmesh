package azure

import (
	"net/http"
	"strings"
)

type baseRoundTripper struct {
	base string
	next http.RoundTripper
}

func (b baseRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return b.next.RoundTrip(r)
}

func baseFromClient(c *http.Client) string {
	if c == nil || c.Transport == nil {
		return ""
	}
	if b, ok := c.Transport.(baseRoundTripper); ok {
		return b.base
	}
	return ""
}

// WithBase returns a client whose requests are rewritten to base by the caller
// and whose transport is the test server.
func WithBase(base string, rt http.RoundTripper) *http.Client {
	return &http.Client{Transport: rewriting{base: strings.TrimRight(base, "/"), next: rt}}
}

type rewriting struct {
	base string
	next http.RoundTripper
}

func (r rewriting) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	path := req.URL.Path
	if path == "" {
		path = req.URL.RequestURI()
	}
	clone.URL.Scheme = "http"
	clone.URL.Host = strings.TrimPrefix(strings.TrimPrefix(r.base, "http://"), "https://")
	clone.URL.Path = req.URL.Path
	clone.URL.RawQuery = req.URL.RawQuery
	clone.Host = clone.URL.Host
	return r.next.RoundTrip(clone)
}
