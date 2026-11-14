//go:build android

package filterlists

import "testing"

func TestCheckListIndexSchema(t *testing.T) {
	for schema, ok := range map[string]bool{
		"v2.0.0": true,
		"v2.3.1": true,
		" v2.0 ": true,
		"v2":     true,
		"":       false,
		"v1.9.0": false,
		"v3.0.0": false,
		"2.0.0":  false,
	} {
		err := checkListIndexSchema(&ListIndexFile{SchemaVersion: schema})
		if (err == nil) != ok {
			t.Errorf("schema %q: got %v, want accepted=%v", schema, err, ok)
		}
	}
}
