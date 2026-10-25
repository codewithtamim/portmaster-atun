//go:build android

package process

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/safing/portmaster/base/log"
	"github.com/safing/portmaster/base/utils/aconn"
	"github.com/safing/portmaster/service/profile"
)

// Errors.
var (
	ErrInvalidUID = errors.New("invalid uid")
	ErrNilProcess = errors.New("process getter returned nil")
)

// getAndroidProcessKey builds the internal storage key for an app identified
// by its UID. The shape cannot collide with the "<pid>-<createdAt>" key used
// on the other platforms.
func getAndroidProcessKey(uid int) string {
	return fmt.Sprintf("uid-%d", uid)
}

// pidForUID is the value shared firewall code reads as Process.Pid.
//
// Own traffic uses os.Getpid() so Pid == ownPID still matches without
// changing the desktop deciders. Every other app uses its UID plus
// androidAppPIDOffset, which is stable for the process lifetime and unique
// per app, so DNS request cache keys do not collide.
//
// androidAppPIDOffset sits above the kernel pid_max (1<<22). App UIDs are
// added to it so an app UID cannot equal Portmaster's own PID and skip filtering.
const androidAppPIDOffset = 1 << 22

func pidForUID(uid int) int {
	if uid == ownUID {
		return os.Getpid()
	}
	if uid < 0 {
		return uid
	}
	return uid + androidAppPIDOffset
}

// GetProcessWithProfileByUID returns the app that owns the given UID,
// including its profile.
// Always returns valid data.
// Errors are logged and returned for information or special handling purposes.
func GetProcessWithProfileByUID(ctx context.Context, uid int) (process *Process, err error) {
	if !enableProcessDetection() {
		log.Tracer(ctx).Tracef("process: process detection disabled")
		return GetUnidentifiedProcess(ctx), nil
	}

	process, err = GetOrFindProcessByUID(ctx, uid)
	if err != nil {
		log.Tracer(ctx).Debugf("process: failed to find process with UID: %s", err)
		return GetUnidentifiedProcess(ctx), err
	}

	changed, err := process.GetProfile(ctx)
	if err != nil {
		log.Tracer(ctx).Errorf("process: failed to get profile for process %s: %s", process, err)
	}

	if changed {
		process.Save()
	}

	return process, nil
}

// GetOrFindProcessByUID returns the app that owns the given UID.
//
// Android exposes no usable PID for other apps, so the app is built from the
// UID alone, with metadata supplied by the Android side through aconn.
func GetOrFindProcessByUID(ctx context.Context, uid int) (*Process, error) {
	log.Tracer(ctx).Tracef("process: getting process for UID %d", uid)

	switch uid {
	case UnidentifiedProcessID:
		return GetUnidentifiedProcess(ctx), nil
	case UnsolicitedProcessID:
		return GetUnsolicitedProcess(ctx), nil
	case UndefinedProcessID:
		return GetUnidentifiedProcess(ctx), nil
	}
	if uid < 0 {
		return nil, fmt.Errorf("%w: %d", ErrInvalidUID, uid)
	}

	key := getAndroidProcessKey(uid)

	// Singleflight: concurrent first connections of one app build one Process.
	p, err, _ := getProcessSingleInflight.Do(key, func() (interface{}, error) {
		return loadProcessByUID(ctx, key, uid)
	})
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, ErrNilProcess
	}

	return p.(*Process), nil // nolint:forcetypeassert // Can only be a *Process.
}

func loadProcessByUID(ctx context.Context, key string, uid int) (*Process, error) {
	if process, ok := GetProcessFromStorage(key); ok {
		return refreshAndroidIdentity(process), nil
	}

	info, err := aconn.GetAppInfo(uid)
	if err != nil {
		return nil, fmt.Errorf("failed to get app info for uid %d: %w", uid, err)
	}

	process := newAndroidProcess(uid, key, info, time.Now().Unix())
	if len(process.Tags) > 0 {
		log.Tracer(ctx).Debugf("profile: added tags: %+v", process.Tags)
	}

	process.Save()
	attachSharedProfile(info)
	return process, nil
}

// newAndroidProcess builds the Process of the app with the given UID, with
// its tags but without a profile.
func newAndroidProcess(uid int, key string, info *aconn.AppInfo, firstSeen int64) *Process {
	process := &Process{
		Pid:       pidForUID(uid),
		ParentPid: UndefinedProcessID,
		LeaderPid: UndefinedProcessID,

		UserID:   uid,
		UserName: info.PackageName,

		Name: info.Name(),
		// Path holds the app's base APK. The package name is what actually
		// identifies the app - APK paths carry randomized segments that change
		// on every update - so the package name is carried as a tag and
		// fingerprinted there, see service/process/tags/android.go.
		// A shared identity uses the sharedUserId, not one sibling's APK path.
		Path:     androidIdentityPath(info),
		ExecName: info.PackageName,

		FirstSeen:  firstSeen,
		LastSeen:   time.Now().Unix(),
		processKey: key,
	}
	process.addTags()
	return process
}

// appPath returns the path of the app's base APK, falling back to the package
// name if the Android side did not report one.
func appPath(info *aconn.AppInfo) string {
	if info.SourceDir != "" {
		return info.SourceDir
	}
	return info.PackageName
}

// androidIdentityPath is the stable path stored for an app.
// Shared UIDs must not keep one sibling's APK path.
func androidIdentityPath(info *aconn.AppInfo) string {
	if info != nil && info.SharedUserID != "" {
		return info.SharedUserID
	}
	return appPath(info)
}

// refreshAndroidIdentity returns p, or a replacement when the app's label or
// shared identity changed. The replacement has no profile yet, so the next
// lookup binds the group profile.
//
// p itself is never changed: connections already hold it and read its
// profile without locking, so it keeps the profile it was evaluated with.
func refreshAndroidIdentity(p *Process) *Process {
	if p.UserID < 0 {
		return p
	}
	info, err := aconn.GetAppInfo(p.UserID)
	if err != nil || info == nil {
		return p
	}

	p.Lock()
	shared, _ := p.GetTag(profile.AndroidSharedUserTagKey)
	unchanged := p.Name == info.Name() &&
		p.Path == androidIdentityPath(info) &&
		shared.Value == info.SharedUserID
	firstSeen := p.FirstSeen
	p.Unlock()
	if unchanged {
		return p
	}

	// Save replaces p in the process storage, as both share the key.
	fresh := newAndroidProcess(p.UserID, p.processKey, info, firstSeen)
	fresh.Save()
	attachSharedProfile(info)
	return fresh
}

// sharedProfileReady remembers a sharedUserId whose group profile was ensured
// in this process. A failed attempt is forgotten so the next lookup retries.
var sharedProfileReady sync.Map

func attachSharedProfile(info *aconn.AppInfo) {
	if info == nil || info.SharedUserID == "" {
		return
	}
	if _, loaded := sharedProfileReady.LoadOrStore(info.SharedUserID, struct{}{}); loaded {
		return
	}
	if _, err := profile.EnsureSharedAndroidProfile(info.SharedUserID, info.Name(), info.Packages); err != nil {
		sharedProfileReady.Delete(info.SharedUserID)
		log.Warningf("profile: shared identity %s: %s", info.SharedUserID, err)
	}
}
