//go:build android

package filterlists

import (
	"fmt"
	"strings"
)

// supportedSchemaMajor is the list index format this build reads.
const supportedSchemaMajor = "v2"

// checkListIndexSchema refuses a list index in a format this build does not
// read. The lists already in the cache stay in use, so filtering keeps working
// on the last good index instead of on one parsed into the wrong shape.
// Any minor or patch release of the same major version is accepted.
func checkListIndexSchema(index *ListIndexFile) error {
	major, _, _ := strings.Cut(strings.TrimSpace(index.SchemaVersion), ".")
	if major != supportedSchemaMajor {
		return fmt.Errorf("intel/filterlists: list index schema %q is not supported, need %s.x", index.SchemaVersion, supportedSchemaMajor)
	}
	return nil
}
