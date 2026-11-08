// Package aconn provides connection owner attribution on Android via
// ConnectivityManager.getConnectionOwnerUid.
//
// https://developer.android.com/reference/android/net/ConnectivityManager#getConnectionOwnerUid(int,%20java.net.InetSocketAddress,%20java.net.InetSocketAddress)
package aconn

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

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

	// AndroidUser is the Android user the UID belongs to (uid / 100000).
	// Non-zero for a cloned app: the same package running as another user,
	// which must stay a separate firewall identity from the original.
	AndroidUser int `json:"androidUser,omitempty"`

	// Signer is the SHA-256 of the app's current signing certificate, in
	// lowercase hex. An app signed by several certificates reports one digest
	// over all of them. Empty when the Android side could not read it.
	Signer string `json:"signer,omitempty"`

	// SignerHistory lists the SHA-256 of every certificate in the app's
	// signing lineage, including the current one. A rotated key keeps its
	// older certificates here.
	SignerHistory []string `json:"signerHistory,omitempty"`
}

// SignedBySameDeveloper reports whether known, a signer digest stored
// earlier, is the app's current signer or part of its signing lineage.
func (a *AppInfo) SignedBySameDeveloper(known string) bool {
	if a == nil || known == "" {
		return false
	}
	known = strings.ToLower(known)
	if strings.ToLower(a.Signer) == known {
		return true
	}
	for _, d := range a.SignerHistory {
		if strings.ToLower(d) == known {
			return true
		}
	}
	return false
}

// Name returns the user visible app name, falling back to the package name.
func (a *AppInfo) Name() string {
	if a == nil {
		return ""
	}
	if label := SafeLabel(a.Label); label != "" {
		return label
	}
	return a.PackageName
}

// MaxLabelLength caps an app label, in characters.
const MaxLabelLength = 50

// SafeLabel makes an app label, which the app itself controls, safe to show
// in Portmaster's own text: only the first line is kept, control, format and
// bidi characters are dropped, runs of spaces are collapsed and the result is
// capped at MaxLabelLength characters.
func SafeLabel(label string) string {
	if i := strings.IndexAny(label, "\r\n\u0085\u2028\u2029"); i >= 0 {
		label = label[:i]
	}
	return capRunes(StripUnsafeText(label), MaxLabelLength)
}

// StripUnsafeText drops control, format and bidi characters and collapses
// all whitespace, including line breaks, to single spaces.
func StripUnsafeText(text string) string {
	var b strings.Builder
	space := false
	for _, r := range text {
		switch {
		case r == utf8.RuneError:
			continue
		case unicode.IsSpace(r) || unicode.In(r, unicode.Zl, unicode.Zp):
			space = b.Len() > 0
			continue
		case unicode.IsControl(r) || unicode.In(r, unicode.Cf, unicode.Co, unicode.Cs) || !unicode.IsPrint(r):
			continue
		}
		if space {
			b.WriteByte(' ')
			space = false
		}
		b.WriteRune(r)
	}
	return b.String()
}

func capRunes(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	runes := []rune(s)
	return strings.TrimSpace(string(runes[:limit-1])) + "\u2026"
}
