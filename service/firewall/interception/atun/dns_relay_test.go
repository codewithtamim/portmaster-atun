//go:build android

package atun

import (
	"bytes"
	"errors"
	"net"
	"testing"
	"time"
)

// relayPair starts relayUDPDNS between an app side and a nameserver side.
func relayPair(t *testing.T) (app, nameserver net.Conn, done chan error) {
	t.Helper()
	app, appRelay := net.Pipe()
	nameserver, nsRelay := net.Pipe()
	t.Cleanup(func() { _ = app.Close(); _ = nameserver.Close(); _ = appRelay.Close(); _ = nsRelay.Close() })
	done = make(chan error, 1)
	go func() { done <- relayUDPDNS(appRelay, nsRelay) }()
	return app, nameserver, done
}

func TestRelayUDPDNSPassesQueryAndReply(t *testing.T) {
	app, nameserver, done := relayPair(t)
	query := []byte("a small query")
	reply := bytes.Repeat([]byte{7}, 300)

	go func() { _, _ = app.Write(query) }()
	got := make([]byte, 1024)
	_ = nameserver.SetDeadline(time.Now().Add(2 * time.Second))
	n, err := nameserver.Read(got)
	if err != nil || !bytes.Equal(got[:n], query) {
		t.Fatalf("nameserver got %q, %v", got[:n], err)
	}
	go func() { _, _ = nameserver.Write(reply) }()
	_ = app.SetDeadline(time.Now().Add(2 * time.Second))
	n, err = app.Read(got)
	if err != nil || !bytes.Equal(got[:n], reply) {
		t.Fatalf("app got %d bytes, %v", n, err)
	}
	// No second query: the flow ends by itself after the linger time.
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("relay ended with %v", err)
		}
	case <-time.After(dnsLingerTimeout + 2*time.Second):
		t.Fatal("relay did not end after the linger time")
	}
}

func TestRelayUDPDNSRefusesOversizedQuery(t *testing.T) {
	app, _, done := relayPair(t)
	go func() { _, _ = app.Write(make([]byte, dnsQueryBufferSize+100)) }()
	select {
	case err := <-done:
		if !errors.Is(err, errDNSQueryTooLarge) {
			t.Fatalf("got %v, want %v", err, errDNSQueryTooLarge)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("relay kept an oversized query")
	}
}
