//go:build android

package resolver

import (
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/miekg/dns"
)

// TestTCPResolverReconnectsAfterSilentConnection checks that a connection
// that stops answering without an error is dropped after one unanswered
// query, so the next query gets a new connection and an answer.
func TestTCPResolverReconnectsAfterSilentConnection(t *testing.T) { //nolint:paralleltest // Replaces package-level settings.
	useShortRequestTimeout(t)

	// The first connection reads queries and never answers. Later ones answer.
	serverPort, accepted := startTCPDNSServer(t, func(conn int, _ string) bool { return conn == 1 })

	tr := newStallTestResolver(serverPort)

	_, err := tr.Query(t.Context(), &Query{FQDN: "first.example.", QType: dns.Type(dns.TypeA)})
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("expected the silent connection to time out, got %v", err)
	}

	rrCache, err := tr.Query(t.Context(), &Query{FQDN: "second.example.", QType: dns.Type(dns.TypeA)})
	if err != nil {
		t.Fatalf("expected an answer over a new connection, got %v", err)
	}
	if len(rrCache.Answer) != 1 {
		t.Errorf("expected one answer, got %d", len(rrCache.Answer))
	}
	if got := accepted.Load(); got != 2 {
		t.Errorf("expected 2 connections, got %d", got)
	}
}

// TestTCPResolverKeepsConnectionForOneSlowAnswer checks that one unanswered
// query does not drop a connection that keeps answering other queries:
// closing it would fail every query in flight.
func TestTCPResolverKeepsConnectionForOneSlowAnswer(t *testing.T) { //nolint:paralleltest // Replaces package-level settings.
	useShortRequestTimeout(t)
	serverPort, accepted := startTCPDNSServer(t, func(_ int, name string) bool { return strings.HasPrefix(name, "slow.") })
	tr := newStallTestResolver(serverPort)

	// Open the connection first, so the slow query and the others share it.
	if _, err := tr.Query(t.Context(), &Query{FQDN: "warm.example.", QType: dns.Type(dns.TypeA)}); err != nil {
		t.Fatalf("warm-up query failed: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	var slowErr error
	go func() {
		defer wg.Done()
		_, slowErr = tr.Query(t.Context(), &Query{FQDN: "slow.example.", QType: dns.Type(dns.TypeA)})
	}()

	// Keep the connection answering while the slow query waits.
	deadline := time.Now().Add(2 * defaultRequestTimeout)
	for i := 0; time.Now().Before(deadline); i++ {
		name := "fast" + strconv.Itoa(i) + ".example."
		if _, err := tr.Query(t.Context(), &Query{FQDN: name, QType: dns.Type(dns.TypeA)}); err != nil {
			t.Fatalf("query %s on the working connection failed: %v", name, err)
		}
		time.Sleep(defaultRequestTimeout / 6)
	}
	wg.Wait()

	if !errors.Is(slowErr, ErrTimeout) {
		t.Errorf("expected the unanswered query to time out, got %v", slowErr)
	}
	if got := accepted.Load(); got != 1 {
		t.Errorf("expected the working connection to be kept, got %d connections", got)
	}
}

// useShortRequestTimeout shortens the query timeout and lets the resolver
// pick any local port for the duration of the test.
func useShortRequestTimeout(t *testing.T) {
	t.Helper()
	previousTimeout := defaultRequestTimeout
	previousFactory := localAddrFactory
	defaultRequestTimeout = 300 * time.Millisecond
	localAddrFactory = func(string) net.Addr { return nil }
	t.Cleanup(func() {
		defaultRequestTimeout = previousTimeout
		localAddrFactory = previousFactory
	})
}

// startTCPDNSServer starts a DNS-over-TCP server that answers every query
// with an A record, except those silent(connection number, name) picks,
// which it reads and never answers. It returns the port and the number of
// accepted connections.
func startTCPDNSServer(t *testing.T, silent func(conn int, name string) bool) (uint16, *atomic.Int32) {
	t.Helper()
	server, serverPort := listenTCP(t)
	accepted := new(atomic.Int32)
	go func() {
		for {
			raw, err := server.Accept()
			if err != nil {
				return
			}
			conn := &dns.Conn{Conn: raw}
			number := int(accepted.Add(1))
			go func() {
				defer func() { _ = conn.Close() }()
				for {
					query, err := conn.ReadMsg()
					if err != nil {
						return
					}
					name := query.Question[0].Name
					if silent(number, name) {
						continue
					}
					reply := new(dns.Msg)
					reply.SetReply(query)
					reply.Answer = append(reply.Answer, &dns.A{
						Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
						A:   net.IPv4(192, 0, 2, 7),
					})
					if err := conn.WriteMsg(reply); err != nil {
						return
					}
				}
			}()
		}
	}()
	return serverPort, accepted
}

func newStallTestResolver(port uint16) *TCPResolver {
	return NewTCPResolver(&Resolver{
		ServerAddress: net.JoinHostPort("127.0.0.1", strconv.Itoa(int(port))),
		Info: &ResolverInfo{
			Name:   "test-stall",
			Type:   ServerTypeTCP,
			Source: ServerSourceConfigured,
			IP:     net.IPv4(127, 0, 0, 1),
			Port:   port,
		},
	})
}
