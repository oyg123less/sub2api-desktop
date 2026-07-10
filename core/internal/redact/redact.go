// Package redact removes credentials and personal identifiers before data is
// written to logs, diagnostics, or user-shareable reports.
package redact

import (
	"regexp"
	"strings"
)

var (
	localKeyPattern   = regexp.MustCompile(`\bsk-local-[A-Za-z0-9_-]{8,}\b`)
	genericKeyPattern = regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{8,}\b`)
	jwtPattern        = regexp.MustCompile(`\b[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b`)
	hexTokenPattern   = regexp.MustCompile(`\b[0-9a-fA-F]{64,}\b`)
	passwordPattern   = regexp.MustCompile(`(?i)("?password"?\s*[:=]\s*")([^"\s]+)(")`)
	tokenFieldPattern = regexp.MustCompile(`(?i)("?(?:control_token|refresh_token|access_token|id_token)"?\s*[:=]\s*")([^"\s]+)(")`)
	proxyUserPattern  = regexp.MustCompile(`(?i)\b(https?|socks5)://[^/@\s]+@`)
	emailPattern      = regexp.MustCompile(`\b[A-Za-z0-9.!#$%&'*+/^_` + "`" + `{|}~-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`)
	windowsUserPath   = regexp.MustCompile(`(?i)\b([A-Z]:\\Users\\)[^\\\s]+`)
	unixUserPath      = regexp.MustCompile(`\b(/(?:home|Users)/)[^/\s]+`)
)

// Sanitize redacts all known sensitive forms from text while retaining enough
// structure for a support report to remain useful.
func Sanitize(input string) string {
	output := localKeyPattern.ReplaceAllStringFunc(input, maskLocalKey)
	output = passwordPattern.ReplaceAllString(output, `$1<redacted-password>$3`)
	output = tokenFieldPattern.ReplaceAllString(output, `$1<redacted-token>$3`)
	output = jwtPattern.ReplaceAllString(output, `<redacted-token>`)
	output = genericKeyPattern.ReplaceAllString(output, `<redacted-token>`)
	output = hexTokenPattern.ReplaceAllString(output, `<redacted-token>`)
	output = proxyUserPattern.ReplaceAllString(output, `$1://<redacted>@`)
	output = emailPattern.ReplaceAllStringFunc(output, MaskEmail)
	output = windowsUserPath.ReplaceAllString(output, `${1}<user>`)
	output = unixUserPath.ReplaceAllString(output, `${1}<user>`)
	return output
}

func maskLocalKey(value string) string {
	if len(value) <= 12 {
		return "<redacted-token>"
	}
	return value[:8] + "..." + value[len(value)-4:]
}

// MaskEmail preserves the domain and first local-part rune for identification.
func MaskEmail(email string) string {
	local, domain, ok := strings.Cut(email, "@")
	if !ok || local == "" || domain == "" {
		return "<redacted-email>"
	}
	first := []rune(local)[0]
	return string(first) + "***@" + domain
}
