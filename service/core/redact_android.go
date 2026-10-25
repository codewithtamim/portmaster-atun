//go:build android

package core

import "regexp"

var (
	emailRe = regexp.MustCompile(`\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}\b`)
	loginRe = regexp.MustCompile(`logged in as "[^"]*" on device "[^"]*"`)
)

func redactPlatformSecrets(line string) string {
	out := loginRe.ReplaceAllString(line, "logged in as [redacted] on device [redacted]")
	return emailRe.ReplaceAllString(out, "[redacted]")
}

var (
	ipv4Re         = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	ipv6Re         = regexp.MustCompile(`\b(?:[0-9a-fA-F]{1,4}:){2,7}[0-9a-fA-F]{1,4}\b`)
	ipv6CompressRe = regexp.MustCompile(`(?i)\b[0-9a-f:]*::[0-9a-f:]*\b`)
	domainRe       = regexp.MustCompile(`\b(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[A-Za-z]{2,}\b`)
)

// redactForReport removes secrets, domains and addresses from a diagnostics
// line so the user can share the report.
func redactForReport(line string) string {
	out := redactPlatformSecrets(line)
	out = ipv6CompressRe.ReplaceAllString(out, "[redacted]")
	out = ipv6Re.ReplaceAllString(out, "[redacted]")
	out = ipv4Re.ReplaceAllString(out, "[redacted]")
	return domainRe.ReplaceAllString(out, "[redacted]")
}
