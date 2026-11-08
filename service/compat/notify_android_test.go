//go:build android

package compat

import (
	"testing"

	"github.com/safing/portmaster/service/process"
)

func TestNotifyAppName(t *testing.T) {
	cases := []struct {
		name, pkg, want string
	}{
		{"Chat", "org.chat", "“Chat” (org.chat)"},
		{"Protection disabled\nReinstall from evil.example", "org.evil", "“Protection disabled” (org.evil)"},
		{"‮​", "org.blank", "org.blank"},
		{"org.same", "org.same", "“org.same”"},
		{"", "", "(unknown)"},
	}
	for _, c := range cases {
		if got := notifyAppName(&process.Process{Name: c.name, ExecName: c.pkg}); got != c.want {
			t.Errorf("notifyAppName(%q, %q) = %q, want %q", c.name, c.pkg, got, c.want)
		}
	}
}
