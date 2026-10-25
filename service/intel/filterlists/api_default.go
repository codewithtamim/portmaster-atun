//go:build !android

package filterlists

import (
	"io"
	"time"

	"github.com/safing/portmaster/service/updates"
)

func startPlatform() error { return nil }

func beginCacheLoad([]*updates.Artifact) {}

func endCacheLoad() {}

// countCacheBytes returns r unchanged: import progress is Android only.
func countCacheBytes(r io.Reader) io.Reader { return r }

// cacheUpdatedAt is zero, which leaves LastUpdatedUnix out of the record.
func cacheUpdatedAt(time.Time) int64 { return 0 }
