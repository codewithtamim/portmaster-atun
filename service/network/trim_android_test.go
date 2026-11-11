//go:build android

package network

import "testing"

func TestOldestBeyondKeepsTheNewest(t *testing.T) {
	ended := []endedConn{{at: 50}, {at: 10}, {at: 40}, {at: 20}, {at: 30}}
	drop := oldestBeyond(ended, 2)
	if len(drop) != 3 {
		t.Fatalf("dropped %d, want 3", len(drop))
	}
	for _, e := range drop {
		if e.at > 30 {
			t.Fatalf("dropped a record that ended at %d, one of the two newest", e.at)
		}
	}
}

func TestOldestBeyondLeavesSmallStoresAlone(t *testing.T) {
	if drop := oldestBeyond([]endedConn{{at: 1}, {at: 2}}, 2); drop != nil {
		t.Fatalf("dropped %d records from a store within the limit", len(drop))
	}
}
