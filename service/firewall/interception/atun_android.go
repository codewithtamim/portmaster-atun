//go:build android

package interception

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/safing/portmaster/base/log"
	"github.com/safing/portmaster/service/firewall/interception/atun"
	"github.com/safing/portmaster/service/mgr"
	"github.com/safing/portmaster/service/netenv"
	"github.com/safing/portmaster/service/network/packet"
)

// Outbound queue IDs match nfqueue. There is no inbound path: the TUN only
// sees traffic the OS routed into the VPN.
const (
	atunOut4QueueID = 17040
	atunOut6QueueID = 17060
)

// nfQueue encapsulates queue providers with the same interface as nfqueue_linux.go.
type nfQueue interface {
	PacketChannel() <-chan packet.Packet
	Destroy()
}

type disabledNfQueue struct{}

func (dnfq *disabledNfQueue) PacketChannel() <-chan packet.Packet {
	return nil
}

func (dnfq *disabledNfQueue) Destroy() {}

var (
	out4Queue nfQueue
	out6Queue nfQueue

	isRunning      atomic.Bool
	shutdownSignal = make(chan struct{})

	// atunLock serializes start/stop (Android restarts VPN in-process).
	atunLock sync.Mutex
)

func activateAtunFirewall() error {
	// Close flows left over from a previous session before replacing the
	// table: once replaced, nothing could reach them to close them.
	atun.TeardownFlowTable()
	if err := atun.InitFlowTable(); err != nil {
		return fmt.Errorf("failed to initialize connection tracking: %w", err)
	}

	return nil
}

// DeactivateAtunFirewall closes every tracked flow. Apps reconnect and get a
// fresh verdict from the next session.
func DeactivateAtunFirewall() error {
	atun.TeardownFlowTable()

	return nil
}

// StartAtunInterception opens the TUN stack and starts feeding its flows to
// packets. Safe to call while running; it then does nothing.
func StartAtunInterception(packets chan<- packet.Packet) (err error) {
	atunLock.Lock()
	defer atunLock.Unlock()

	if !isRunning.CompareAndSwap(false, true) {
		return nil // already running
	}

	shutdownSignal = make(chan struct{})

	// Failed start must leave nothing behind. The VPN will retry.
	defer func() {
		if err == nil {
			return
		}
		_ = atun.Close()
		out4Queue = nil
		out6Queue = nil
		_ = DeactivateAtunFirewall()
		isRunning.Store(false)
	}()

	if err = activateAtunFirewall(); err != nil {
		return fmt.Errorf("could not initialize atun: %w", err)
	}

	if err = atun.Open(module.mgr); err != nil {
		return fmt.Errorf("atun stack: %w", err)
	}

	out4Queue, err = atun.New(atunOut4QueueID, false)
	if err != nil {
		return fmt.Errorf("atun queue(IPv4, out): %w", err)
	}

	if netenv.IPv6Enabled() {
		out6Queue, err = atun.New(atunOut6QueueID, true)
		if err != nil {
			return fmt.Errorf("atun queue(IPv6, out): %w", err)
		}
	} else {
		log.Warningf("interception: no IPv6 stack detected, disabling IPv6 network integration")
		out6Queue = &disabledNfQueue{}
	}

	// Pass this session's queues and signal by value. A worker that re-read
	// the globals could outlive a Stop/Start and pick up the next session's.
	in4, in6, done := out4Queue.PacketChannel(), out6Queue.PacketChannel(), shutdownSignal
	module.mgr.Go("atun packet handler", func(_ *mgr.WorkerCtx) error {
		return handleInterception(packets, in4, in6, done)
	})

	return nil
}

// StopAtunInterception tears the TUN stack down. Safe to call while stopped.
func StopAtunInterception() error {
	atunLock.Lock()
	defer atunLock.Unlock()

	if !isRunning.CompareAndSwap(true, false) {
		return nil // not running
	}

	defer close(shutdownSignal)

	if out4Queue != nil {
		out4Queue.Destroy()
	}
	if out6Queue != nil {
		out6Queue.Destroy()
	}

	if err := atun.Close(); err != nil {
		log.Warningf("interception: error while closing atun stack: %s", err)
	}

	return DeactivateAtunFirewall()
}

// handleInterception moves flows from one session's queues to the firewall
// until that session's done channel closes. A nil queue channel (IPv6
// disabled) blocks forever in select, which is the intended "never ready".
func handleInterception(packets chan<- packet.Packet, in4, in6 <-chan packet.Packet, done <-chan struct{}) error {
	for {
		var pkt packet.Packet
		select {
		case <-done:
			return nil
		case pkt = <-in4:
		case pkt = <-in6:
		}
		pkt.SetOutbound()

		select {
		case packets <- pkt:
		case <-done:
			return nil
		}
	}
}
