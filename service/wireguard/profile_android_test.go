//go:build android

package wireguard

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"testing"
)

func testKey(fill byte) string {
	b := make([]byte, keyLen)
	for i := range b {
		b[i] = fill
	}
	return base64.StdEncoding.EncodeToString(b)
}

// validProfile returns a document that must parse, so each test can vary one
// field and assert on that field alone.
func validProfile(replace ...string) string {
	doc := `{
		"schema": 1,
		"id": "p1",
		"name": "Test",
		"privateKey": "` + testKey(1) + `",
		"addresses": ["10.0.0.2/32", "fd00::2/128"],
		"peers": [{
			"publicKey": "` + testKey(2) + `",
			"allowedIps": ["0.0.0.0/0", "::/0"],
			"endpointHost": "vpn.example.com",
			"endpointPort": 51820
		}]
	}`
	for i := 0; i+1 < len(replace); i += 2 {
		doc = strings.Replace(doc, replace[i], replace[i+1], 1)
	}
	return doc
}

func TestParseProfile(t *testing.T) {
	t.Parallel()

	p, err := ParseProfile(validProfile())
	if err != nil {
		t.Fatalf("ParseProfile: %s", err)
	}

	if got := p.Peers[0].Keepalive; got != 0 {
		t.Errorf("Keepalive = %d, want 0: a profile without one keeps it off", got)
	}
	if strings.Contains(p.UAPI(nil), "persistent_keepalive_interval") {
		t.Error("uapi sets a keepalive the profile does not have")
	}

	set, err := ParseProfile(validProfile(`"endpointPort": 51820`, `"endpointPort": 51820, "keepalive": 25`))
	if err != nil {
		t.Fatalf("ParseProfile with keepalive: %s", err)
	}
	if got := set.Peers[0].Keepalive; got != 25 {
		t.Errorf("Keepalive = %d, want the profile's 25", got)
	}

	v4, v6 := p.Captures()
	if !v4 || !v6 {
		t.Errorf("Captures() = (%t, %t), want both true", v4, v6)
	}
	if got := len(p.InterfaceAddrs()); got != 2 {
		t.Errorf("len(InterfaceAddrs()) = %d, want 2", got)
	}
}

func TestParseProfileRejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		doc  string
	}{
		{"not json", "{"},
		{"unknown field", strings.Replace(validProfile(), `"id": "p1"`, `"id": "p1", "surprise": 1`, 1)},
		{"wrong schema", validProfile(`"schema": 1`, `"schema": 2`)},
		{"short private key", validProfile(testKey(1), base64.StdEncoding.EncodeToString([]byte("short")))},
		{"no addresses", validProfile(`["10.0.0.2/32", "fd00::2/128"]`, `[]`)},
		{"no peers", validProfile(`"peers": [{`, `"peers2": [{`)},
		{"no endpoint port", validProfile(`"endpointPort": 51820`, `"endpointPort": 0`)},
		{"no allowed ips", validProfile(`["0.0.0.0/0", "::/0"]`, `[]`)},
		{"no default route", validProfile(`["0.0.0.0/0", "::/0"]`, `["10.1.0.0/16"]`)},
		{"single half route", validProfile(`["0.0.0.0/0", "::/0"]`, `["0.0.0.0/1"]`)},
		{"route without address", validProfile(`["10.0.0.2/32", "fd00::2/128"]`, `["10.0.0.2/32"]`, `["0.0.0.0/0", "::/0"]`, `["::/0"]`)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := ParseProfile(tc.doc); err == nil {
				t.Error("ParseProfile accepted an invalid profile")
			}
		})
	}
}

func TestParseProfileErrorIsBadProfile(t *testing.T) {
	t.Parallel()

	_, err := ParseProfile(validProfile(`"schema": 1`, `"schema": 99`))
	if !errors.Is(err, ErrBadProfile) {
		t.Errorf("err = %v, want it to wrap ErrBadProfile", err)
	}
}

// A tunnel that can only source IPv4 must not claim to capture IPv6, or that
// traffic would be dropped inside the device instead of blocked by the firewall.
func TestParseProfileCapturesNeedsBothSourceAndRoute(t *testing.T) {
	t.Parallel()

	p, err := ParseProfile(validProfile(`["10.0.0.2/32", "fd00::2/128"]`, `["10.0.0.2/32"]`))
	if err != nil {
		t.Fatalf("ParseProfile: %s", err)
	}

	v4, v6 := p.Captures()
	if !v4 {
		t.Error("Captures() v4 = false, want true")
	}
	if v6 {
		t.Error("Captures() v6 = true, but the profile has no IPv6 interface address")
	}
}

// A config's MTU entry never overrides the computed tunnel MTU, so no
// value of it makes a profile invalid.
func TestParseProfileIgnoresMTU(t *testing.T) {
	t.Parallel()

	for _, mtu := range []int{0, 100, 1420, 9000} {
		doc := validProfile(`"id": "p1"`, fmt.Sprintf(`"id": "p1", "mtu": %d`, mtu))
		if _, err := ParseProfile(doc); err != nil {
			t.Errorf("mtu %d: %s", mtu, err)
		}
	}
}

// Narrower than a full tunnel is split tunneling, refused with a reason
// the app can show.
func TestParseProfileSplitTunnelError(t *testing.T) {
	t.Parallel()

	_, err := ParseProfile(validProfile(`["0.0.0.0/0", "::/0"]`, `["10.1.0.0/16", "fd00:1::/64"]`))
	if !errors.Is(err, ErrSplitTunnel) || !errors.Is(err, ErrBadProfile) {
		t.Fatalf("err = %v, want ErrSplitTunnel wrapped in ErrBadProfile", err)
	}
}

func TestCoversFamily(t *testing.T) {
	t.Parallel()

	prefixes := func(ss ...string) []netip.Prefix {
		out := make([]netip.Prefix, 0, len(ss))
		for _, s := range ss {
			out = append(out, netip.MustParsePrefix(s))
		}
		return out
	}
	for _, tc := range []struct {
		name   string
		in     []netip.Prefix
		v4, v6 bool
	}{
		{"both defaults", prefixes("0.0.0.0/0", "::/0"), true, true},
		{"v4 half pair", prefixes("0.0.0.0/1", "128.0.0.0/1"), true, false},
		{"v6 half pair", prefixes("::/1", "8000::/1"), false, true},
		{"single half", prefixes("0.0.0.0/1"), false, false},
		{"narrow", prefixes("10.0.0.0/8", "fd00::/8"), false, false},
		{"default plus extra", prefixes("10.0.0.0/8", "::/0"), false, true},
	} {
		if got := coversFamily(tc.in, true); got != tc.v4 {
			t.Errorf("%s: v4 = %v, want %v", tc.name, got, tc.v4)
		}
		if got := coversFamily(tc.in, false); got != tc.v6 {
			t.Errorf("%s: v6 = %v, want %v", tc.name, got, tc.v6)
		}
	}
}
