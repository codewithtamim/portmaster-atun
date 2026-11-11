//go:build android

package netquery

import (
	"strings"
	"testing"
)

func TestLiveAndHistoryTakeEachConnectionOnce(t *testing.T) {
	got := platformSource([]DatabaseName{LiveDatabase, HistoryDatabase}, "WHERE verdict = :v")
	want := "SELECT * FROM history.connections WHERE verdict = :v UNION ALL " +
		"SELECT * FROM main.connections WHERE verdict = :v AND id NOT IN (SELECT id FROM history.connections)"
	if got != want {
		t.Fatalf("source\n got %q\nwant %q", got, want)
	}
	if !strings.Contains(platformSource([]DatabaseName{HistoryDatabase, LiveDatabase}, ""), "WHERE id NOT IN") {
		t.Fatal("no filter: the live part must still skip rows already in history")
	}
}

func TestOtherDatabaseSetsKeepTheDefault(t *testing.T) {
	for _, dbs := range [][]DatabaseName{{LiveDatabase}, {HistoryDatabase}, {LiveDatabase, LiveDatabase}} {
		if got := platformSource(dbs, "WHERE x = 1"); got != "" {
			t.Fatalf("%v: got %q, want the default", dbs, got)
		}
	}
}
