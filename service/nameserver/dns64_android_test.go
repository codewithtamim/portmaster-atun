//go:build android

package nameserver

import (
	"net"
	"testing"

	"github.com/miekg/dns"
)

func a(name, ip string) dns.RR {
	return &dns.A{
		Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
		A:   net.ParseIP(ip),
	}
}

func synthFor(prefix string) func(net.IP) (net.IP, bool) {
	return func(v4 net.IP) (net.IP, bool) {
		ip := net.ParseIP(prefix).To16()
		copy(ip[12:], v4.To4())
		return ip, true
	}
}

func TestNat64AnswerKeepsCNAMEChainAndPublicAddresses(t *testing.T) {
	cname := &dns.CNAME{
		Hdr:    dns.RR_Header{Name: "www.example.com.", Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: 300},
		Target: "example.com.",
	}
	in := []dns.RR{
		cname,
		a("example.com.", "93.184.216.34"),
		a("example.com.", "10.0.0.5"),
		a("example.com.", "127.0.0.1"),
	}
	out, ok := nat64Answer(in, synthFor("64:ff9b::"))
	if !ok {
		t.Fatal("expected an answer")
	}
	if len(out) != 2 {
		t.Fatalf("want the CNAME and one AAAA, got %d records: %v", len(out), out)
	}
	if _, isCNAME := out[0].(*dns.CNAME); !isCNAME {
		t.Fatalf("first record must be the CNAME, got %T", out[0])
	}
	aaaa, isAAAA := out[1].(*dns.AAAA)
	if !isAAAA {
		t.Fatalf("second record must be an AAAA, got %T", out[1])
	}
	if !aaaa.AAAA.Equal(net.ParseIP("64:ff9b::5db8:d822")) {
		t.Fatalf("wrong address %s", aaaa.AAAA)
	}
	if aaaa.Hdr.Ttl != 60 || aaaa.Hdr.Name != "example.com." {
		t.Fatalf("header not carried over: %+v", aaaa.Hdr)
	}
}

func TestNat64AnswerIsEmptyWithoutPublicAddresses(t *testing.T) {
	in := []dns.RR{a("lan.example.", "192.168.1.10"), a("lan.example.", "0.0.0.0")}
	if out, ok := nat64Answer(in, synthFor("64:ff9b::")); ok || len(out) != 0 {
		t.Fatalf("private addresses must not be synthesized, got %v", out)
	}
}
