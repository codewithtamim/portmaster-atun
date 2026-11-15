//go:build !android

package status

// registerPlatformAPI registers nothing on desktop.
func (s *Status) registerPlatformAPI() error {
	return nil
}
