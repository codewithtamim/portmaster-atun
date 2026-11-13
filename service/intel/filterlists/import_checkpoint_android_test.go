//go:build android

package filterlists

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/safing/portmaster/base/database"
	_ "github.com/safing/portmaster/base/database/storage/hashmap"
	"github.com/safing/portmaster/service/updates"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "filterlists")
	if err != nil {
		fmt.Printf("failed to create data directory: %s\n", err)
		os.Exit(1)
	}
	if err := database.Initialize(dir); err != nil {
		fmt.Printf("failed to initialize database: %s\n", err)
		os.Exit(1)
	}
	if _, err := database.Register(&database.Database{
		Name:        "cache",
		Description: "test cache",
		StorageType: "hashmap",
	}); err != nil {
		fmt.Printf("failed to register cache database: %s\n", err)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func listFiles(baseVersion string) []*updates.Artifact {
	return []*updates.Artifact{
		{Filename: baseListFilePath, Version: baseVersion},
		{Filename: intermediateListFilePath, Version: "1.0.1"},
	}
}

func TestImportResumesWhereItStopped(t *testing.T) { //nolint:paralleltest // shares the cache database.
	t.Cleanup(finishImport)
	files := listFiles("1.0.0")
	first := time.Unix(1_000, 0)

	started := beginImport(files, first)
	if !started.Equal(first) {
		t.Fatalf("fresh import started at %s, want %s", started, first)
	}
	if got := importResumePoint(files[0]); got != 0 {
		t.Fatalf("fresh import resumes at %d, want 0", got)
	}

	// Only whole checkpoint steps are stored.
	noteImported(files[0], importCheckpointEvery-1_000, false)
	noteImported(files[0], 2*importCheckpointEvery, false)
	noteImported(files[0], 2*importCheckpointEvery+1_000, false)

	// The process ends here. A new run of the same import picks up.
	checkpoint = nil
	resumed := beginImport(files, time.Unix(2_000, 0))
	if !resumed.Equal(first) {
		t.Fatalf("resumed import started at %s, want the first start %s", resumed, first)
	}
	if got := importResumePoint(files[0]); got != 2*importCheckpointEvery {
		t.Fatalf("resume point = %d, want %d", got, 2*importCheckpointEvery)
	}
	if got := importResumePoint(files[1]); got != 0 {
		t.Fatalf("untouched file resumes at %d, want 0", got)
	}
}

func TestFinishedFileIsSkippedWhole(t *testing.T) { //nolint:paralleltest // shares the cache database.
	t.Cleanup(finishImport)
	files := listFiles("1.0.0")
	beginImport(files, time.Unix(1_000, 0))
	noteImported(files[0], 123_456, true)

	checkpoint = nil
	beginImport(files, time.Unix(2_000, 0))
	if got := importResumePoint(files[0]); got != 123_456 {
		t.Fatalf("finished file resumes at %d, want all 123456 records", got)
	}
}

func TestNewListVersionStartsOver(t *testing.T) { //nolint:paralleltest // shares the cache database.
	t.Cleanup(finishImport)
	beginImport(listFiles("1.0.0"), time.Unix(1_000, 0))
	noteImported(listFiles("1.0.0")[0], importCheckpointEvery, false)

	checkpoint = nil
	files := listFiles("2.0.0")
	now := time.Unix(5_000, 0)
	if started := beginImport(files, now); !started.Equal(now) {
		t.Fatalf("import of new lists started at %s, want %s", started, now)
	}
	if got := importResumePoint(files[0]); got != 0 {
		t.Fatalf("new lists resume at %d, want 0", got)
	}
}

func TestFinishedImportLeavesNoCheckpoint(t *testing.T) { //nolint:paralleltest // shares the cache database.
	files := listFiles("1.0.0")
	beginImport(files, time.Unix(1_000, 0))
	noteImported(files[0], importCheckpointEvery, false)
	finishImport()

	if cp := loadImportCheckpoint(); cp != nil {
		t.Fatalf("checkpoint left after the import finished: %+v", cp)
	}
	now := time.Unix(9_000, 0)
	if started := beginImport(files, now); !started.Equal(now) {
		t.Fatalf("next import started at %s, want %s", started, now)
	}
	finishImport()
}
