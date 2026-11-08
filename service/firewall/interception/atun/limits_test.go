//go:build android

package atun

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/miekg/dns"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/stack"

	pmpacket "github.com/safing/portmaster/service/network/packet"
)

func setFlowLimits(t *testing.T, udpTotal, udpPerUID, tcpTotal, tcpPerUID int) {
	t.Helper()
	oldUT, oldUU, oldTT, oldTU := maxUDPFlows, maxUDPFlowsPerUID, maxTCPFlows, maxTCPFlowsPerUID
	maxUDPFlows, maxUDPFlowsPerUID, maxTCPFlows, maxTCPFlowsPerUID = udpTotal, udpPerUID, tcpTotal, tcpPerUID
	t.Cleanup(func() {
		maxUDPFlows, maxUDPFlowsPerUID, maxTCPFlows, maxTCPFlowsPerUID = oldUT, oldUU, oldTT, oldTU
	})

	if err := InitFlowTable(); err != nil {
		t.Fatalf("InitFlowTable: %s", err)
	}
	t.Cleanup(TeardownFlowTable)
}

func flowInfo(protocol pmpacket.IPProtocol, dst string, dstPort uint16) pmpacket.Info {
	return pmpacket.Info{
		Version:  pmpacket.IPv4,
		Protocol: protocol,
		Src:      net.ParseIP("10.111.222.2"),
		SrcPort:  40000,
		Dst:      net.ParseIP(dst),
		DstPort:  dstPort,
	}
}

func uidPtr(uid int) *int { return &uid }

func TestFlowTotalCapExemptsDNS(t *testing.T) {
	setFlowLimits(t, 4, 4, 4, 4)

	var first *flow
	for i := range 4 {
		f, err := flows.begin(fmt.Sprintf("udp-%d", i), flowInfo(pmpacket.UDP, "9.9.9.9", 20000))
		if err != nil {
			t.Fatalf("flow %d refused: %s", i, err)
		}
		if first == nil {
			first = f
		}
	}
	if _, err := flows.begin("udp-over", flowInfo(pmpacket.UDP, "9.9.9.9", 20000)); !errors.Is(err, errFlowLimit) {
		t.Fatalf("flow over the cap: err = %v, want errFlowLimit", err)
	}

	// DNS, to the fake DNS address or elsewhere, is never refused.
	for i := range 20 {
		if _, err := flows.begin(fmt.Sprintf("dns-%d", i), flowInfo(pmpacket.UDP, "10.111.222.3", dnsPort)); err != nil {
			t.Fatalf("dns flow %d refused: %s", i, err)
		}
	}

	// TCP has its own budget, charged after the verdict.
	tcpFlow, err := flows.begin("tcp-0", flowInfo(pmpacket.TCP, "9.9.9.9", 443))
	if err != nil {
		t.Fatalf("tcp flow refused by the udp cap: %s", err)
	}
	if !flows.charge(tcpFlow, uidPtr(10100)) {
		t.Fatal("tcp flow charge refused by the udp cap")
	}

	first.close()
	if _, err := flows.begin("udp-after-close", flowInfo(pmpacket.UDP, "9.9.9.9", 20000)); err != nil {
		t.Fatalf("flow after a close refused: %s", err)
	}
}

func TestFlowPerUIDCap(t *testing.T) {
	setFlowLimits(t, 100, 2, 100, 2)

	var open []*flow
	begin := func(id string) *flow {
		t.Helper()
		f, err := flows.begin(id, flowInfo(pmpacket.UDP, "9.9.9.9", 3478))
		if err != nil {
			t.Fatalf("begin %s: %s", id, err)
		}
		open = append(open, f)
		return f
	}

	if !flows.charge(begin("a1"), uidPtr(10100)) || !flows.charge(begin("a2"), uidPtr(10100)) {
		t.Fatal("flows within the per-app cap were refused")
	}
	if flows.charge(begin("a3"), uidPtr(10100)) {
		t.Fatal("third flow of one app was accepted over its cap of 2")
	}
	if !flows.charge(begin("b1"), uidPtr(10200)) {
		t.Fatal("another app was refused because of the first one")
	}
	if !flows.charge(begin("u1"), nil) || !flows.charge(begin("u2"), uidPtr(-1)) || !flows.charge(begin("u3"), nil) {
		t.Fatal("unattributed flows were refused although the total has room")
	}

	if total, perUID := flows.counts(pmpacket.UDP, 10100); total != 7 || perUID != 2 {
		t.Fatalf("counts = %d total, %d for the app; want 7, 2", total, perUID)
	}

	for _, f := range open {
		f.close()
	}
	if total, perUID := flows.counts(pmpacket.UDP, 10100); total != 0 || perUID != 0 {
		t.Fatalf("counts after close = %d, %d; want 0, 0", total, perUID)
	}
}

// A port 53 flow is not counted at begin, as it may be DNS for us. If the
// verdict says it is not, charge counts it like any other flow.
func TestFlowChargeCountsNonDNSPort53(t *testing.T) {
	setFlowLimits(t, 1, 1, 1, 1)

	other, err := flows.begin("other", flowInfo(pmpacket.UDP, "9.9.9.9", 443))
	if err != nil {
		t.Fatalf("begin: %s", err)
	}
	dnsFlow, err := flows.begin("dns", flowInfo(pmpacket.UDP, "8.8.8.8", dnsPort))
	if err != nil {
		t.Fatalf("port 53 flow refused at begin: %s", err)
	}
	if flows.charge(dnsFlow, uidPtr(10100)) {
		t.Fatal("non-DNS port 53 flow was charged over a full total")
	}
	if total, _ := flows.counts(pmpacket.UDP, 10100); total != 1 {
		t.Fatalf("refused charge changed the total to %d", total)
	}

	other.close()
	if !flows.charge(dnsFlow, uidPtr(10100)) {
		t.Fatal("non-DNS port 53 flow refused with room in the caps")
	}
	dnsFlow.close()
	if total, perUID := flows.counts(pmpacket.UDP, 10100); total != 0 || perUID != 0 {
		t.Fatalf("counts after close = %d, %d; want 0, 0", total, perUID)
	}
}

func TestFlowChargeAfterCloseIsRefused(t *testing.T) {
	setFlowLimits(t, 10, 10, 10, 10)

	f, err := flows.begin("x", flowInfo(pmpacket.TCP, "9.9.9.9", 443))
	if err != nil {
		t.Fatalf("begin: %s", err)
	}
	f.close()
	if flows.charge(f, uidPtr(10100)) {
		t.Fatal("a closed flow was charged")
	}
	if total, perUID := flows.counts(pmpacket.TCP, 10100); total != 0 || perUID != 0 {
		t.Fatalf("counts = %d, %d; want 0, 0", total, perUID)
	}
}

func TestFlowBeginConcurrentNoOvershoot(t *testing.T) {
	const limit = 50
	setFlowLimits(t, limit, limit, limit, limit)

	var (
		wg       sync.WaitGroup
		accepted atomic.Int32
	)
	for i := range 500 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := flows.begin(fmt.Sprintf("c-%d", i), flowInfo(pmpacket.UDP, "9.9.9.9", 443)); err == nil {
				accepted.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := accepted.Load(); got != limit {
		t.Errorf("accepted %d flows, want exactly %d", got, limit)
	}
	if total, _ := flows.counts(pmpacket.UDP, 0); total != limit {
		t.Errorf("total count = %d, want %d", total, limit)
	}
}

// Pending TCP handshakes are not counted, so a burst of them cannot fill the
// total. Charging after the verdict holds both caps under concurrency.
func TestTCPChargeConcurrentNoOvershoot(t *testing.T) {
	const total, perUID = 40, 10
	setFlowLimits(t, total, perUID, total, perUID)

	pending := make([]*flow, 0, 500)
	for i := range 500 {
		f, err := flows.begin(fmt.Sprintf("t-%d", i), flowInfo(pmpacket.TCP, "9.9.9.9", 443))
		if err != nil {
			t.Fatalf("pending tcp flow %d refused: %s", i, err)
		}
		pending = append(pending, f)
	}
	if n, _ := flows.counts(pmpacket.TCP, 0); n != 0 {
		t.Fatalf("pending tcp flows counted: %d", n)
	}

	var (
		wg       sync.WaitGroup
		accepted atomic.Int32
	)
	for i, f := range pending {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Ten apps, fifty flows each.
			if flows.charge(f, uidPtr(10000+i%10)) {
				accepted.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := accepted.Load(); got != total {
		t.Errorf("accepted %d flows, want exactly %d", got, total)
	}
	for uid := 10000; uid < 10010; uid++ {
		if _, n := flows.counts(pmpacket.TCP, uid); n > perUID {
			t.Errorf("uid %d holds %d flows, over its cap of %d", uid, n, perUID)
		}
	}
}

// A UDP flow handed to the SPN closes after the idle timeout, and leaves
// the table, while traffic keeps it open.
func TestIdleConnClosesFlow(t *testing.T) {
	setFlowLimits(t, 10, 10, 10, 10)

	f, err := flows.begin("spn", flowInfo(pmpacket.UDP, "9.9.9.9", 3478))
	if err != nil {
		t.Fatalf("begin: %s", err)
	}
	appSide, peer := net.Pipe()
	t.Cleanup(func() { _ = peer.Close() })

	const timeout = 150 * time.Millisecond
	ic := newIdleConn(&ownedConn{Conn: appSide, flow: f}, timeout)
	f.addCloser(ic)

	go func() {
		buf := make([]byte, 8)
		for {
			if _, err := peer.Read(buf); err != nil {
				return
			}
		}
	}()

	// Activity for several timeouts keeps it open.
	for range 8 {
		if _, err := ic.Write([]byte("x")); err != nil {
			t.Fatalf("write on an active flow: %s", err)
		}
		time.Sleep(timeout / 3)
	}
	if f.isClosed() {
		t.Fatal("active flow was closed as idle")
	}

	deadline := time.Now().Add(5 * timeout)
	for !f.isClosed() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !f.isClosed() {
		t.Fatal("idle flow was not closed")
	}
	if _, ok := flows.get("spn"); ok {
		t.Error("idle flow is still in the table")
	}
	if total, _ := flows.counts(pmpacket.UDP, 0); total != 0 {
		t.Errorf("total count = %d after the idle close, want 0", total)
	}
	if err := ic.Close(); err != nil {
		t.Errorf("second close: %s", err)
	}
}

func testSession(t *testing.T, packets chan pmpacket.Packet) *stackSession {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s := &stackSession{
		ctx:     ctx,
		cancel:  cancel,
		fakeDNS: netip.MustParseAddr("10.111.222.3"),
	}
	s.queue4 = &Queue{id: 1, stack: s, packets: packets}

	return s
}

func TestDecideWithoutVerdict(t *testing.T) {
	setFlowLimits(t, 10, 10, 10, 10)

	// No reader and no buffer: the queue is full.
	s := testSession(t, make(chan pmpacket.Packet))
	f, err := flows.begin("full", flowInfo(pmpacket.UDP, "10.111.222.3", dnsPort))
	if err != nil {
		t.Fatalf("begin: %s", err)
	}
	if v := decide(s, s.queue4, f); v.decided || v.mark != MarkDrop {
		t.Fatalf("full queue: decide = %+v, want no verdict and MarkDrop", v)
	}

	s.queue4.Destroy()
	if v := decide(s, s.queue4, f); v.decided {
		t.Fatalf("closed queue: decide = %+v, want no verdict", v)
	}
}

func TestDecideReturnsVerdictAndOwner(t *testing.T) {
	setFlowLimits(t, 10, 10, 10, 10)

	packets := make(chan pmpacket.Packet, 1)
	s := testSession(t, packets)
	go func() {
		pkt := <-packets
		uid := 10123
		pkt.Info().UID = &uid
		_ = pkt.Block()
	}()

	f, err := flows.begin("blocked", flowInfo(pmpacket.UDP, "10.111.222.3", dnsPort))
	if err != nil {
		t.Fatalf("begin: %s", err)
	}
	v := decide(s, s.queue4, f)
	if !v.decided || v.mark != MarkBlock {
		t.Fatalf("decide = %+v, want a Block verdict", v)
	}
	if v.uid == nil || *v.uid != 10123 {
		t.Fatalf("decide uid = %v, want 10123", v.uid)
	}
}

// Fake DNS with no verdict is answered with SERVFAIL, not relayed.
func TestUDPFakeDNSWithoutVerdictServFails(t *testing.T) {
	setFlowLimits(t, 10, 10, 10, 10)

	s := testSession(t, make(chan pmpacket.Packet))
	appSide, client := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })

	id := stack.TransportEndpointID{
		LocalAddress:  tcpip.AddrFrom4([4]byte{10, 111, 222, 3}),
		LocalPort:     dnsPort,
		RemoteAddress: tcpip.AddrFrom4([4]byte{10, 111, 222, 2}),
		RemotePort:    40000,
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		handleUDPFlow(s, id, appSide)
	}()

	var query dns.Msg
	query.SetQuestion("example.com.", dns.TypeA)
	raw, err := query.Pack()
	if err != nil {
		t.Fatalf("pack: %s", err)
	}
	_ = client.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := client.Write(raw); err != nil {
		t.Fatalf("write query: %s", err)
	}
	buf := make([]byte, 512)
	n, err := client.Read(buf)
	if err != nil {
		t.Fatalf("read reply: %s", err)
	}
	var reply dns.Msg
	if err := reply.Unpack(buf[:n]); err != nil {
		t.Fatalf("unpack reply: %s", err)
	}
	if reply.Rcode != dns.RcodeServerFailure || reply.Id != query.Id {
		t.Fatalf("reply rcode %d id %d, want SERVFAIL for id %d", reply.Rcode, reply.Id, query.Id)
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("flow handler did not return")
	}
	if _, err := client.Read(buf); err == nil {
		t.Error("app side still open after SERVFAIL")
	}
}

func TestSlotsCap(t *testing.T) {
	sl := newSlots(3)
	for i := range 3 {
		if !sl.acquire() {
			t.Fatalf("slot %d refused", i)
		}
	}
	if sl.acquire() {
		t.Fatal("slot over the cap granted")
	}
	sl.release()
	if !sl.acquire() {
		t.Fatal("slot refused after a release")
	}
	// Release on an empty set must not block.
	empty := newSlots(1)
	empty.release()
}
