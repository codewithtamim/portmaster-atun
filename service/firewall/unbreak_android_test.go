//go:build android

package firewall

import "testing"

func TestBlockedByAny(t *testing.T) {
	security := []string{"urlhaus", "phishing-army"}
	if !blockedByAny([]string{"easyprivacy", "urlhaus", "unbreak"}, security) {
		t.Fatal("a malware list match must keep the block")
	}
	if blockedByAny([]string{"easyprivacy", "unbreak"}, security) {
		t.Fatal("a tracker-only match may be lifted by unbreak")
	}
	if blockedByAny(nil, security) {
		t.Fatal("no matches means nothing to keep")
	}
}
