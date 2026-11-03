//go:build android

package profile

import (
	"strconv"
	"strings"
)

// AndroidPackageTagKey is the tag and fingerprint key for an Android package name.
const AndroidPackageTagKey = "android-package"

// AndroidSharedUserTagKey fingerprints a sharedUserId, not the numeric UID.
// A later app that is assigned the same UID does not inherit the group's rules.
const AndroidSharedUserTagKey = "android-shared-user"

// AndroidUserPackageTagKey fingerprints a package that runs as another
// Android user, such as a cloned app, as "<package>@<user>". It keeps the
// clone's rules apart from the original app's.
const AndroidUserPackageTagKey = "android-user-package"

// AndroidUIDTagKey records the current numeric UID. It is not a fingerprint.
const AndroidUIDTagKey = "android-uid"

// SplitAndroidUserIdentity splits "<identity>@<user>" into its parts. An
// identity without a user suffix belongs to user 0.
func SplitAndroidUserIdentity(value string) (string, int) {
	at := strings.LastIndexByte(value, '@')
	if at <= 0 {
		return value, 0
	}
	user, err := strconv.Atoi(value[at+1:])
	if err != nil || user <= 0 {
		return value, 0
	}
	return value[:at], user
}

// AndroidUserIdentity is the inverse of SplitAndroidUserIdentity.
func AndroidUserIdentity(value string, user int) string {
	if user <= 0 {
		return value
	}
	return value + "@" + strconv.Itoa(user)
}
