//go:build android

package atun

import (
	"errors"
	"testing"
)

func TestNewTunConfig(t *testing.T) {
	cfg, err := NewTunConfig(42, 0, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.FD != 42 || cfg.MTU != defaultMTU ||
		cfg.Addr.String() != defaultTunAddr || cfg.FakeDNS.String() != defaultFakeDNSAddr {
		t.Fatalf("defaults not applied: %+v", cfg)
	}

	cfg, err = NewTunConfig(7, 1400, " 10.0.0.2 ", "::ffff:10.0.0.53")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MTU != 1400 || cfg.Addr.String() != "10.0.0.2" || cfg.FakeDNS.String() != "10.0.0.53" {
		t.Fatalf("parameters not kept: %+v", cfg)
	}

	for name, tc := range map[string]struct {
		fd, mtu   int
		addr, dns string
		want      error
	}{
		"negative fd":   {-1, 0, "", "", ErrInvalidFD},
		"negative mtu":  {1, -1, "", "", ErrInvalidMTU},
		"huge mtu":      {1, 70000, "", "", ErrInvalidMTU},
		"bad tun addr":  {1, 0, "nope", "", ErrInvalidTunAddr},
		"ipv6 tun addr": {1, 0, "fd00::1", "", ErrInvalidTunAddr},
		"bad fake dns":  {1, 0, "", "10.0.0", ErrInvalidDNSAddr},
	} {
		if _, err := NewTunConfig(tc.fd, tc.mtu, tc.addr, tc.dns); !errors.Is(err, tc.want) {
			t.Errorf("%s: got %v, want %v", name, err, tc.want)
		}
	}
}

func TestTunConfigStore(t *testing.T) {
	t.Cleanup(ClearTunConfig)

	if _, ok := CurrentTunConfig(); ok {
		t.Fatal("config present before SetTunConfig")
	}
	cfg, err := NewTunConfig(3, 1500, "", "")
	if err != nil {
		t.Fatal(err)
	}
	SetTunConfig(cfg)
	if got, ok := CurrentTunConfig(); !ok || got != cfg {
		t.Fatalf("got %+v, %v", got, ok)
	}
	ClearTunConfig()
	if _, ok := CurrentTunConfig(); ok {
		t.Fatal("config survives ClearTunConfig")
	}
}

func TestOpenWithoutConfig(t *testing.T) {
	ClearTunConfig()
	if err := Open(nil); !errors.Is(err, ErrNoProtector) && !errors.Is(err, ErrNoTunConfig) {
		t.Fatalf("got %v, want ErrNoProtector or ErrNoTunConfig", err)
	}
	if Running() {
		t.Fatal("running without a session")
	}
}
