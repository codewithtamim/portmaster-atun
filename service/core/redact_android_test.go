//go:build android

package core

import (
	"strings"
	"testing"
)

func TestRedactForReportKeepsOnlyAllowedWords(t *testing.T) {
	line := `261103 08:56:57.463 ▶ WARN 412 firewall: blocking tracker.example.com (203.0.113.5:443) for com.example.app after 1.2s`
	got := redactForReport(line)
	for _, gone := range []string{"tracker.example.com", "203.0.113.5", "com.example.app", "443"} {
		if strings.Contains(got, gone) {
			t.Errorf("%q survived: %s", gone, got)
		}
	}
	for _, kept := range []string{"08:56:57.463", "WARN", "firewall:", "blocking", "after", "1.2s"} {
		if !strings.Contains(got, kept) {
			t.Errorf("%q was removed: %s", kept, got)
		}
	}
}

func TestRedactForReportRemovesUnknownShapes(t *testing.T) {
	for _, secret := range []string{
		"2001:db8::1",
		"yAnz5TF+lXXJte14tji3zlMNq+hd2rYUIgJBgB3fBmk=",
		`"alice@example.com"`,
		"/data/user/0/io.safing/files",
		"10.0.0.1",
		"Bearer abc.def",
	} {
		got := redactForReport("value " + secret)
		if strings.Contains(got, strings.Fields(secret)[0]) && secret != "Bearer abc.def" {
			t.Errorf("%q survived: %s", secret, got)
		}
		if secret == "Bearer abc.def" && strings.Contains(got, "abc.def") {
			t.Errorf("token survived: %s", got)
		}
	}
}

func TestRedactForReportCollapsesRuns(t *testing.T) {
	got := redactForReport("dial a.example b.example c.example failed")
	if got != "dial […] failed" {
		t.Fatalf("got %q", got)
	}
}
