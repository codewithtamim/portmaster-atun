//go:build android

package profile

// AndroidPackageTagKey is the tag and fingerprint key for an Android package name.
const AndroidPackageTagKey = "android-package"

// AndroidSharedUserTagKey fingerprints a sharedUserId, not the numeric UID.
// A later app that is assigned the same UID does not inherit the group's rules.
const AndroidSharedUserTagKey = "android-shared-user"

// AndroidUIDTagKey records the current numeric UID. It is not a fingerprint.
const AndroidUIDTagKey = "android-uid"
