//go:build android

package profile

import (
	"net"
	"testing"

	"github.com/safing/portmaster/service/intel"
	"github.com/safing/portmaster/service/profile/endpoints"
)

func layerWithRules(t *testing.T, id string, rules ...string) *Profile {
	t.Helper()
	parsed, err := endpoints.ParseEndpoints(rules)
	if err != nil {
		t.Fatal(err)
	}
	layer := &Profile{ID: id, Source: SourceLocal, endpoints: parsed}
	layer.SetKey(MakeProfileKey(layer.Source, layer.ID))
	return layer
}

func TestEndpointRuleSourceNamesTheDecidingLayer(t *testing.T) {
	t.Parallel()

	app := layerWithRules(t, "app", "- tracker.example.com")
	lp := &LayeredProfile{layers: []*Profile{app}}

	own := &intel.Entity{Domain: "tracker.example.com."}
	own.Init(0)
	if got := lp.EndpointRuleSource(t.Context(), own, false); got != app.Key() {
		t.Fatalf("app rule decided, got source %q, want %q", got, app.Key())
	}

	other := &intel.Entity{Domain: "ads.example.net."}
	other.Init(0)
	global := MakeProfileKey(SourceSpecial, globalConfigProfileID)
	if got := lp.EndpointRuleSource(t.Context(), other, false); got != global {
		t.Fatalf("no app rule matched, got source %q, want %q", got, global)
	}
}

func TestEndpointRuleSourceUsesInboundRulesForInbound(t *testing.T) {
	t.Parallel()

	app := &Profile{ID: "app", Source: SourceLocal}
	app.SetKey(MakeProfileKey(app.Source, app.ID))
	parsed, err := endpoints.ParseEndpoints([]string{"+ 192.168.1.10"})
	if err != nil {
		t.Fatal(err)
	}
	app.serviceEndpoints = parsed
	lp := &LayeredProfile{layers: []*Profile{app}}

	peer := &intel.Entity{IP: net.ParseIP("192.168.1.10")}
	peer.Init(0)
	if got := lp.EndpointRuleSource(t.Context(), peer, true); got != app.Key() {
		t.Fatalf("inbound rule decided, got source %q, want %q", got, app.Key())
	}
	if got := lp.EndpointRuleSource(t.Context(), peer, false); got != MakeProfileKey(SourceSpecial, globalConfigProfileID) {
		t.Fatalf("outbound has no app rule, got source %q, want the global profile", got)
	}
}
