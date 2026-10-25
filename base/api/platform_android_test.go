//go:build android

package api

import "testing"

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
