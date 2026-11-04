//go:build !android

package netquery

import "context"

func compactLiveDB(context.Context, *Database) {}
