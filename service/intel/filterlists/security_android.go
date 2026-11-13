//go:build android

package filterlists

import (
	"regexp"
	"sort"
)

// securityWords mark a list source that blocks malware or phishing, whatever
// category it is filed under. Some mixed lists (ads and malware together) sit
// in a general category, and an unbreak match must not lift their blocks.
// Whole words only, so names that merely contain a word stay unaffected.
var securityWords = regexp.MustCompile(`(?i)\b(malware|malicious|phishing)\b`)

// ResolveSecurityListIDs returns the source IDs of categoryIDs, plus every
// source whose name or description says it blocks malware or phishing.
func ResolveSecurityListIDs(categoryIDs []string) ([]string, error) {
	ids, err := ResolveListIDs(categoryIDs)
	if err != nil {
		return nil, err
	}
	index, err := getListIndexFromCache()
	if err != nil {
		return nil, err
	}
	return appendSecuritySources(ids, index), nil
}

// appendSecuritySources adds the sources of index that describe themselves as
// blocking malware or phishing to ids, without duplicates, sorted.
func appendSecuritySources(ids []string, index *ListIndexFile) []string {
	index.RLock()
	defer index.RUnlock()

	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	for _, source := range index.Sources {
		if securityWords.MatchString(source.Name) || securityWords.MatchString(source.Description) {
			set[source.ID] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}
