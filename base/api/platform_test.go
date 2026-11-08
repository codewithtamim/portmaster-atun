package api

import (
	"net/http/httptest"
	"testing"
)

// readPermissionOf returns the read permission of an endpoint registered by New.
func readPermissionOf(t *testing.T, path string) Permission {
	t.Helper()
	e, err := GetEndpointByPath(path)
	if err != nil {
		t.Fatal(err)
	}
	return e.Read
}

var protectedDebugPaths = []string{
	"debug/stack",
	"debug/stack/print",
	"debug/cpu",
	"debug/heap",
	"debug/allocs",
	"debug/info",
}

func TestKeyPrefix(t *testing.T) {
	for key, want := range map[string]string{
		"":         "",
		"a":        "a",
		"abc":      "abc",
		"abcd":     "abcd",
		"abcdefgh": "abcd",
		"äbcdefgh": "äbc",
	} {
		if got := keyPrefix(key); got != want {
			t.Errorf("keyPrefix(%q) = %q, want %q", key, got, want)
		}
	}
}

func TestCheckAPIKeyShortKeys(t *testing.T) {
	for _, header := range []string{
		"Bearer ",
		"Bearer a",
		"Bearer abc",
		"Basic Og==", // empty user and password
		"Basic YTo=", // "a:"
	} {
		r := httptest.NewRequest("GET", "/api/v1/ping", nil)
		r.Header.Set("Authorization", header)
		if token := checkAPIKey(r); token != nil {
			t.Errorf("%q: got token %+v", header, token)
		}
	}
}
