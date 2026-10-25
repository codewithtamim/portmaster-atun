package crew

import "testing"

func TestClearStickyHubsDropsRememberedRoutes(t *testing.T) {
	stickyLock.Lock()
	stickyIPs["test"] = &stickyHub{}
	stickyDomains["test"] = &stickyHub{}
	stickyLock.Unlock()

	ClearStickyHubs()

	stickyLock.Lock()
	defer stickyLock.Unlock()
	if len(stickyIPs) != 0 || len(stickyDomains) != 0 {
		t.Fatal("sticky routes were kept after a hop change")
	}
}
