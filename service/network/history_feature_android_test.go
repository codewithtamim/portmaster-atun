//go:build android

package network

import "testing"

func TestHistoryRecordsWithoutAccount(t *testing.T) {
	t.Parallel()
	if !historyFeatureAllowed(nil) {
		t.Fatal("Android history must record without an SPN account")
	}
}
