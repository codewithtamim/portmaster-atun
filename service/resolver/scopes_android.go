//go:build android

package resolver

// ianaDocumentationDomains are delegated in the public DNS. Android has no
// local resolver, so treating them as special-use blocks them with
// ErrNoCompliance. Other platforms keep the local-resolver path.
var ianaDocumentationDomains = []string{
	".example.com.",
	".example.net.",
	".example.org.",
}

func documentationNamesResolvedGlobally(dotPrefixedFQDN string) bool {
	return domainInScope(dotPrefixedFQDN, ianaDocumentationDomains)
}
