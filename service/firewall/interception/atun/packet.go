//go:build android

package atun

import (
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"github.com/tevino/abool"
	"gvisor.dev/gvisor/pkg/tcpip/stack"

	pmpacket "github.com/safing/portmaster/service/network/packet"
	"github.com/safing/portmaster/service/process"
)

// Verdict marks reuse the nfq values, so traces and debug output read the
// same on Android and Linux. Nothing outside this package sees them.
const (
	MarkAccept           = 1700 // 0x6a4
	MarkBlock            = 1701 // 0x6a5
	MarkDrop             = 1702 // 0x6a6
	MarkAcceptFinal      = 1709 // 0x6ad Accept and finalize the decision. Portmaster-owned outbound flows only.
	MarkAcceptAlways     = 1710 // 0x6ae
	MarkBlockAlways      = 1711 // 0x6af
	MarkDropAlways       = 1712 // 0x6b0
	MarkRerouteNS        = 1799 // 0x707
	MarkRerouteSPN       = 1717 // 0x6b5
	MarkRerouteSplitTun  = 1719 // 0x6b7
	MarkRerouteWireGuard = 1720 // 0x6b8
)

func markToString(mark int) string {
	switch mark {
	case MarkAccept:
		return "Accept"
	case MarkBlock:
		return "Block"
	case MarkDrop:
		return "Drop"
	case MarkAcceptFinal:
		return "AcceptFinal"
	case MarkAcceptAlways:
		return "AcceptAlways"
	case MarkBlockAlways:
		return "BlockAlways"
	case MarkDropAlways:
		return "DropAlways"
	case MarkRerouteNS:
		return "RerouteNS"
	case MarkRerouteSPN:
		return "RerouteSPN"
	case MarkRerouteSplitTun:
		return "RerouteSplitTun"
	case MarkRerouteWireGuard:
		return "RerouteWireGuard"
	}
	return "unknown"
}

// packet is the firewall's view of one terminated flow. atun decides once per
// flow, at connect time, not per IP packet.
type packet struct {
	pmpacket.Base

	pktID          uint32
	queue          *Queue
	verdictSet     chan struct{}
	verdictPending *abool.AtomicBool
	verdict        atomic.Int32
}

func (pkt *packet) ID() string {
	return fmt.Sprintf("atun-pkt:%d qid:%d", pkt.pktID, pkt.queue.id)
}

// LoadPacketData does nothing: gVisor has already consumed the packet, so
// there is no payload to load.
func (pkt *packet) LoadPacketData() error {
	return nil
}

// VerdictSet reports whether the flow already has a verdict, including the
// drop decide records when the firewall did not answer in time. The firewall
// then skips the queued flow instead of deciding one nobody waits for.
func (pkt *packet) VerdictSet() bool {
	return pkt.verdictPending.IsSet()
}

func (pkt *packet) appliedMark() int {
	return int(pkt.verdict.Load())
}

// mark stores the first verdict and wakes the forwarder waiting in decide.
// Later verdicts return ErrVerdictAlreadySet and are ignored.
func (pkt *packet) mark(mark int) (err error) {
	if pkt.verdictPending.SetToIf(false, true) {
		defer close(pkt.verdictSet)
		return pkt.setMark(mark)
	}

	return ErrVerdictAlreadySet
}

func (pkt *packet) setMark(mark int) error {
	pkt.verdict.Store(int32(mark))
	return nil
}

func (pkt *packet) Accept() error {
	return pkt.mark(MarkAccept)
}

func (pkt *packet) Block() error {
	if pkt.Info().Protocol == pmpacket.ICMP || pkt.Info().Protocol == pmpacket.ICMPv6 {
		// There is no way to reject a flow the stack already accepted, so a
		// blocked ICMP flow is dropped instead.
		return pkt.mark(MarkDrop)
	}
	return pkt.mark(MarkBlock)
}

func (pkt *packet) Drop() error {
	return pkt.mark(MarkDrop)
}

func (pkt *packet) PermanentAccept() error {
	// Loopback never reaches the TUN, so a permanent verdict would pin a
	// decision the stack does not re-evaluate.
	if !pkt.Info().Inbound && pkt.Info().Dst.IsLoopback() {
		return pkt.Accept()
	}

	return pkt.mark(MarkAcceptAlways)
}

// PermanentAcceptFinal accepts the flow and finalizes the decision.
// This should only be used for Portmaster-owned outbound connections.
func (pkt *packet) PermanentAcceptFinal() error {
	// Loopback never reaches the TUN, so a permanent verdict would pin a
	// decision the stack does not re-evaluate.
	if !pkt.Info().Inbound && pkt.Info().Dst.IsLoopback() {
		return pkt.Accept()
	}

	return pkt.mark(MarkAcceptFinal)
}

func (pkt *packet) PermanentBlock() error {
	if pkt.Info().Protocol == pmpacket.ICMP || pkt.Info().Protocol == pmpacket.ICMPv6 {
		// See Block: a blocked ICMP flow is dropped rather than rejected.
		return pkt.mark(MarkDropAlways)
	}
	return pkt.mark(MarkBlockAlways)
}

func (pkt *packet) PermanentDrop() error {
	return pkt.mark(MarkDropAlways)
}

func (pkt *packet) RerouteToNameserver() error {
	return pkt.mark(MarkRerouteNS)
}

func (pkt *packet) RerouteToTunnel() error {
	return pkt.mark(MarkRerouteSPN)
}

func (pkt *packet) RerouteToWireGuard() error {
	return pkt.mark(MarkRerouteWireGuard)
}

func (pkt *packet) RerouteToSplitTun() error {
	return pkt.mark(MarkRerouteSplitTun)
}

var _ pmpacket.Packet = &packet{}

// newPacketInfo builds outbound packet info for a terminated flow.
// gVisor's endpoint ID is from the stack's side, so Remote is the app and
// Local is the destination; they are swapped here. UID stays nil until
// the connection owner lookup fills it in.
func newPacketInfo(id stack.TransportEndpointID, protocol pmpacket.IPProtocol) pmpacket.Info {
	appIP := net.IP(id.RemoteAddress.AsSlice())
	dstIP := net.IP(id.LocalAddress.AsSlice())

	version := pmpacket.IPv6
	if appIP.To4() != nil {
		version = pmpacket.IPv4
	}

	return pmpacket.Info{
		Inbound:  false,
		Version:  version,
		Protocol: protocol,
		Src:      appIP,
		SrcPort:  id.RemotePort,
		Dst:      dstIP,
		DstPort:  id.LocalPort,
		PID:      process.UndefinedProcessID,
		SeenAt:   time.Now(),
	}
}

func dstAddr(info pmpacket.Info) string {
	return net.JoinHostPort(info.Dst.String(), portToString(info.DstPort))
}
