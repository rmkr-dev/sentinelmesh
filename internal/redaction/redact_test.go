package redaction

import "testing"

func TestRedactHeadersAndBearer(t *testing.T) {
	p := DefaultPolicy()
	got := RedactHeaders(p, map[string]string{
		"Authorization": "Bearer super-secret-token",
		"Cookie":        "session=abc",
		"X-Request-Id":  "req-1",
	})
	if got["Authorization"] != "[REDACTED]" || got["Cookie"] != "[REDACTED]" {
		t.Fatalf("%v", got)
	}
	if got["X-Request-Id"] != "req-1" {
		t.Fatal(got["X-Request-Id"])
	}
	if RedactString(p, "token=abc123") == "token=abc123" {
		t.Fatal("token assignment should be redacted")
	}
}

func TestRedactQueryAndJSON(t *testing.T) {
	p := DefaultPolicy()
	u := RedactQuery(p, "https://shop.example/pay?token=sekret&sku=sku-1")
	if contains(u, "sekret") {
		t.Fatal(u)
	}
	if !contains(u, "sku-1") && !contains(u, "sku=sku-1") {
		t.Fatal(u)
	}
	body := RedactMap(p, map[string]any{
		"sku":        "sku-1",
		"card_token": "tok_live_123",
		"note":       "email ada@example.com password=hunter2",
	})
	if body["card_token"] != "[REDACTED]" {
		t.Fatal(body["card_token"])
	}
	note := body["note"].(string)
	if contains(note, "ada@example.com") || contains(note, "hunter2") {
		t.Fatal(note)
	}
	if body["sku"] != "sku-1" {
		t.Fatal(body["sku"])
	}
}

func TestAWSKey(t *testing.T) {
	got := RedactString(DefaultPolicy(), "key AKIAIOSFODNN7EXAMPLE leaked")
	if contains(got, "AKIAIOSFODNN7EXAMPLE") {
		t.Fatal(got)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || (len(s) > 0 && (indexOf(s, sub) >= 0)))
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
