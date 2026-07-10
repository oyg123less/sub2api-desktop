package redact

import (
	"strings"
	"testing"
)

func TestSanitize(t *testing.T) {
	jwt := strings.Repeat("a", 12) + "." + strings.Repeat("b", 12) + "." + strings.Repeat("c", 12)
	input := `key=sk-local-1234567890abcdef password="secret" token=` + jwt +
		` proxy=https://alice:secret@proxy.example user=a.person@example.com path=C:\Users\Astin\AppData`
	got := Sanitize(input)
	for _, secret := range []string{"1234567890abcdef", `"secret"`, jwt, "alice:secret", "a.person@example.com", `\Astin\`} {
		if strings.Contains(got, secret) {
			t.Fatalf("sanitized output still contains %q: %s", secret, got)
		}
	}
	for _, expected := range []string{"sk-local...cdef", "<redacted-token>", "https://<redacted>@", "a***@example.com", `C:\Users\<user>\AppData`} {
		if !strings.Contains(got, expected) {
			t.Fatalf("sanitized output missing %q: %s", expected, got)
		}
	}
}

func TestMaskEmailHandlesInvalidInput(t *testing.T) {
	if got := MaskEmail("invalid"); got != "<redacted-email>" {
		t.Fatalf("MaskEmail(invalid) = %q", got)
	}
}
