package aprotect

// Protector is implemented by the Android VpnService wrapper.
// It must protect outbound sockets from being routed back into the TUN.
type Protector interface {
	Protect(fd int32) bool
}
