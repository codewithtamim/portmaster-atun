//go:build android

package filterlists

import (
	"reflect"
	"testing"
)

func TestAppendSecuritySourcesAddsMixedMalwareLists(t *testing.T) { //nolint:paralleltest // the package TestMain shares state.
	index := &ListIndexFile{
		Sources: []Source{
			{ID: "ads-and-malware", Category: "BAD", Name: "Ads and Malware Collection", Description: "Many ads and malware domains."},
			{ID: "nasty", Category: "BAD", Name: "Bad Domains", Description: "List of ad, malware, tracking and other nasty services."},
			{ID: "phish", Category: "BAD", Name: "Phishing Army", Description: ""},
			{ID: "general", Category: "BAD", Name: "List of Bad Stuff", Description: "Miscellaneous bad stuff."},
			{ID: "antimalwareish", Category: "TRAC", Name: "Antimalwareish Trackers", Description: "Trackers only."},
		},
	}

	got := appendSecuritySources([]string{"mal-1", "nasty"}, index)
	want := []string{"ads-and-malware", "mal-1", "nasty", "phish"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("security sources = %v, want %v", got, want)
	}
}
