//go:build android

package wireguard

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"time"

	"github.com/miekg/dns"

	"github.com/safing/portmaster/base/config"
	"github.com/safing/portmaster/base/utils/aprotect"
)

// cfgOptionNameServersKey mirrors resolver.CfgOptionNameServersKey. Copied
// because the resolver imports this package.
const cfgOptionNameServersKey = "dns/nameservers"

// bootstrapServerTimeout bounds one server, so a dead first server leaves
// time for the next one within resolveTimeout.
const bootstrapServerTimeout = 2 * time.Second

// maxDoHResponse is far above any DNS message.
const maxDoHResponse = 64 << 10

var errNoBootstrapServer = errors.New("wireguard: no configured nameserver has an IP address")

// bootstrapServer is a configured nameserver reached on a protected socket.
type bootstrapServer struct {
	// kind is "udp", "tcp", "tls" or "https".
	kind       string
	addr       netip.AddrPort
	serverName string
	path       string
}

// lookupProtected resolves host by asking the configured nameservers directly,
// on protected sockets.
//
// This is the lookup that brings the tunnel up. Portmaster's own resolver
// sends its queries along the routing target, and with lockdown on those are
// blocked until the tunnel is connected, so asking it would wait on itself.
// The lookup is not filtered or logged by Portmaster. It uses the same
// servers and the same transport (DoT, DoH, plain) the user configured.
func lookupProtected(ctx context.Context, host string) ([]net.IP, error) {
	servers := bootstrapServers(config.Concurrent.GetAsStringArray(cfgOptionNameServersKey, nil)())
	if len(servers) == 0 {
		return nil, errNoBootstrapServer
	}

	var lastErr error
	for _, server := range servers {
		if ctx.Err() != nil {
			break
		}
		ips, err := queryServer(ctx, server, host)
		if err == nil && len(ips) > 0 {
			return ips, nil
		}
		if err == nil {
			err = fmt.Errorf("no address for %s", host)
		}
		lastErr = fmt.Errorf("%s %s: %w", server.kind, server.addr, err)
	}
	if lastErr == nil {
		lastErr = ctx.Err()
	}
	return nil, lastErr
}

// bootstrapServers parses resolver URLs as the resolver does, keeping only
// those with an IP address: a server known only by name would need a lookup
// of its own.
func bootstrapServers(urls []string) []bootstrapServer {
	servers := make([]bootstrapServer, 0, len(urls))
	for _, raw := range urls {
		if server, ok := parseBootstrapServer(raw); ok {
			servers = append(servers, server)
		}
	}
	return servers
}

func parseBootstrapServer(raw string) (bootstrapServer, bool) {
	u, err := url.Parse(raw)
	if err != nil {
		return bootstrapServer{}, false
	}

	var server bootstrapServer
	var port uint16
	switch u.Scheme {
	case "dns":
		server.kind, port = "udp", 53
	case "tcp":
		server.kind, port = "tcp", 53
	case "dot", "tls":
		server.kind, port = "tls", 853
	case "doh", "https":
		server.kind, port = "https", 443
	default:
		return bootstrapServer{}, false
	}
	if p := u.Port(); p != "" {
		parsed, err := strconv.ParseUint(p, 10, 16)
		if err != nil || parsed == 0 {
			return bootstrapServer{}, false
		}
		port = uint16(parsed)
	}

	query := u.Query()
	host := u.Hostname()
	ip, err := netip.ParseAddr(host)
	switch {
	case err == nil:
		// IP in the URL; DoT and DoH name the certificate with verify.
		server.serverName = query.Get("verify")
	case server.kind == "tls" || server.kind == "https":
		// Name in the URL, IP as a parameter.
		ip, err = netip.ParseAddr(query.Get("ip"))
		if err != nil {
			return bootstrapServer{}, false
		}
		server.serverName = host
	default:
		return bootstrapServer{}, false
	}
	if (server.kind == "tls" || server.kind == "https") && server.serverName == "" {
		// Nothing to verify the certificate against.
		return bootstrapServer{}, false
	}

	server.addr = netip.AddrPortFrom(ip.Unmap(), port)
	server.path = u.Path
	return server, true
}

// queryServer asks one server for A records and, if there are none, AAAA.
func queryServer(ctx context.Context, server bootstrapServer, host string) ([]net.IP, error) {
	ctx, cancel := context.WithTimeout(ctx, bootstrapServerTimeout)
	defer cancel()

	var lastErr error
	for _, qtype := range []uint16{dns.TypeA, dns.TypeAAAA} {
		msg := new(dns.Msg)
		msg.SetQuestion(dns.Fqdn(host), qtype)

		reply, err := exchange(ctx, server, msg)
		if err != nil {
			lastErr = err
			continue
		}
		if reply.Rcode != dns.RcodeSuccess {
			lastErr = fmt.Errorf("rcode %s", dns.RcodeToString[reply.Rcode])
			continue
		}
		if ips := answerIPs(reply); len(ips) > 0 {
			return ips, nil
		}
	}
	return nil, lastErr
}

func exchange(ctx context.Context, server bootstrapServer, msg *dns.Msg) (*dns.Msg, error) {
	if server.kind == "https" {
		return exchangeDoH(ctx, server, msg)
	}

	client := &dns.Client{Net: server.kind, Dialer: aprotect.NewDialer()}
	switch server.kind {
	case "tls":
		client.Net = "tcp-tls"
		client.TLSConfig = &tls.Config{ServerName: server.serverName, MinVersion: tls.VersionTLS12}
	case "udp":
		client.Net = "udp"
	}
	reply, _, err := client.ExchangeContext(ctx, msg, server.addr.String())
	return reply, err
}

// exchangeDoH sends one RFC 8484 POST. The transport dials the configured IP,
// never a name.
func exchangeDoH(ctx context.Context, server bootstrapServer, msg *dns.Msg) (*dns.Msg, error) {
	body, err := msg.Pack()
	if err != nil {
		return nil, err
	}

	dialer := aprotect.NewDialer()
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, server.addr.String())
		},
		TLSClientConfig:   &tls.Config{ServerName: server.serverName, MinVersion: tls.VersionTLS12},
		ForceAttemptHTTP2: true,
		DisableKeepAlives: true,
	}
	defer transport.CloseIdleConnections()

	path := server.path
	if path == "" {
		path = "/dns-query"
	}
	target := url.URL{Scheme: "https", Host: server.serverName, Path: path}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/dns-message")
	req.Header.Set("Accept", "application/dns-message")

	resp, err := (&http.Client{Transport: transport}).Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("doh status %s", resp.Status)
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxDoHResponse))
	if err != nil {
		return nil, err
	}
	reply := new(dns.Msg)
	if err := reply.Unpack(raw); err != nil {
		return nil, err
	}
	return reply, nil
}

func answerIPs(reply *dns.Msg) []net.IP {
	var ips []net.IP
	for _, rr := range reply.Answer {
		switch rec := rr.(type) {
		case *dns.A:
			ips = append(ips, rec.A)
		case *dns.AAAA:
			ips = append(ips, rec.AAAA)
		}
	}
	return ips
}
