//go:build android

package utils

import "reflect"

// IsNilCallback reports whether v is a nil gomobile callback. gomobile
// typed pointers are never == nil directly, so a nil interface holding a
// nil pointer must be detected via reflection. Used by the Android-specific
// bridge packages (aconn, anet, anotif, aprotect).
func IsNilCallback(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	return rv.Kind() == reflect.Ptr && rv.IsNil()
}
