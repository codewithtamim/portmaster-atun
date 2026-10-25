//go:build !android

package network

import "testing"

func TestHistoryStillRequiresAccount(t *testing.T) {
	t.Parallel()
	if historyFeatureAllowed(nil) {
		t.Fatal("history without an account must stay off")
	}
}
