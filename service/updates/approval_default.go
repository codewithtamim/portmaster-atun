//go:build !android

package updates

import "github.com/safing/portmaster/service/mgr"

func (u *Updater) holdsIntelForApproval() bool {
	return false
}

func (u *Updater) stageIntelForApproval(*mgr.WorkerCtx) error {
	return nil
}
