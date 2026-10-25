//go:build android

package wireguard

import (
	"errors"
	"math/rand/v2"
	"time"

	"golang.zx2c4.com/wireguard/device"

	"github.com/safing/portmaster/base/log"
	"github.com/safing/portmaster/service/mgr"
	"github.com/safing/portmaster/service/routing"
)

// Supervision timing.
const (
	// How often the device is checked, by state. Connecting is polled tightly so
	// the UI settles quickly; connected is cheap to check but there is nothing to
	// learn from doing it often.
	connectingInterval = 2 * time.Second
	connectedInterval  = 10 * time.Second
	idleInterval       = time.Minute

	// How long a peer may stay quiet after going stale before the device is torn
	// down and rebuilt.
	degradedGrace = 30 * time.Second

	minBackoff = time.Second
	maxBackoff = time.Minute
)

// supervisor owns the device. It is the only thing that starts, stops or
// reconfigures it, so the device needs no lock of its own.
func (w *WireGuard) supervisor(ctx *mgr.WorkerCtx) error {
	defer w.teardown()

	timer := time.NewTimer(w.step(ctx))
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-w.wake:
			stopTimer(timer)
		case <-timer.C:
		}
		timer.Reset(w.step(ctx))
	}
}

func stopTimer(timer *time.Timer) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}

// step brings the device towards what the settings ask for and returns how long
// to wait before looking again.
func (w *WireGuard) step(ctx *mgr.WorkerCtx) time.Duration {
	profile := w.wantedProfile()
	wanted := profile != nil && routing.Current() == routing.TargetWireGuard

	switch {
	case !wanted:
		if w.session != nil {
			log.Info("wireguard: routing target changed, taking the device down")
			w.teardown()
		}
		w.failures = 0
		w.retryAt = time.Time{}
		w.publishDown(profile)
		return idleInterval

	case w.session != nil && w.session.profile != profile:
		// A different profile was installed while running.
		log.Info("wireguard: profile replaced, restarting the device")
		w.failures = 0
		w.retryAt = time.Time{}
		w.teardown()
		fallthrough

	case w.session == nil:
		return w.bringUp(ctx, profile)

	default:
		// A network change re-resolves the endpoint and rebuilds the sockets
		// instead of waiting for the peer to time out.
		if w.networkDirty.Swap(false) {
			// The device's MTU is fixed when it is created. A network with a
			// different link MTU needs a new device, or packets fragment.
			if mtu := tunnelMTU(); mtu != w.session.mtu {
				log.Infof("wireguard: link mtu changed, rebuilding the device with mtu %d", mtu)
				w.teardown()
				return w.bringUp(ctx, profile)
			}
			w.onNetworkChange(ctx)
		}
		return w.check(ctx)
	}
}

// bringUp starts a device, honouring the failure backoff.
func (w *WireGuard) bringUp(ctx *mgr.WorkerCtx, profile *Profile) time.Duration {
	if remaining := time.Until(w.retryAt); remaining > 0 {
		return remaining
	}

	session, err := startSession(ctx.Ctx(), profile)
	if err != nil {
		w.failures++
		w.retryAt = time.Now().Add(backoff(w.failures))
		log.Warningf("wireguard: bring-up failed (attempt %d): %s", w.failures, err)
		w.publish(&Snapshot{
			State:     StateFailed,
			Profile:   profile.Meta(),
			ErrCode:   errCodeFor(err),
			ErrDetail: err.Error(),
		})
		return time.Until(w.retryAt)
	}

	w.failures = 0
	w.session = session
	w.liveStack.Store(session.net)
	w.publishFrom(session, StateConnecting, ErrCodeNone, "")
	return connectingInterval
}

// check reads the device counters and moves the state machine.
func (w *WireGuard) check(ctx *mgr.WorkerCtx) time.Duration {
	s := w.session
	stats := s.stats()

	handshaked := !stats.LastHandshake.IsZero()
	if handshaked {
		s.everHandshaked = true
	}

	// The peer is answering if a handshake landed within the protocol's reject
	// window, or if bytes came back since the last check. Persistent keepalives
	// increase the send counter and get no reply, and an idle tunnel only
	// rekeys around RekeyAfterTime (120s), so a shorter window looks like a
	// dead peer on every healthy idle tunnel.
	receiving := stats.RxBytes > s.lastRx
	sending := stats.TxBytes > s.lastTx
	fresh := handshaked && time.Since(stats.LastHandshake) < device.RejectAfterTime

	s.lastRx, s.lastTx = stats.RxBytes, stats.TxBytes

	switch {
	case !s.everHandshaked:
		// Still shaking hands. Give up only once the backoff would have.
		if time.Since(s.startedAt) > maxBackoff {
			log.Warning("wireguard: no handshake, rebuilding the device")
			w.teardown()
			w.failures++
			w.retryAt = time.Now().Add(backoff(w.failures))
			w.publish(&Snapshot{
				State:     StateFailed,
				Profile:   s.profile.Meta(),
				ErrCode:   ErrCodeHandshake,
				ErrDetail: "no handshake with the peer",
			})
			return time.Until(w.retryAt)
		}
		w.publishStats(s, StateConnecting, stats)
		return connectingInterval

	case fresh || receiving:
		s.staleSince = time.Time{}
		w.maybeProbeV6(s)
		w.publishStats(s, StateConnected, stats)
		return connectedInterval

	case !sending:
		// Nothing is being sent, so there is nothing for the peer to answer.
		// An idle tunnel is a working tunnel.
		w.maybeProbeV6(s)
		w.publishStats(s, StateConnected, stats)
		return connectedInterval

	default:
		// We are sending and the peer has gone quiet.
		if s.staleSince.IsZero() {
			s.staleSince = time.Now()
			log.Warning("wireguard: peer stopped answering, re-resolving the endpoint")
			if err := s.rebind(ctx.Ctx()); err != nil {
				log.Warningf("wireguard: rebind failed: %s", err)
			}
		} else if time.Since(s.staleSince) > degradedGrace {
			log.Warning("wireguard: peer still quiet, rebuilding the device")
			w.teardown()
			w.failures++
			w.retryAt = time.Now().Add(backoff(w.failures))
			return time.Until(w.retryAt)
		}
		w.publishStats(s, StateDegraded, stats)
		return connectingInterval
	}
}

// onNetworkChange re-resolves and rebinds without a full restart. Runs on the
// supervisor, so the device is still only ever touched from one goroutine.
func (w *WireGuard) onNetworkChange(ctx *mgr.WorkerCtx) {
	if w.session == nil {
		return
	}
	if err := w.session.rebind(ctx.Ctx()); err != nil {
		log.Warningf("wireguard: rebind after network change failed: %s", err)
	}
}

func (w *WireGuard) teardown() {
	if w.session == nil {
		return
	}
	// Retract the stack before closing, so no new dial can land on a device
	// that is going away.
	w.liveStack.Store(nil)
	w.session.close()
	w.session = nil
}

func (w *WireGuard) publishStats(s *session, state State, stats Stats) {
	w.publish(&Snapshot{
		State:         state,
		Profile:       s.effectiveMeta(),
		Endpoint:      s.endpointString(),
		LastHandshake: stats.LastHandshake,
		RxBytes:       stats.RxBytes,
		TxBytes:       stats.TxBytes,
		endpoints:     s.endpoints,
		peerHost:      peerHost(s.profile),
	})
}

func (w *WireGuard) publishFrom(s *session, state State, errCode, errDetail string) {
	w.publish(&Snapshot{
		State:     state,
		Profile:   s.effectiveMeta(),
		Endpoint:  s.endpointString(),
		ErrCode:   errCode,
		ErrDetail: errDetail,
		endpoints: s.endpoints,
		peerHost:  peerHost(s.profile),
	})
}

func (w *WireGuard) publishDown(profile *Profile) {
	snapshot := &Snapshot{State: StateDown}
	if profile != nil {
		snapshot.Profile = profile.Meta()
	} else {
		snapshot.ErrCode = ErrCodeNoProfileC
	}

	// The idle tick runs forever while the target is not WireGuard. Republishing
	// an identical snapshot would be a websocket message a minute saying nothing.
	current := Status()
	if current.State == snapshot.State &&
		current.Profile == snapshot.Profile &&
		current.ErrCode == snapshot.ErrCode {
		return
	}

	w.publish(snapshot)
}

// backoff grows with consecutive failures and is jittered so a flapping network
// does not line every retry up on the same instant.
func backoff(failures int) time.Duration {
	if failures < 1 {
		failures = 1
	}
	d := minBackoff << min(failures-1, 8)
	d = min(d, maxBackoff)
	//nolint:gosec // backoff jitter, not a secret
	return d/2 + time.Duration(rand.Int64N(int64(d/2)+1))
}

func errCodeFor(err error) string {
	switch {
	case errors.Is(err, ErrNoProtector), errors.Is(err, ErrProtect):
		return ErrCodeNoProtect
	case errors.Is(err, ErrResolve):
		return ErrCodeResolve
	case errors.Is(err, ErrBadProfile):
		return ErrCodeConfig
	default:
		return ErrCodeDeviceUp
	}
}
