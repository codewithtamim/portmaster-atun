//go:build android

package wireguard

import (
	"golang.zx2c4.com/wireguard/conn"

	"github.com/safing/portmaster/base/utils/aprotect"
)

// protectedBind excludes the UDP sockets from the TUN inside Open, before
// wireguard-go sends the first handshake on them.
type protectedBind struct {
	*conn.StdNetBind
}

func newProtectedBind() (*protectedBind, error) {
	std, ok := conn.NewStdNetBind().(*conn.StdNetBind)
	if !ok {
		return nil, ErrProtect
	}
	return &protectedBind{StdNetBind: std}, nil
}

func (b *protectedBind) Open(port uint16) ([]conn.ReceiveFunc, uint16, error) {
	fns, actual, err := b.StdNetBind.Open(port)
	if err != nil {
		return nil, 0, err
	}
	if err := protectBind(b.StdNetBind); err != nil {
		_ = b.StdNetBind.Close()
		return nil, 0, err
	}
	return fns, actual, nil
}

func protectBind(peek conn.PeekLookAtSocketFd) error {
	protected := 0
	failed := 0
	for _, look := range []func() (int, error){peek.PeekLookAtSocketFd4, peek.PeekLookAtSocketFd6} {
		fd, ok := lookFD(look)
		if !ok {
			continue
		}
		if !aprotect.ProtectFD(fd) {
			failed++
			continue
		}
		protected++
	}
	if failed > 0 || protected == 0 {
		return ErrProtect
	}
	return nil
}

// lookFD reads one socket. A missing address family leaves the conn nil, and
// Peek then panics; that family is simply not up.
func lookFD(look func() (int, error)) (int, bool) {
	panicked := false
	var fd int
	var err error
	func() {
		defer func() {
			if recover() != nil {
				panicked = true
			}
		}()
		fd, err = look()
	}()
	if panicked || err != nil || fd < 0 {
		return -1, false
	}
	return fd, true
}
