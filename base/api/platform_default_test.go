//go:build !android

package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDebugEndpointsKeepUpstreamPermission(t *testing.T) {
	for _, path := range append([]string{"ping", "ready"}, protectedDebugPaths...) {
		if got := readPermissionOf(t, path); got != PermitAnyone {
			t.Errorf("%s: read permission %s, want %s", path, got, PermitAnyone)
		}
	}
}

func TestPlatformHandlerAcceptsAnyHost(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	h := platformHandler(next, "127.0.0.1:817")
	for _, host := range []string{"127.0.0.1:817", "example.com", ""} {
		r := httptest.NewRequest("GET", "/api/v1/ping", nil)
		r.Host = host
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusTeapot {
			t.Errorf("host %q: status %d", host, w.Code)
		}
	}
}
