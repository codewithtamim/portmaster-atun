//go:build !android

package resolver

import "context"

func (tr *TCPResolver) lockDial(context.Context, int) (*tcpResolverConn, func(), error) {
	return nil, func() {}, nil
}
