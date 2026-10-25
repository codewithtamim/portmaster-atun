//go:build !android

package endpoints

import (
	"testing"

	"github.com/safing/portmaster/service/intel"
)

func TestExactDomainStaysExact(t *testing.T) {
	t.Parallel()

	denied, err := parseEndpoint("- example.com.")
	if err != nil {
		t.Fatal(err)
	}
	testEndpointMatch(t, denied, (&intel.Entity{Domain: "example.com."}).Init(0), Denied)
	testEndpointMatch(t, denied, (&intel.Entity{Domain: "www.example.com."}).Init(0), NoMatch)
}
