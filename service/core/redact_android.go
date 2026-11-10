//go:build android

package core

import (
	"regexp"
	"strings"
)

// redactedToken replaces every word of a log line that is not on the
// allowlist.
const redactedToken = "[…]"

// allowedLogTokens is the allowlist for words in a shared log line. A word
// survives only if it is plainly not user data: a word made of letters, a
// module or setting name, a small number, a version, a time or a duration.
// Everything else, such as domains, addresses, keys, package names, paths and
// quoted values, is replaced. Matching what may stay, rather than what must
// go, means a kind of data nobody thought of is removed by default.
var allowedLogTokens = []*regexp.Regexp{
	// Words, optionally followed by punctuation: "failed", "connection,".
	regexp.MustCompile(`^[A-Za-z]{1,24}[,.;:!?)]*$`),
	// Opening punctuation before a word: "(timeout".
	regexp.MustCompile(`^[(\[][A-Za-z]{1,24}[,.;:!?)\]]*$`),
	// Module prefixes and setting keys: "firewall:", "intel/filterlists:",
	// "filter/blockOnMobile:".
	regexp.MustCompile(`^[a-z][A-Za-z]{0,23}(/[a-z][A-Za-z]{0,31}){0,3}:?$`),
	// Small numbers: counts, status codes, levels.
	regexp.MustCompile(`^\d{1,6}[,.;:)]*$`),
	// Decimals with one point: "1.5".
	regexp.MustCompile(`^\d{1,6}\.\d{1,6}$`),
	// Versions with at most three parts, so an IPv4 address never matches.
	regexp.MustCompile(`^v?\d{1,4}\.\d{1,4}(\.\d{1,6})?(-[A-Za-z]{1,12}(\.\d{1,4})?)?$`),
	// Times and dates of the log prefix: "08:56:57.463", "2026-11-03".
	regexp.MustCompile(`^\d{2}:\d{2}:\d{2}(\.\d{1,6})?$`),
	regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`),
	// Durations: "30s", "1.2ms", "5m0s".
	regexp.MustCompile(`^\d{1,6}(\.\d{1,6})?(ns|µs|us|ms|s|m|h)(\d{1,6}(\.\d{1,6})?(ns|µs|us|ms|s|m|h))*[,.]?$`),
	// Punctuation only: "▶", "-", "=", "|", the redacted marker itself.
	regexp.MustCompile(`^[\p{P}\p{S}]{1,4}$`),
	regexp.MustCompile(`^` + regexp.QuoteMeta(redactedToken) + `[,.;:)]*$`),
}

func allowedLogToken(token string) bool {
	for _, re := range allowedLogTokens {
		if re.MatchString(token) {
			return true
		}
	}
	return false
}

// RedactLogLine keeps only the allowlisted words of a log line, the same way
// the shared report does.
func RedactLogLine(line string) string {
	return redactForReport(line)
}

// redactForReport keeps only allowlisted words of a log line, so the user can
// share the report. Runs of removed words collapse into one marker.
func redactForReport(line string) string {
	fields := strings.Fields(line)
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		token := field
		if !allowedLogToken(token) {
			token = redactedToken
		}
		if token == redactedToken && len(out) > 0 && out[len(out)-1] == redactedToken {
			continue
		}
		out = append(out, token)
	}
	return strings.Join(out, " ")
}
