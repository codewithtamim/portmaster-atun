package crew

import (
	"fmt"
	"sync"
	"time"

	"github.com/safing/portmaster/base/log"
	"github.com/safing/portmaster/service/mgr"
	"github.com/safing/portmaster/service/network"
	"github.com/safing/portmaster/service/network/packet"
	"github.com/safing/portmaster/spn/navigator"
)

const (
	stickyTTL = 1 * time.Hour
)

var (
	stickyIPs     = make(map[string]*stickyHub)
	stickyDomains = make(map[string]*stickyHub)
	stickyLock    sync.Mutex
)

type stickyHub struct {
	Pin      *navigator.Pin
	Route    *navigator.Route
	LastSeen time.Time
	Avoid    bool
	// RequestedProfile is the hop profile selected when this route was built,
	// captured before route finding rewrites it. Only Android reads it.
	RequestedProfile string
}

func (sh *stickyHub) isExpired() bool {
	return time.Now().Add(-stickyTTL).After(sh.LastSeen)
}

func makeStickyIPKey(conn *network.Connection) string {
	if p := conn.Process().Profile(); p != nil {
		return fmt.Sprintf(
			"%s/%s>%s",
			p.LocalProfile().Source,
			p.LocalProfile().ID,
			conn.Entity.IP,
		)
	}

	return "?>" + string(conn.Entity.IP)
}

func makeStickyDomainKey(conn *network.Connection) string {
	if p := conn.Process().Profile(); p != nil {
		return fmt.Sprintf(
			"%s/%s>%s",
			p.LocalProfile().Source,
			p.LocalProfile().ID,
			conn.Entity.Domain,
		)
	}

	return "?>" + conn.Entity.Domain
}

func getStickiedHub(conn *network.Connection) (sticksTo *stickyHub) {
	stickyLock.Lock()
	defer stickyLock.Unlock()

	// Check if IP is sticky.
	sticksTo = stickyIPs[makeStickyIPKey(conn)] // byte comparison
	if sticksTo != nil && !sticksTo.isExpired() {
		sticksTo.LastSeen = time.Now()
	}

	// If the IP did not stick and we have a domain, check if that sticks.
	if sticksTo == nil && conn.Entity.Domain != "" {
		var ok bool
		sticksTo, ok = stickyDomains[makeStickyDomainKey(conn)]
		if ok && !sticksTo.isExpired() {
			sticksTo.LastSeen = time.Now()
		}
	}

	// If nothing sticked, return now.
	if sticksTo == nil {
		return nil
	}

	// Get intel from map before locking pin to avoid simultaneous locking.
	mapIntel := navigator.Main.GetIntel()

	// Lock Pin for checking.
	sticksTo.Pin.Lock()
	defer sticksTo.Pin.Unlock()

	// Check if the stickied Hub supports the needed IP version.
	switch {
	case conn.IPVersion == packet.IPv4 && sticksTo.Pin.EntityV4 == nil:
		// Connection is IPv4, but stickied Hub has no IPv4.
		return nil
	case conn.IPVersion == packet.IPv6 && sticksTo.Pin.EntityV6 == nil:
		// Connection is IPv4, but stickied Hub has no IPv4.
		return nil
	}

	// Disregard stickied Hub if it is disregard with the current options.
	matcher := conn.TunnelOpts.Destination.Matcher(mapIntel)
	if !matcher(sticksTo.Pin) {
		return nil
	}

	// Return fully checked stickied Hub.
	return sticksTo
}

func (t *Tunnel) stickDestinationToHub() {
	stickyLock.Lock()
	defer stickyLock.Unlock()

	// Stick to IP.
	ipKey := makeStickyIPKey(t.connInfo)
	stickyIPs[ipKey] = &stickyHub{
		Pin:              t.dstPin,
		Route:            t.route,
		LastSeen:         time.Now(),
		RequestedProfile: t.requestedProfile,
	}
	log.Infof("spn/crew: sticking %s to %s", ipKey, t.dstPin.Hub)

	// Stick to Domain, if present.
	if t.connInfo.Entity.Domain != "" {
		domainKey := makeStickyDomainKey(t.connInfo)
		stickyDomains[domainKey] = &stickyHub{
			Pin:              t.dstPin,
			Route:            t.route,
			LastSeen:         time.Now(),
			RequestedProfile: t.requestedProfile,
		}
		log.Infof("spn/crew: sticking %s to %s", domainKey, t.dstPin.Hub)
	}
}

func (t *Tunnel) avoidDestinationHub() {
	stickyLock.Lock()
	defer stickyLock.Unlock()

	// Stick to Hub/IP Pair.
	ipKey := makeStickyIPKey(t.connInfo)
	stickyIPs[ipKey] = &stickyHub{
		Pin:      t.dstPin,
		LastSeen: time.Now(),
		Avoid:    true,
	}
	log.Warningf("spn/crew: avoiding %s for %s", t.dstPin.Hub, ipKey)

	// TODO: Question: Should we avoid the domain as well? (stickyDomains)
}

func cleanStickyHubs(ctx *mgr.WorkerCtx) error {
	stickyLock.Lock()
	defer stickyLock.Unlock()

	for _, stickyRegistry := range []map[string]*stickyHub{stickyIPs, stickyDomains} {
		for key, stickedEntry := range stickyRegistry {
			if stickedEntry.isExpired() {
				delete(stickyRegistry, key)
			}
		}
	}

	return nil
}

func clearStickyHubs() {
	stickyLock.Lock()
	defer stickyLock.Unlock()

	for _, stickyRegistry := range []map[string]*stickyHub{stickyIPs, stickyDomains} {
		for key := range stickyRegistry {
			delete(stickyRegistry, key)
		}
	}
}

// ClearStickyHubs drops remembered routes. A stickied route is reused for an
// hour without asking the route finder, so a hop change would otherwise keep
// sending new connections down the old path.
func ClearStickyHubs() {
	clearStickyHubs()
}

// NoteRoute, if set, receives every route crew establishes: the real hop
// count and the exit hub's country. Android's routing status sets it; it stays
// nil on desktop.
var NoteRoute func(hops int, exitCountry string)

func publishEstablishedRoute(route *navigator.Route, pin *navigator.Pin) {
	if NoteRoute == nil || route == nil {
		return
	}
	NoteRoute(len(route.Path), pinCountryName(pin))
}

func pinCountryName(pin *navigator.Pin) string {
	if pin == nil {
		return ""
	}
	if pin.LocationV4 != nil && pin.LocationV4.Country.Name != "" {
		return pin.LocationV4.Country.Name
	}
	if pin.LocationV6 != nil {
		return pin.LocationV6.Country.Name
	}
	return ""
}
