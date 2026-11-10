//go:build android

package atun

import (
	"strings"
	"testing"

	"github.com/miekg/dns"
)

func bigTXTReply(t *testing.T, query *dns.Msg, records int) []byte {
	t.Helper()
	reply := new(dns.Msg)
	reply.SetReply(query)
	for i := 0; i < records; i++ {
		rr, err := dns.NewRR("example.com. 60 IN TXT \"" + strings.Repeat("x", 80) + "\"")
		if err != nil {
			t.Fatal(err)
		}
		reply.Answer = append(reply.Answer, rr)
	}
	packed, err := reply.Pack()
	if err != nil {
		t.Fatal(err)
	}

	return packed
}

func packQuery(t *testing.T, edns uint16) (*dns.Msg, []byte) {
	t.Helper()
	q := new(dns.Msg)
	q.SetQuestion("example.com.", dns.TypeTXT)
	if edns > 0 {
		q.SetEdns0(edns, false)
	}
	packed, err := q.Pack()
	if err != nil {
		t.Fatal(err)
	}

	return q, packed
}

// A UDP reply larger than the app asked for is trimmed to its size with TC
// set, so the app retries over TCP instead of reading a cut-off packet.
func TestFitUDPReplyTrimsToTheAppsSize(t *testing.T) {
	for _, tc := range []struct {
		edns uint16
		max  int
	}{{0, 512}, {512, 512}, {1232, 1232}} {
		q, query := packQuery(t, tc.edns)
		reply := bigTXTReply(t, q, 60)
		got := fitUDPReply(query, reply)
		if len(got) > tc.max {
			t.Fatalf("edns %d: reply is %d bytes, want at most %d", tc.edns, len(got), tc.max)
		}
		var m dns.Msg
		if err := m.Unpack(got); err != nil {
			t.Fatalf("edns %d: trimmed reply does not parse: %v", tc.edns, err)
		}
		if !m.Truncated {
			t.Fatalf("edns %d: TC not set on a trimmed reply", tc.edns)
		}
	}
}

func TestFitUDPReplyKeepsRepliesThatFit(t *testing.T) {
	q, query := packQuery(t, 4096)
	reply := bigTXTReply(t, q, 20)
	if got := fitUDPReply(query, reply); len(got) != len(reply) {
		t.Fatalf("reply that fits was changed: %d -> %d bytes", len(reply), len(got))
	}
	small := bigTXTReply(t, q, 1)
	if got := fitUDPReply(query, small); len(got) != len(small) {
		t.Fatal("small reply was changed")
	}
	junk := []byte(strings.Repeat("\xff", 2000))
	if got := fitUDPReply(query, junk); len(got) != len(junk) {
		t.Fatal("unparseable reply was changed")
	}
}
