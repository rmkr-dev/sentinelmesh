// Package redaction removes secrets and selected PII before telemetry is stored
// or sent to an AI provider.
package redaction

import (
	"net/url"
	"regexp"
	"strings"
)

// Policy lists fields and patterns to remove. Matching is case-insensitive for names.
type Policy struct {
	Headers         []string
	QueryParameters []string
	JSONFields      []string
	RedactEmails    bool
	ExtraPatterns   []string
}

// DefaultPolicy covers credentials commonly found in HTTP telemetry.
func DefaultPolicy() Policy {
	return Policy{
		Headers: []string{
			"authorization", "cookie", "set-cookie", "x-api-key", "proxy-authorization",
		},
		QueryParameters: []string{
			"token", "password", "access_token", "id_token", "api_key", "apikey", "secret",
		},
		JSONFields: []string{
			"password", "token", "access_token", "refresh_token", "api_key", "secret",
			"card_token", "card_number", "pan", "ssn", "authorization",
		},
		RedactEmails: true,
	}
}

const redacted = "[REDACTED]"

var (
	bearerRE = regexp.MustCompile(`(?i)bearer\s+[a-z0-9\-._~+/]+=*`)
	emailRE  = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	awsRE    = regexp.MustCompile(`AKIA[0-9A-Z]{16}`)
	connRE   = regexp.MustCompile(`(?i)(password|pwd|token|secret|api[_-]?key)\s*[=:]\s*\S+`)
)

// RedactString applies value patterns.
func RedactString(policy Policy, in string) string {
	out := bearerRE.ReplaceAllString(in, "Bearer "+redacted)
	out = awsRE.ReplaceAllString(out, redacted)
	out = connRE.ReplaceAllString(out, "$1="+redacted)
	if policy.RedactEmails {
		out = emailRE.ReplaceAllString(out, redacted)
	}
	for _, p := range policy.ExtraPatterns {
		re, err := regexp.Compile(p)
		if err != nil {
			continue
		}
		out = re.ReplaceAllString(out, redacted)
	}
	return out
}

// RedactHeaders returns a copy with blocked header values removed.
func RedactHeaders(policy Policy, headers map[string]string) map[string]string {
	blocked := nameSet(policy.Headers)
	out := make(map[string]string, len(headers))
	for k, v := range headers {
		if blocked[strings.ToLower(k)] {
			out[k] = redacted
			continue
		}
		out[k] = RedactString(policy, v)
	}
	return out
}

// RedactQuery removes blocked query parameters from a URL string.
func RedactQuery(policy Policy, rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.RawQuery == "" {
		return RedactString(policy, rawURL)
	}
	blocked := nameSet(policy.QueryParameters)
	q := u.Query()
	for key := range q {
		if blocked[strings.ToLower(key)] {
			q.Set(key, redacted)
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// RedactMap recursively redacts map keys listed in the policy and string values.
func RedactMap(policy Policy, in map[string]any) map[string]any {
	blocked := nameSet(append(append([]string{}, policy.JSONFields...), policy.Headers...))
	return redactValue(policy, blocked, in).(map[string]any)
}

func redactValue(policy Policy, blocked map[string]bool, v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, child := range t {
			if blocked[strings.ToLower(k)] {
				out[k] = redacted
				continue
			}
			out[k] = redactValue(policy, blocked, child)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, child := range t {
			out[i] = redactValue(policy, blocked, child)
		}
		return out
	case string:
		return RedactString(policy, t)
	default:
		return v
	}
}

func nameSet(names []string) map[string]bool {
	out := make(map[string]bool, len(names))
	for _, n := range names {
		out[strings.ToLower(n)] = true
	}
	return out
}
