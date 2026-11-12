//go:build !android

package resolver

// TODO: desktop has the same weakness: a DNS-over-TCP/TLS connection that
// goes silent without an error is never dropped, because its unanswered
// queries stay in flight and block recycling. Android drops it in
// stall_android.go; desktop keeps the upstream behavior.

func noteTCPRead(*tcpResolverConn) {}

func forgetTCPConn(*tcpResolverConn) {}

func (tr *TCPResolver) tcpReplyTimedOut(*tcpResolverConn) {}
