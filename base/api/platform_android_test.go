//go:build android

package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDevModeNeverGrantsAccessOnAndroid(t *testing.T) {
	if devModeGrantsAccess() {
		t.Fatal("dev mode must not bypass authentication on Android")
	}
}

func TestPlatformListenAddressStaysOnLoopback(t *testing.T) {
	previous := defaultListenAddress
	t.Cleanup(func() { defaultListenAddress = previous })
	defaultListenAddress = "127.0.0.1:3008"

	for configured, want := range map[string]string{
		"127.0.0.1:3008": "127.0.0.1:3008",
		"[::1]:3008":     "[::1]:3008",
		"0.0.0.0:3008":   "127.0.0.1:3008",
		"192.168.1.5:80": "127.0.0.1:3008",
		"":               "127.0.0.1:3008",
	} {
		if got := platformListenAddress(configured); got != want {
			t.Errorf("platformListenAddress(%q) = %q, want %q", configured, got, want)
		}
	}

	// A non-loopback default must not become the fallback either.
	defaultListenAddress = ""
	if got := platformListenAddress("0.0.0.0:3008"); got != "127.0.0.1:3008" {
		t.Errorf("with empty default: got %q", got)
	}
}

func TestProcessAPIKey(t *testing.T) {
	t.Cleanup(func() { SetProcessAPIKey("") })

	if _, ok := platformAPIKey("anything"); ok {
		t.Fatal("matched with no process key set")
	}

	SetProcessAPIKey("secret-key")
	token, ok := platformAPIKey("secret-key")
	if !ok || token.Read != PermitAdmin || token.Write != PermitAdmin {
		t.Fatalf("got %+v, %v", token, ok)
	}
	token.Read = PermitSelf
	again, ok := platformAPIKey("secret-key")
	if !ok || again.Read != PermitAdmin {
		t.Fatal("a caller changed the shared token")
	}
	for _, wrong := range []string{"", "secret", "secret-key2", "SECRET-KEY"} {
		if _, ok := platformAPIKey(wrong); ok {
			t.Errorf("matched wrong key %q", wrong)
		}
	}

	SetProcessAPIKey("")
	if _, ok := platformAPIKey("secret-key"); ok {
		t.Fatal("still matched after clearing")
	}
}

func TestDebugEndpointsNeedUserOnAndroid(t *testing.T) {
	for _, path := range protectedDebugPaths {
		if got := readPermissionOf(t, path); got != PermitUser {
			t.Errorf("%s: read permission %s, want %s", path, got, PermitUser)
		}
	}
	for _, path := range []string{"ping", "ready"} {
		if got := readPermissionOf(t, path); got != PermitAnyone {
			t.Errorf("%s: read permission %s, want %s", path, got, PermitAnyone)
		}
	}
}

func TestPlatformHandlerChecksHost(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})

	for listen, hosts := range map[string]map[string]bool{
		"127.0.0.1:3008": {
			"127.0.0.1:3008":        true,
			"localhost:3008":        true,
			"LocalHost:3008":        true,
			"[::1]:3008":            true,
			"127.0.0.1:3009":        false,
			"127.0.0.1":             false,
			"localhost":             false,
			"":                      false,
			"evil.example:3008":     false,
			"127.0.0.1.nip.io:3008": false,
		},
		"[::1]:4000": {
			"[::1]:4000":     true,
			"127.0.0.1:4000": true,
			"localhost:4000": true,
			"127.0.0.1:3008": false,
		},
		"127.0.0.2:3008": {
			"127.0.0.2:3008": true,
			"127.0.0.1:3008": true,
			"127.0.0.3:3008": false,
		},
	} {
		h := platformHandler(next, listen)
		for host, ok := range hosts {
			r := httptest.NewRequest("GET", "/api/v1/ping", nil)
			r.Host = host
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if got := w.Code == http.StatusTeapot; got != ok {
				t.Errorf("listen %s, host %q: status %d, want allowed=%v", listen, host, w.Code, ok)
			}
		}
	}

	// The websocket bridge builds requests in process with a dummy Host.
	h := platformHandler(next, "127.0.0.1:3008")
	r := httptest.NewRequest("GET", "/api/v1/ping", nil)
	r.RemoteAddr = endpointBridgeRemoteAddress
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusTeapot {
		t.Errorf("bridged request: status %d", w.Code)
	}
}

func TestDebugEndpointNeedsKeyThroughServer(t *testing.T) {
	t.Cleanup(func() { SetProcessAPIKey("") })
	SetProcessAPIKey("process-key")
	h := platformHandler(&mainHandler{mux: mainMux}, "127.0.0.1:3008")

	get := func(path, host, key string) int {
		r := httptest.NewRequest("GET", apiV1Path+path, nil)
		r.Host = host
		if key != "" {
			r.Header.Set("Authorization", "Bearer "+key)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}

	if code := get("debug/stack", "127.0.0.1:3008", ""); code == http.StatusOK {
		t.Error("debug/stack readable without a key")
	}
	if code := get("debug/stack", "127.0.0.1:3008", "wrong-key"); code == http.StatusOK {
		t.Error("debug/stack readable with a wrong key")
	}
	if code := get("debug/stack", "127.0.0.1:3008", "process-key"); code != http.StatusOK {
		t.Errorf("debug/stack with the process key: status %d", code)
	}
	if code := get("debug/stack", "rebound.example:3008", "process-key"); code != http.StatusForbidden {
		t.Errorf("debug/stack with a foreign Host: status %d", code)
	}
	if code := get("ping", "localhost:3008", ""); code != http.StatusOK {
		t.Errorf("ping without a key: status %d", code)
	}
}
