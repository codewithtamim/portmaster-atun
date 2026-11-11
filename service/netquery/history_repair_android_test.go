//go:build android

package netquery

import (
	"os"
	"path/filepath"
	"testing"

	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

func TestRepairHistoryFile(t *testing.T) {
	t.Parallel()

	t.Run("a file that is not a database is set aside", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		file := filepath.Join(dir, "history.db")
		if err := os.WriteFile(file, []byte("this is not a sqlite database, only text padding padding padding padding"), 0o600); err != nil {
			t.Fatal(err)
		}
		_ = os.WriteFile(file+"-wal", []byte("x"), 0o600)

		repairHistoryFile(file)

		if _, err := os.Stat(file); !os.IsNotExist(err) {
			t.Fatalf("damaged file still in place: %v", err)
		}
		if _, err := os.Stat(file + ".corrupt"); err != nil {
			t.Fatalf("damaged file not kept: %v", err)
		}
		if _, err := os.Stat(file + "-wal"); !os.IsNotExist(err) {
			t.Fatalf("stale log file not removed: %v", err)
		}
	})

	t.Run("a good database is left alone", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		file := filepath.Join(dir, "history.db")
		conn, err := sqlite.OpenConn(file, sqlite.OpenCreate, sqlite.OpenReadWrite)
		if err != nil {
			t.Fatal(err)
		}
		for _, stmt := range []string{"CREATE TABLE t (a INTEGER);", "INSERT INTO t VALUES (1);"} {
			if err := sqlitex.ExecuteTransient(conn, stmt, nil); err != nil {
				t.Fatal(err)
			}
		}
		_ = conn.Close()

		repairHistoryFile(file)

		if _, err := os.Stat(file); err != nil {
			t.Fatalf("good database moved: %v", err)
		}
		if _, err := os.Stat(file + ".corrupt"); !os.IsNotExist(err) {
			t.Fatalf("good database was copied aside: %v", err)
		}
	})

	t.Run("a missing file is fine", func(t *testing.T) {
		t.Parallel()
		repairHistoryFile(filepath.Join(t.TempDir(), "history.db"))
	})
}
