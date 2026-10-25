//go:build android

package network

import "github.com/safing/portmaster/service/process"

// NewPolicyConnection returns a connection owned by uid for Android device-policy tests.
func NewPolicyConnection(uid int) *Connection {
	return &Connection{process: &process.Process{UserID: uid}}
}
