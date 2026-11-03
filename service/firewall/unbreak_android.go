//go:build android

package firewall

import (
	"github.com/safing/portmaster/base/log"
	"github.com/safing/portmaster/service/intel/filterlists"
	"github.com/safing/portmaster/service/network"
)

// securityCategoryIDs are the catalog categories an unbreak match must never
// lift: malware, and deception (phishing, fraud). "BAD" is left out on
// purpose: it mixes trackers with malware, and lifting tracker overblocking
// is exactly what unbreak is for.
var securityCategoryIDs = []string{"MAL", "DECEP"}

// unbreakMayLift reports whether an unbreak match may cancel this
// connection's filter-list block. It may not when a malware or deception list
// also matched: unbreak keeps sites working that tracker lists break, and must
// not open a door that a security list closed.
//
// The index lookup runs only when an unbreak list matched, which is rare.
func unbreakMayLift(conn *network.Connection) bool {
	if conn.Entity == nil || len(conn.Entity.BlockedByLists) == 0 {
		return true
	}
	// Cheap in-memory check first: without an unbreak match there is nothing
	// to lift, and no index lookup is needed.
	if !blockedByAny(conn.Entity.BlockedByLists, filterlists.GetUnbreakFilterListIDs()) {
		return true
	}
	security, err := filterlists.ResolveListIDs(securityCategoryIDs)
	if err != nil {
		log.Warningf("filter: failed to resolve security filter lists: %s", err)
		// Failing closed keeps a malware block in place.
		return false
	}
	return !blockedByAny(conn.Entity.BlockedByLists, security)
}

func blockedByAny(blocked, ids []string) bool {
	for _, b := range blocked {
		for _, id := range ids {
			if b == id {
				return true
			}
		}
	}
	return false
}
