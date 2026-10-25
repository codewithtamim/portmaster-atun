//go:build android

package aconn

import (
	"errors"
	"sync/atomic"
	"testing"
)

type fakeAppInfo struct {
	calls atomic.Int32
	json  map[int32]string
}

func (f *fakeAppInfo) GetAppInfoJSON(uid int32) string {
	f.calls.Add(1)
	return f.json[uid]
}

func TestAppInfoCache(t *testing.T) {
	provider := &fakeAppInfo{json: map[int32]string{
		10100: `{"packageName":"org.example","label":"Example"}`,
	}}
	RegisterAppInfoProvider(provider)
	t.Cleanup(func() { RegisterAppInfoProvider(nil) })

	for range 3 {
		info, err := GetAppInfo(10100)
		if err != nil || info.PackageName != "org.example" {
			t.Fatalf("got %+v, %v", info, err)
		}
	}
	if got := provider.calls.Load(); got != 1 {
		t.Fatalf("provider called %d times for one app, want 1", got)
	}

	// An update changes the label; the package receiver invalidates the UID.
	provider.json[10100] = `{"packageName":"org.example","label":"Renamed"}`
	InvalidateAppInfo(10100)
	info, err := GetAppInfo(10100)
	if err != nil || info.Label != "Renamed" {
		t.Fatalf("after invalidate: got %+v, %v", info, err)
	}

	// Unknown UIDs are cached too, so they do not cost a JNI call each.
	for range 2 {
		if _, err := GetAppInfo(10999); !errors.Is(err, ErrUnknownApp) {
			t.Fatalf("got %v, want ErrUnknownApp", err)
		}
	}
	if got := provider.calls.Load(); got != 3 {
		t.Fatalf("provider called %d times, want 3", got)
	}
}

func TestAppInfoNotCachedWithoutProvider(t *testing.T) {
	RegisterAppInfoProvider(nil)
	if _, err := GetAppInfo(10200); !errors.Is(err, ErrNoAppInfoProvider) {
		t.Fatalf("got %v, want ErrNoAppInfoProvider", err)
	}

	// The first answer must not be stuck behind a cached "no provider".
	provider := &fakeAppInfo{json: map[int32]string{10200: `{"packageName":"org.late"}`}}
	RegisterAppInfoProvider(provider)
	t.Cleanup(func() { RegisterAppInfoProvider(nil) })
	if info, err := GetAppInfo(10200); err != nil || info.PackageName != "org.late" {
		t.Fatalf("got %+v, %v", info, err)
	}
}
