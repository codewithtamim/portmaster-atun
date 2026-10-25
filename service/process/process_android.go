//go:build android

package process

import (
	"context"
)

const (
	// SystemProcessID is the PID of the System/Kernel itself.
	SystemProcessID = 0

	// SystemInitID is the PID of the system init process.
	SystemInitID = 1
)

// FindProcessGroupLeader returns the process that leads the process group.
// Returns nil on Android: an unprivileged app cannot see other UIDs, and the
// owning UID is already the identity nearest to the user.
func (p *Process) FindProcessGroupLeader(_ context.Context) error {
	return nil
}

// GetProcessGroupID returns the process group ID of the given PID.
// Returns the undefined process ID on Android, as it does not have process groups.
func GetProcessGroupID(_ context.Context, _ int) (int, error) {
	return UndefinedProcessID, nil
}
