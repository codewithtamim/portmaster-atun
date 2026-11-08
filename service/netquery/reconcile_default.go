//go:build !android

package netquery

import "context"

// reconcileActive does nothing off Android, as upstream.
func reconcileActive(context.Context, *Database) {}

// repairStaleHistory does nothing off Android, as upstream.
func repairStaleHistory(context.Context, *Database) {}
