//go:build !android

package filterlists

import (
	"time"

	"github.com/safing/portmaster/service/updates"
)

// beginImport returns now: desktop imports always start from the beginning.
func beginImport(_ []*updates.Artifact, now time.Time) time.Time { return now }

func importResumePoint(*updates.Artifact) int { return 0 }

func noteImported(*updates.Artifact, int, bool) {}

func finishImport() {}
