package logging

import (
	"bytes"
	"strings"
	"testing"
)

func TestRedactSecrets(t *testing.T) {
	cases := []string{
		"password=secret",
		"postgres://user:pass@localhost/db",
		"api_token=abc",
	}
	for _, tc := range cases {
		if got := Redact(tc); got != "[REDACTED]" {
			t.Fatalf("Redact(%q) = %q", tc, got)
		}
	}
}

func TestLoggerWritesJSONLine(t *testing.T) {
	var out bytes.Buffer
	log := New(&out, "info")
	log.Info("server starting", "addr", ":8080")

	got := out.String()
	if !strings.Contains(got, `"message":"server starting"`) {
		t.Fatalf("log output missing message: %s", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Fatalf("log output should end with newline: %q", got)
	}
}
