//go:build android

package endpoints

import (
	"bufio"
	"net"
	"os"
	"strings"
	"testing"

	"github.com/safing/portmaster/service/intel"
)

// TestAndroidRuleVectors runs the shared rule vectors against the engine's
// matcher. The app's rule field runs the same file; see the file header.
func TestAndroidRuleVectors(t *testing.T) {
	t.Parallel()

	file, err := os.Open("testdata/android_rule_vectors.tsv")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()

	results := map[string]EPResult{"Denied": Denied, "Permitted": Permitted, "NoMatch": NoMatch}
	scanner := bufio.NewScanner(file)
	lines := 0
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 7 {
			t.Fatalf("vector %q has %d fields, want 7", line, len(fields))
		}
		rule, target, block, allow := fields[0], fields[4], fields[5], fields[6]
		lines++

		entity := &intel.Entity{}
		if ip := net.ParseIP(target); ip != nil {
			entity.IP = ip
		} else {
			entity.Domain = target + "."
		}
		entity.Init(0)

		for _, check := range []struct {
			prefix string
			want   string
		}{{"- ", block}, {"+ ", allow}} {
			ep, err := parseEndpoint(check.prefix + rule)
			if err != nil {
				t.Errorf("%s%s: parse failed: %s", check.prefix, rule, err)
				continue
			}
			want, ok := results[check.want]
			if !ok {
				t.Fatalf("vector %q: unknown result %q", line, check.want)
			}
			got, _ := ep.Matches(t.Context(), entity)
			if got != want {
				t.Errorf("%s%s against %s: got %s, want %s", check.prefix, rule, target, got, want)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if lines == 0 {
		t.Fatal("no vectors read")
	}
}
