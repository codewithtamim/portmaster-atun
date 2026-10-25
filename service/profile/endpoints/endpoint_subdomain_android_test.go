//go:build android

package endpoints

import (
	"testing"

	"github.com/safing/portmaster/service/intel"
)

func TestDeniedExactDomainMatchesSubdomains(t *testing.T) {
	t.Parallel()

	denied, err := parseEndpoint("- example.com.")
	if err != nil {
		t.Fatal(err)
	}
	testEndpointMatch(t, denied, (&intel.Entity{Domain: "example.com."}).Init(0), Denied)
	testEndpointMatch(t, denied, (&intel.Entity{Domain: "www.example.com."}).Init(0), Denied)

	allowed, err := parseEndpoint("+ example.com.")
	if err != nil {
		t.Fatal(err)
	}
	testEndpointMatch(t, allowed, (&intel.Entity{Domain: "example.com."}).Init(0), Permitted)
	testEndpointMatch(t, allowed, (&intel.Entity{Domain: "www.example.com."}).Init(0), NoMatch)

	wildcard, err := parseEndpoint("- *.example.com.")
	if err != nil {
		t.Fatal(err)
	}
	testEndpointMatch(t, wildcard, (&intel.Entity{Domain: "www.example.com."}).Init(0), Denied)
	testEndpointMatch(t, wildcard, (&intel.Entity{Domain: "example.com."}).Init(0), NoMatch)
}
