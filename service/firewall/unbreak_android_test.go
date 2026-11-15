//go:build android

package firewall

import (
	"context"
	"testing"

	"github.com/safing/portmaster/service/intel"
)

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

// The check that runs after a DNS answer clears the entity's list matches and
// nothing loads them again, so it cannot block by filter list. If that ever
// changes, it must apply the unbreak rule before blocking, as the main filter
// list check does, or every unbroken domain is blocked after it resolves.
func TestListMatchesAreGoneAfterReset(t *testing.T) {
	t.Parallel()

	entity := (&intel.Entity{Domain: "example.com."}).Init(443)
	entity.ListOccurences = map[string][]string{"example.com.": {"TRAC", "UNBREAK"}}
	if !entity.MatchLists([]string{"TRAC", "UNBREAK"}) {
		t.Fatal("the entity must match its lists before the reset")
	}

	entity.ResetLists()
	entity.EnableCNAMECheck(context.Background(), true)
	if entity.MatchLists([]string{"TRAC", "UNBREAK"}) {
		t.Fatal("list matches survived the reset: the check after DNS answers can now block unbroken domains")
	}
}
