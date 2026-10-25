//go:build android

package endpoints

import "strings"

// deniedExactMatchesSubdomain is the Android reading of a blocked domain.
// A denied exact domain also covers its subdomains. An allow stays exact,
// and a *.example.com suffix still does not include the apex.
func deniedExactMatchesSubdomain(ep *EndpointDomain, domain string) bool {
	if ep.Permitted {
		return false
	}
	return strings.HasSuffix(domain, "."+ep.Domain)
}
