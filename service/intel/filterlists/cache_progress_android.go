//go:build android

package filterlists

import (
	"io"
	"os"
	"sync/atomic"
	"time"

	"github.com/safing/portmaster/service/updates"
)

// cacheBytesRead is how many bytes of the current import have been pulled
// from the list files. The decode step fills a small channel, then waits
// on the database writer, so the count stays with the cache load.
var cacheBytesRead atomic.Int64

func beginCacheLoad(files []*updates.Artifact) {
	var total int64
	for _, file := range files {
		if file == nil {
			continue
		}
		info, err := os.Stat(file.Path())
		if err != nil {
			continue
		}
		total += info.Size()
	}
	cacheBytesRead.Store(0)
	updater := intelUpdater()
	if updater == nil {
		return
	}
	updater.BeginFilterListCache(total)
}

func endCacheLoad() {
	updater := intelUpdater()
	if updater == nil {
		return
	}
	updater.EndFilterListCache()
}

func noteCacheBytes(n int64) {
	if n <= 0 {
		return
	}
	done := cacheBytesRead.Add(n)
	updater := intelUpdater()
	if updater == nil {
		return
	}
	updater.NoteFilterListCache(done)
}

func intelUpdater() *updates.Updater {
	if module == nil || module.instance == nil {
		return nil
	}
	return module.instance.IntelUpdates()
}

// cacheProgressReader counts bytes as the list file is decoded.
type cacheProgressReader struct {
	r io.Reader
}

func (c cacheProgressReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if n > 0 {
		noteCacheBytes(int64(n))
	}
	return n, err
}

// countCacheBytes wraps a list file so its import is counted.
func countCacheBytes(r io.Reader) io.Reader {
	return cacheProgressReader{r: r}
}

// cacheUpdatedAt records when the lists were applied, for the UI.
func cacheUpdatedAt(now time.Time) int64 {
	return now.Unix()
}
