//go:build android

package filterlists

import (
	"errors"
	"sync"
	"time"

	"github.com/safing/portmaster/base/database"
	"github.com/safing/portmaster/base/database/record"
	"github.com/safing/portmaster/base/log"
	"github.com/safing/portmaster/service/updates"
)

// importCheckpointKey holds how far an unfinished import got. It sits outside
// the list prefix, so the cleanup of old list entries never removes it.
const importCheckpointKey = cacheDBPrefix + "/importCheckpoint"

// importCheckpointEvery is how many records are written between checkpoints.
// Each checkpoint syncs the cache database, so this trades a few seconds of
// redone work after a kill against a sync every few seconds.
const importCheckpointEvery = 50_000

// importFileProgress is how many records of one list file are in the cache.
type importFileProgress struct {
	Version   string
	Persisted int
}

// importCheckpoint lets an import that Android ended part way continue where
// it stopped instead of starting over. The full import of millions of records
// takes minutes on a phone, and the system may end the process at any time.
type importCheckpoint struct {
	record.Base
	sync.Mutex

	// StartedAt is when the interrupted import began. The resumed import keeps
	// it, so the cleanup of old entries does not remove the ones written
	// before the interruption.
	StartedAt int64
	Files     map[string]importFileProgress
}

var (
	checkpointLock sync.Mutex
	checkpoint     *importCheckpoint
)

func loadImportCheckpoint() *importCheckpoint {
	r, err := cache.Get(importCheckpointKey)
	if err != nil {
		if !errors.Is(err, database.ErrNotFound) {
			log.Warningf("intel/filterlists: failed to read import checkpoint: %s", err)
		}
		return nil
	}
	// The database's in-memory cache can still hand out the record that
	// finishImport deleted. Writing to it again would delete the new
	// checkpoint as well.
	if r.Meta() != nil && r.Meta().IsDeleted() {
		return nil
	}
	cp := new(importCheckpoint)
	if r.IsWrapped() {
		if err := record.Unwrap(r, cp); err != nil {
			log.Warningf("intel/filterlists: failed to read import checkpoint: %s", err)
			return nil
		}
	} else {
		stored, ok := r.(*importCheckpoint)
		if !ok {
			return nil
		}
		cp = stored
	}
	return cp
}

// beginImport returns when the import of files began. A checkpoint left by an
// interrupted import of the same files is kept and its start time returned.
// Any other checkpoint is dropped.
func beginImport(files []*updates.Artifact, now time.Time) time.Time {
	checkpointLock.Lock()
	defer checkpointLock.Unlock()

	cp := loadImportCheckpoint()
	if cp != nil && !checkpointMatches(cp, files) {
		cp = nil
	}
	if cp == nil {
		cp = &importCheckpoint{
			StartedAt: now.Unix(),
			Files:     make(map[string]importFileProgress),
		}
		cp.SetKey(importCheckpointKey)
	} else {
		log.Infof("intel/filterlists: resuming an interrupted import that began %s", time.Unix(cp.StartedAt, 0))
	}
	checkpoint = cp
	return time.Unix(cp.StartedAt, 0)
}

// checkpointMatches reports whether every file the checkpoint has progress
// for is still part of this import, at the same version.
func checkpointMatches(cp *importCheckpoint, files []*updates.Artifact) bool {
	if len(cp.Files) == 0 {
		return false
	}
	versions := make(map[string]string, len(files))
	for _, file := range files {
		versions[file.Filename] = file.Version
	}
	for name, progress := range cp.Files {
		if versions[name] != progress.Version {
			return false
		}
	}
	return true
}

// importResumePoint returns how many records of file are already in the
// cache from an interrupted import. Those records are decoded again, so the
// bloom filter is complete, but not written again.
func importResumePoint(file *updates.Artifact) int {
	checkpointLock.Lock()
	defer checkpointLock.Unlock()

	if checkpoint == nil {
		return 0
	}
	progress, ok := checkpoint.Files[file.Filename]
	if !ok || progress.Version != file.Version {
		return 0
	}
	log.Infof("intel/filterlists: %s already has %d records in the cache, writing the rest", file.Filename, progress.Persisted)
	return progress.Persisted
}

// noteImported records that the first persisted records of file are written.
// Only every importCheckpointEvery records is the checkpoint stored, and the
// sync that stores it also makes those records durable.
func noteImported(file *updates.Artifact, persisted int, final bool) {
	if !final && persisted%importCheckpointEvery != 0 {
		return
	}

	checkpointLock.Lock()
	defer checkpointLock.Unlock()

	if checkpoint == nil {
		return
	}
	checkpoint.Files[file.Filename] = importFileProgress{
		Version:   file.Version,
		Persisted: persisted,
	}
	if err := cache.Put(checkpoint); err != nil {
		log.Warningf("intel/filterlists: failed to save import checkpoint: %s", err)
		return
	}
	if err := persistCache(); err != nil {
		log.Warningf("intel/filterlists: failed to sync import checkpoint: %s", err)
	}
}

// finishImport drops the checkpoint once the import is complete and the
// version record says so.
func finishImport() {
	checkpointLock.Lock()
	defer checkpointLock.Unlock()

	checkpoint = nil
	err := cache.Delete(importCheckpointKey)
	if err != nil && !errors.Is(err, database.ErrNotFound) {
		log.Warningf("intel/filterlists: failed to remove import checkpoint: %s", err)
	}
}
