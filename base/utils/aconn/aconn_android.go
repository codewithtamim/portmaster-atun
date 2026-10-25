//go:build android

package aconn

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/safing/portmaster/base/log"
	"github.com/safing/portmaster/base/utils"
)

const (
	protocolTCP = 6
	protocolUDP = 17
)

var (
	ErrNoResolver          = errors.New("connection owner resolver not registered")
	ErrNoAppInfoProvider   = errors.New("app info provider not registered")
	ErrUnknownOwner        = errors.New("connection owner unknown")
	ErrUnknownApp          = errors.New("app unknown")
	ErrUnsupportedProtocol = errors.New("protocol not supported by the connection owner lookup")
	ErrInvalidAddress      = errors.New("invalid address")
)

var (
	resolverMu sync.RWMutex
	resolver   OwnerResolver

	appInfoMu       sync.RWMutex
	appInfoProvider AppInfoProvider
)

// RegisterOwnerResolver must be called from a VpnService; getConnectionOwnerUid
// only answers for flows routed through the caller's own VPN.
func RegisterOwnerResolver(r OwnerResolver) {
	resolverMu.Lock()
	defer resolverMu.Unlock()
	if utils.IsNilCallback(r) {
		resolver = nil
		return
	}
	resolver = r
}

func HasOwnerResolver() bool {
	resolverMu.RLock()
	defer resolverMu.RUnlock()
	return !utils.IsNilCallback(resolver)
}

// RegisterAppInfoProvider sets the PackageManager bridge. Answers cached from
// a previous provider are dropped.
func RegisterAppInfoProvider(p AppInfoProvider) {
	appInfoMu.Lock()
	if utils.IsNilCallback(p) {
		appInfoProvider = nil
	} else {
		appInfoProvider = p
	}
	appInfoMu.Unlock()

	InvalidateAppInfo(UnknownUID)
}

func HasAppInfoProvider() bool {
	appInfoMu.RLock()
	defer appInfoMu.RUnlock()
	return !utils.IsNilCallback(appInfoProvider)
}

// LookupUID looks up the UID from the TUN interface perspective (local is app side,
// remote is destination). Only TCP and UDP are supported.
func LookupUID(protocol uint8, localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16) (int, error) {
	switch protocol {
	case protocolTCP, protocolUDP:
		// Supported.
	default:
		return UnknownUID, fmt.Errorf("%w: %d", ErrUnsupportedProtocol, protocol)
	}

	if len(localIP) == 0 || len(remoteIP) == 0 {
		return UnknownUID, ErrInvalidAddress
	}

	resolverMu.RLock()
	r := resolver
	resolverMu.RUnlock()
	if utils.IsNilCallback(r) {
		return UnknownUID, ErrNoResolver
	}

	uid := r.GetConnectionOwnerUid(
		int32(protocol),
		localIP.String(), int32(localPort),
		remoteIP.String(), int32(remotePort),
	)
	if uid < 0 {
		return UnknownUID, ErrUnknownOwner
	}

	return int(uid), nil
}

// App info cache. Every new connection looks up its app, and each lookup is a
// JNI call into PackageManager plus a JSON decode, so answers are cached per
// UID. The app calls InvalidateAppInfo from its package receiver when an app
// is installed, updated or removed; the TTLs only bound staleness if a
// broadcast is missed.
const (
	appInfoTTL     = 10 * time.Minute
	appInfoMissTTL = 30 * time.Second
)

type appInfoEntry struct {
	info    *AppInfo
	err     error
	expires time.Time
}

var (
	appInfoCacheMu sync.Mutex
	appInfoCache   = make(map[int]appInfoEntry)
	appInfoFetch   singleflight.Group
)

// GetAppInfo returns the app that owns uid. The result is shared through the
// cache: callers must not modify it.
func GetAppInfo(uid int) (*AppInfo, error) {
	if uid < 0 {
		return nil, ErrUnknownApp
	}

	appInfoCacheMu.Lock()
	entry, ok := appInfoCache[uid]
	appInfoCacheMu.Unlock()
	if ok && time.Now().Before(entry.expires) {
		return entry.info, entry.err
	}

	// Concurrent first connections of one app share a single JNI call.
	v, err, _ := appInfoFetch.Do(strconv.Itoa(uid), func() (any, error) {
		return fetchAppInfo(uid)
	})
	if err != nil {
		return nil, err
	}
	info, ok := v.(*AppInfo)
	if !ok {
		return nil, fmt.Errorf("aconn: unexpected app info type %T", v)
	}
	return info, nil
}

// InvalidateAppInfo drops the cached answer for uid, or every answer when uid
// is negative.
func InvalidateAppInfo(uid int) {
	appInfoCacheMu.Lock()
	defer appInfoCacheMu.Unlock()
	if uid < 0 {
		clear(appInfoCache)
		return
	}
	delete(appInfoCache, uid)
}

func cacheAppInfo(uid int, info *AppInfo, err error, ttl time.Duration) {
	appInfoCacheMu.Lock()
	defer appInfoCacheMu.Unlock()
	appInfoCache[uid] = appInfoEntry{info: info, err: err, expires: time.Now().Add(ttl)}
}

// fetchAppInfo asks PackageManager through the provider. Unknown UIDs are
// cached briefly. A missing provider or a bad answer is not cached, so it
// recovers as soon as the provider does.
func fetchAppInfo(uid int) (*AppInfo, error) {
	appInfoMu.RLock()
	p := appInfoProvider
	appInfoMu.RUnlock()
	if utils.IsNilCallback(p) {
		return nil, ErrNoAppInfoProvider
	}

	jsonStr := p.GetAppInfoJSON(int32(uid))
	if strings.TrimSpace(jsonStr) == "" {
		err := fmt.Errorf("%w: uid %d", ErrUnknownApp, uid)
		cacheAppInfo(uid, nil, err, appInfoMissTTL)
		return nil, err
	}

	info := &AppInfo{}
	if err := json.Unmarshal([]byte(jsonStr), info); err != nil {
		log.Warningf("aconn: failed to parse app info for uid %d: %s", uid, err)
		return nil, fmt.Errorf("parse app info for uid %d: %w", uid, err)
	}

	// The provider may omit the UID; it is authoritative here either way.
	info.UID = uid

	if info.PackageName == "" {
		err := fmt.Errorf("%w: uid %d has no package name", ErrUnknownApp, uid)
		cacheAppInfo(uid, nil, err, appInfoMissTTL)
		return nil, err
	}

	cacheAppInfo(uid, info, nil, appInfoTTL)
	return info, nil
}
