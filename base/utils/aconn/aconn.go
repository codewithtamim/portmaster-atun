// Package aconn provides connection owner attribution on Android via
// ConnectivityManager.getConnectionOwnerUid.
//
// https://developer.android.com/reference/android/net/ConnectivityManager#getConnectionOwnerUid(int,%20java.net.InetSocketAddress,%20java.net.InetSocketAddress)
package aconn

// UnknownUID is the owner reported when no UID could be attributed.
const UnknownUID = -1

// OwnerResolver must call ConnectivityManager.getConnectionOwnerUid, which only
// returns a UID for flows routed through the caller's own VPN.
type OwnerResolver interface {
	// GetConnectionOwnerUid returns the UID owning the described connection,
	// or -1 if the owner is unknown.
	GetConnectionOwnerUid(protocol int32, localAddr string, localPort int32, remoteAddr string, remotePort int32) int32
}

// AppInfoProvider returns the app details of a UID as a JSON encoded AppInfo.
type AppInfoProvider interface {
	GetAppInfoJSON(uid int32) string
}

// AppInfo describes the app that owns a UID.
type AppInfo struct {
	UID          int      `json:"uid"`
	PackageName  string   `json:"packageName"`            // Primary package; the app identifier.
	Packages     []string `json:"packages"`               // All packages sharing this UID.
	Label        string   `json:"label"`                  // User visible app name.
	SharedUserID string   `json:"sharedUserId,omitempty"` // Manifest sharedUserId when several user apps share one.
	SystemApp    bool     `json:"systemApp"`
	VersionName  string   `json:"versionName"`

	// SourceDir is the path of the app's base APK, as reported by
	// ApplicationInfo.sourceDir.
	//
	// Note that this path is not stable: Android 9+ includes randomized
	// segments that change whenever the app is updated. Use PackageName, not
	// this path, to identify an app.
	SourceDir string `json:"sourceDir"`
}

// Name returns the user visible app name, falling back to the package name.
func (a *AppInfo) Name() string {
	switch {
	case a == nil:
		return ""
	case a.Label != "":
		return a.Label
	default:
		return a.PackageName
	}
}
