//go:build android

package wireguard

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/netip"
	"strings"
)

// Profile limits and defaults.
const (
	// profileSchema is the version of the JSON the app sends. Bump it only for a
	// breaking change; the app and the core ship together, so this exists to fail
	// loudly on a stale AAR rather than to support old shapes.
	profileSchema = 1

	// The tunnel MTU is computed by the engine, never taken from the config;
	// see tunnelMTU. minMTU is the IPv6 minimum, which the
	// tunnel carries. maxTunnelMTU is wg-quick's 1420: a 1500 byte link minus
	// wireGuardOverhead, the worst case of WireGuard, UDP and IPv6 headers.
	minMTU            = 1280
	maxTunnelMTU      = 1420
	wireGuardOverhead = 80

	// defaultKeepalive is installed when a profile has none.
	defaultKeepalive = 25

	keyLen = 32
)

// Profile is an installed WireGuard configuration.
//
// It is decoded from JSON handed over by the app, which parsed and validated the
// wg-quick file first. The core deliberately has no wg-quick parser of its
// own: one grammar, one implementation.
type Profile struct {
	Schema     int            `json:"schema"`
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	PrivateKey string         `json:"privateKey"`
	Addresses  []netip.Prefix `json:"addresses"`
	// MTU is the config file's value. It is accepted and ignored: the engine
	// computes the tunnel MTU. Kept so the app's document,
	// decoded with DisallowUnknownFields, still matches.
	MTU   int    `json:"mtu"`
	Peers []Peer `json:"peers"`

	// capturesV4 and capturesV6 are derived by Validate. See Captures.
	capturesV4 bool
	capturesV6 bool
}

// Peer is one WireGuard peer.
type Peer struct {
	PublicKey    string         `json:"publicKey"`
	PresharedKey string         `json:"presharedKey"`
	AllowedIPs   []netip.Prefix `json:"allowedIps"`
	EndpointHost string         `json:"endpointHost"`
	EndpointPort uint16         `json:"endpointPort"`
	Keepalive    uint16         `json:"keepalive"`
}

// ParseProfile decodes and validates a profile document.
func ParseProfile(doc string) (*Profile, error) {
	var p Profile
	dec := json.NewDecoder(strings.NewReader(doc))
	dec.DisallowUnknownFields()
	err := dec.Decode(&p)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBadProfile, err)
	}
	err = p.Validate()
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// Validate checks the profile and fills in the derived fields. It is the only
// place that decides whether a profile can carry traffic.
func (p *Profile) Validate() error {
	if p.Schema != profileSchema {
		return fmt.Errorf("%w: schema %d, want %d", ErrBadProfile, p.Schema, profileSchema)
	}
	err := checkKey("private key", p.PrivateKey)
	if err != nil {
		return err
	}
	if len(p.Addresses) == 0 {
		return fmt.Errorf("%w: no interface address", ErrBadProfile)
	}
	for _, addr := range p.Addresses {
		if !addr.IsValid() {
			return fmt.Errorf("%w: invalid interface address", ErrBadProfile)
		}
	}
	if len(p.Peers) == 0 {
		return fmt.Errorf("%w: no peers", ErrBadProfile)
	}

	allowed, err := p.validatePeers()
	if err != nil {
		return err
	}

	// A family is captured only when the tunnel can both source it and route it.
	// Sending a family the peers do not cover into the device would be a
	// blackhole: wireguard-go drops it by cryptokey routing, silently.
	// A family without a literal default route is not captured. That is "no
	// IPv4" or "no IPv6", not a reason to reject the profile. Traffic for an
	// uncaptured family is blocked. Allowed IPs that are not /0 do not widen
	// capture to the whole family.
	allowsV4, allowsV6 := coversFamily(allowed, true), coversFamily(allowed, false)
	if !allowsV4 && !allowsV6 {
		// Narrower than a full tunnel is split tunneling, which is not supported.
		return fmt.Errorf("%w: %w", ErrBadProfile, ErrSplitTunnel)
	}
	p.capturesV4 = allowsV4 && p.hasAddressFamily(false)
	p.capturesV6 = allowsV6 && p.hasAddressFamily(true)
	if !p.capturesV4 && !p.capturesV6 {
		return fmt.Errorf("%w: no interface address in a routed family", ErrBadProfile)
	}

	return nil
}

// validatePeers checks every peer and fills in derived fields. It returns the
// allowed-IP prefixes accumulated across peers.
func (p *Profile) validatePeers() ([]netip.Prefix, error) {
	var allowed []netip.Prefix
	for i := range p.Peers {
		peer := &p.Peers[i]
		err := checkKey("public key", peer.PublicKey)
		if err != nil {
			return nil, err
		}
		if peer.PresharedKey != "" {
			err = checkKey("preshared key", peer.PresharedKey)
			if err != nil {
				return nil, err
			}
		}
		if peer.EndpointHost == "" || peer.EndpointPort == 0 {
			return nil, fmt.Errorf("%w: peer has no endpoint", ErrBadProfile)
		}
		if len(peer.AllowedIPs) == 0 {
			return nil, fmt.Errorf("%w: peer has no allowed ips", ErrBadProfile)
		}
		for _, prefix := range peer.AllowedIPs {
			if !prefix.IsValid() {
				return nil, fmt.Errorf("%w: invalid allowed ip", ErrBadProfile)
			}
		}
		allowed = append(allowed, peer.AllowedIPs...)
		if peer.Keepalive == 0 {
			peer.Keepalive = defaultKeepalive
		}
	}

	return allowed, nil
}

// coversFamily reports whether prefixes route a whole address family a /0,
// or the 0.0.0.0/1 + 128.0.0.0/1 pair some providers use instead. The app's
// ProfileCodec.capturesDefaultRoute applies the same rule; keep them in step.
func coversFamily(prefixes []netip.Prefix, v4 bool) bool {
	var lowHalf, highHalf bool
	for _, prefix := range prefixes {
		if prefix.Addr().Is4() != v4 {
			continue
		}
		switch prefix.Bits() {
		case 0:
			return true
		case 1:
			if prefix.Addr().AsSlice()[0]&0x80 == 0 {
				lowHalf = true
			} else {
				highHalf = true
			}
		}
	}
	return lowHalf && highHalf
}

// Captures reports which address families the tunnel can carry. Traffic of a
// family it cannot carry is blocked rather than sent out unprotected.
func (p *Profile) Captures() (v4, v6 bool) {
	return p.capturesV4, p.capturesV6
}

// InterfaceAddrs returns the addresses for the userspace network stack.
func (p *Profile) InterfaceAddrs() []netip.Addr {
	addrs := make([]netip.Addr, 0, len(p.Addresses))
	for _, prefix := range p.Addresses {
		addrs = append(addrs, prefix.Addr())
	}
	return addrs
}

// UAPI renders the profile in the WireGuard cross-platform userspace API format.
//
// resolved maps a peer's index to its resolved endpoint. A peer with no entry is
// still configured, just without an endpoint.
func (p *Profile) UAPI(resolved map[int]netip.AddrPort) string {
	var sb strings.Builder

	sb.WriteString("private_key=")
	sb.WriteString(mustHex(p.PrivateKey))
	sb.WriteString("\nreplace_peers=true\n")

	for i, peer := range p.Peers {
		// public_key starts a new peer; order matters to wireguard-go.
		sb.WriteString("public_key=")
		sb.WriteString(mustHex(peer.PublicKey))
		sb.WriteByte('\n')

		for _, allowed := range peer.AllowedIPs {
			sb.WriteString("allowed_ip=")
			sb.WriteString(allowed.String())
			sb.WriteByte('\n')
		}
		if ap, ok := resolved[i]; ok {
			sb.WriteString("endpoint=")
			sb.WriteString(ap.String())
			sb.WriteByte('\n')
		}
		if peer.Keepalive > 0 {
			fmt.Fprintf(&sb, "persistent_keepalive_interval=%d\n", peer.Keepalive)
		}
		if peer.PresharedKey != "" {
			sb.WriteString("preshared_key=")
			sb.WriteString(mustHex(peer.PresharedKey))
			sb.WriteByte('\n')
		}
	}

	return sb.String()
}

// EndpointUAPI updates peer endpoints without replacing peers. replace_peers
// drops keypairs and queued packets, which is what a network change must not do.
func (p *Profile) EndpointUAPI(resolved map[int]netip.AddrPort) string {
	var sb strings.Builder
	for i, peer := range p.Peers {
		ap, ok := resolved[i]
		if !ok {
			continue
		}
		sb.WriteString("public_key=")
		sb.WriteString(mustHex(peer.PublicKey))
		sb.WriteByte('\n')
		sb.WriteString("update_only=true\n")
		sb.WriteString("endpoint=")
		sb.WriteString(ap.String())
		sb.WriteByte('\n')
	}
	return sb.String()
}

// Meta is the part of a profile that is safe to show and to publish.
type Meta struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	CapturesV4 bool   `json:"capturesV4"`
	CapturesV6 bool   `json:"capturesV6"`
}

// Meta returns the non-secret description of the profile.
func (p *Profile) Meta() Meta {
	return Meta{
		ID:         p.ID,
		Name:       p.Name,
		CapturesV4: p.capturesV4,
		CapturesV6: p.capturesV6,
	}
}

func (p *Profile) hasAddressFamily(v6 bool) bool {
	for _, addr := range p.Addresses {
		if addr.Addr().Is4() == !v6 {
			return true
		}
	}
	return false
}

func checkKey(what, encoded string) error {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return fmt.Errorf("%w: %s is not base64", ErrBadProfile, what)
	}
	if len(raw) != keyLen {
		return fmt.Errorf("%w: %s is %d bytes, want %d", ErrBadProfile, what, len(raw), keyLen)
	}
	return nil
}

// mustHex converts a base64 key to hex. Validate has already checked the key, so
// a failure here cannot happen; an empty string would be rejected by wireguard-go
// rather than silently configuring a zero key.
func mustHex(encoded string) string {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(raw) != keyLen {
		return ""
	}
	return hex.EncodeToString(raw)
}
