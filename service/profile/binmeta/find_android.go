//go:build android

package binmeta

import "context"

// GetIconAndName returns zero values on Android. App identity comes from
// PackageManager through aconn, not from scanning XDG icon directories.
func GetIconAndName(_ context.Context, _ string, _ string) (icon *Icon, name string, err error) {
	return nil, "", nil
}
