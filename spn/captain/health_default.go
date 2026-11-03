//go:build !android

package captain

import "github.com/safing/portmaster/spn/docks"

func homeLinkBytesIn(_ *docks.Crane) uint64 { return 0 }

// homeLinkBusy keeps the upstream behavior: a ping timeout reconnects.
func homeLinkBusy(_ *docks.Crane, _ uint64) bool { return false }
