//go:build android

package updates

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/safing/portmaster/base/utils/aconn"
	"github.com/safing/portmaster/service/mgr"
)

// newCorruptedIntel returns an intel updater whose installed files are broken,
// and a server that offers the index in serveDir.
func newCorruptedIntel(t *testing.T, serveDir string) (*Updater, *httptest.Server) {
	t.Helper()
	installedDir := t.TempDir()
	if err := GenerateMockFolder(installedDir, "Test", "1.0.0", testPublished); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installedDir, "portmaster"), []byte("corrupted"), 0o600); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.FileServer(http.Dir(serveDir)))
	t.Cleanup(srv.Close)

	u, err := New(&testInstance{}, "Test", Config{
		Name:              intelUpdaterName,
		Directory:         installedDir,
		DownloadDirectory: t.TempDir(),
		PurgeDirectory:    t.TempDir(),
		IndexFile:         "index.json",
		AutoCheck:         true,
		AutoDownload:      true,
		AutoApply:         true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if u.corruptedInstallation == nil {
		t.Fatal("expected corrupted installation to be detected")
	}
	// Set after New, which only accepts HTTPS index URLs.
	u.cfg.IndexURLs = []string{srv.URL + "/index.json"}
	return u, srv
}

// setArtifactURLs points the artifacts of the index in dir at baseURL.
func setArtifactURLs(t *testing.T, dir, baseURL string) {
	t.Helper()
	path := filepath.Join(dir, "index.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var index map[string]any
	if err := json.Unmarshal(data, &index); err != nil {
		t.Fatal(err)
	}
	artifacts, _ := index["Artifacts"].([]any)
	for _, a := range artifacts {
		artifact, _ := a.(map[string]any)
		artifact["URLs"] = []string{baseURL + "/" + artifact["Filename"].(string)}
	}
	data, err = json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

var testPublished = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// generateNewerLists writes version 1.0.1 of the mock lists into dir, with one
// file the installed 1.0.0 lacks, so the update has something to download.
func generateNewerLists(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "new-list"), []byte("mock:new-list"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := GenerateMockFolder(dir, "Test", "1.0.1", testPublished.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
}

func runStage(t *testing.T, u *Updater) {
	t.Helper()
	m := mgr.New("updates stage test")
	if err := m.Do("stage", u.stageIntelForApproval); err != nil {
		t.Logf("stage returned: %s", err)
	}
}

func TestStageRepairsCorruptedIntelAtSameVersion(t *testing.T) {
	serveDir := t.TempDir()
	if err := GenerateMockFolder(serveDir, "Test", "1.0.0", testPublished); err != nil {
		t.Fatal(err)
	}
	u, srv := newCorruptedIntel(t, serveDir)
	// The broken file is fetched from the server too.
	setArtifactURLs(t, serveDir, srv.URL)

	runStage(t, u)

	if u.corruptedInstallation != nil {
		t.Fatalf("corruption not repaired: %v", u.corruptedInstallation)
	}
	if err := u.index.VerifyArtifacts(u.cfg.Directory); err != nil {
		t.Fatalf("installed files still broken: %v", err)
	}
	if state := u.Progress().State; state == ProgressAwaitingApproval {
		t.Fatalf("repair of the same version waited for approval")
	}
}

func TestStageWithCorruptedIntelAndNoIndexIsNotReady(t *testing.T) {
	u, _ := newCorruptedIntel(t, t.TempDir()) // Serves no index.
	u.setProgressState(ProgressReady)

	runStage(t, u)

	if state := u.Progress().State; state != ProgressFailed {
		t.Fatalf("state %q, want %q", state, ProgressFailed)
	}
}

func TestStageWithCorruptedIntelAndNewerIndexWaitsForApproval(t *testing.T) {
	serveDir := t.TempDir()
	generateNewerLists(t, serveDir)
	u, _ := newCorruptedIntel(t, serveDir)

	runStage(t, u)

	if state := u.Progress().State; state != ProgressAwaitingApproval {
		t.Fatalf("state %q, want %q", state, ProgressAwaitingApproval)
	}
	if u.corruptedInstallation == nil {
		t.Fatal("newer version installed without approval")
	}
}

// newHealthyIntel returns an intel updater with intact installed lists at
// version 1.0.0 and a server that offers the index in serveDir.
func newHealthyIntel(t *testing.T, serveDir string) *Updater {
	t.Helper()
	installedDir := t.TempDir()
	if err := GenerateMockFolder(installedDir, "Test", "1.0.0", testPublished); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.FileServer(http.Dir(serveDir)))
	t.Cleanup(srv.Close)
	setArtifactURLs(t, serveDir, srv.URL)

	u, err := New(&testInstance{}, "Test", Config{
		Name:              intelUpdaterName,
		Directory:         installedDir,
		DownloadDirectory: t.TempDir(),
		PurgeDirectory:    t.TempDir(),
		IndexFile:         "index.json",
		AutoCheck:         true,
		AutoDownload:      true,
		AutoApply:         true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if u.corruptedInstallation != nil {
		t.Fatalf("installed mock lists reported broken: %v", u.corruptedInstallation)
	}
	u.cfg.IndexURLs = []string{srv.URL + "/index.json"}
	return u
}

func setMetered(t *testing.T, metered bool) {
	t.Helper()
	state := `{"metered":false}`
	if metered {
		state = `{"metered":true}`
	}
	if err := aconn.UpdateDeviceStateJSON(state); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = aconn.UpdateDeviceStateJSON(`{"metered":false}`) })
}

func TestStageDownloadsUpdateOnUnmeteredNetwork(t *testing.T) {
	serveDir := t.TempDir()
	if err := GenerateMockFolder(serveDir, "Test", "1.0.1", testPublished.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	u := newHealthyIntel(t, serveDir)
	markDownloaded(t, u)
	setMetered(t, false)

	runStage(t, u)

	if state := u.Progress().State; state == ProgressAwaitingApproval {
		t.Fatal("update on an unmetered network waited for approval")
	}
	if got := u.index.Version; got != "1.0.1" {
		t.Fatalf("installed version %q, want 1.0.1", got)
	}
}

// markDownloaded records a finished download, as after the first one the user
// approved.
func markDownloaded(t *testing.T, u *Updater) {
	t.Helper()
	if err := os.WriteFile(u.firstDownloadMarker(), nil, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestStageFirstDownloadWaitsForApprovalOnUnmeteredNetwork(t *testing.T) {
	serveDir := t.TempDir()
	generateNewerLists(t, serveDir)
	u := newHealthyIntel(t, serveDir)
	setMetered(t, false)

	runStage(t, u)

	progress := u.Progress()
	if progress.State != ProgressAwaitingApproval {
		t.Fatalf("state %q, want %q", progress.State, ProgressAwaitingApproval)
	}
	if !progress.FirstDownload {
		t.Fatal("first download not reported")
	}
	if got := u.index.Version; got != "1.0.0" {
		t.Fatalf("installed version %q before approval, want 1.0.0", got)
	}
}

func TestApprovedDownloadIsRecorded(t *testing.T) {
	serveDir := t.TempDir()
	if err := GenerateMockFolder(serveDir, "Test", "1.0.1", testPublished.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	u := newHealthyIntel(t, serveDir)
	setMetered(t, false)

	m := mgr.New("updates approve test")
	if err := m.Do("approve", func(w *mgr.WorkerCtx) error {
		return u.updateAndUpgrade(w, u.getIndexURLsWithLock(), false, true)
	}); err != nil {
		t.Fatal(err)
	}

	if got := u.index.Version; got != "1.0.1" {
		t.Fatalf("installed version %q, want 1.0.1", got)
	}
	if !u.downloadedSinceInstall() {
		t.Fatal("approved download not recorded")
	}
	if u.Progress().FirstDownload {
		t.Fatal("first download still reported after a download")
	}
}

// A size check that fails must not leave a total smaller than the download,
// or the progress bar runs past 100%.
func TestUnsizedArtifactsDoNotPushProgressPastTheTotal(t *testing.T) {
	serveDir := t.TempDir()
	generateNewerLists(t, serveDir)
	u := newHealthyIntel(t, serveDir)
	files := http.FileServer(http.Dir(serveDir))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead && r.URL.Path != "/index.json" {
			http.Error(w, "size check refused", http.StatusServiceUnavailable)
			return
		}
		files.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	setArtifactURLs(t, serveDir, srv.URL)
	u.cfg.IndexURLs = []string{srv.URL + "/index.json"}
	setMetered(t, false)

	runStage(t, u)
	if progress := u.Progress(); progress.State != ProgressAwaitingApproval || progress.TotalBytes != 0 {
		t.Fatalf("staged %q with total %d, want %q with an unknown (0) total",
			progress.State, progress.TotalBytes, ProgressAwaitingApproval)
	}

	m := mgr.New("updates approve test")
	if err := m.Do("approve", func(w *mgr.WorkerCtx) error {
		return u.updateAndUpgrade(w, u.getIndexURLsWithLock(), false, true)
	}); err != nil {
		t.Fatal(err)
	}
	progress := u.Progress()
	if progress.DownloadedBytes == 0 || progress.DownloadedBytes > progress.TotalBytes {
		t.Fatalf("downloaded %d of %d bytes", progress.DownloadedBytes, progress.TotalBytes)
	}
}

func TestRepairIsNotRecordedAsDownload(t *testing.T) {
	serveDir := t.TempDir()
	if err := GenerateMockFolder(serveDir, "Test", "1.0.0", testPublished); err != nil {
		t.Fatal(err)
	}
	u, srv := newCorruptedIntel(t, serveDir)
	setArtifactURLs(t, serveDir, srv.URL)

	runStage(t, u)

	if u.downloadedSinceInstall() {
		t.Fatal("repair recorded as a download")
	}
}

func TestStageWaitsForApprovalOnMeteredNetwork(t *testing.T) {
	serveDir := t.TempDir()
	generateNewerLists(t, serveDir)
	u := newHealthyIntel(t, serveDir)
	setMetered(t, true)

	runStage(t, u)

	if state := u.Progress().State; state != ProgressAwaitingApproval {
		t.Fatalf("state %q, want %q", state, ProgressAwaitingApproval)
	}
	if got := u.index.Version; got != "1.0.0" {
		t.Fatalf("installed version %q on a metered network, want 1.0.0", got)
	}
}

func TestStageWithoutListsAndNoIndexFails(t *testing.T) {
	srv := httptest.NewServer(http.FileServer(http.Dir(t.TempDir()))) // Serves no index.
	t.Cleanup(srv.Close)
	u, err := New(&testInstance{}, "Test", Config{
		Name:              intelUpdaterName,
		Directory:         t.TempDir(),
		DownloadDirectory: t.TempDir(),
		PurgeDirectory:    t.TempDir(),
		IndexFile:         "index.json",
		AutoCheck:         true,
		AutoDownload:      true,
		AutoApply:         true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if u.index != nil && len(u.index.Artifacts) > 0 {
		t.Fatal("expected no installed lists")
	}
	u.cfg.IndexURLs = []string{srv.URL + "/index.json"}

	runStage(t, u)

	if state := u.Progress().State; state != ProgressFailed {
		t.Fatalf("state %q, want %q", state, ProgressFailed)
	}
}

func TestStageWithHealthyListsAndNoIndexKeepsState(t *testing.T) {
	serveDir := t.TempDir()
	if err := GenerateMockFolder(serveDir, "Test", "1.0.1", testPublished.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	u := newHealthyIntel(t, serveDir)
	// The server stops offering an index, as when it is unreachable.
	if err := os.Remove(filepath.Join(serveDir, "index.json")); err != nil {
		t.Fatal(err)
	}
	u.setProgressState(ProgressReady)

	runStage(t, u)

	if state := u.Progress().State; state != ProgressReady {
		t.Fatalf("state %q, want %q", state, ProgressReady)
	}
}

// Installed lists that cannot be checked are reported as failing only after
// repeated failures on a real network: offline or behind a sign-in portal the
// check cannot work, and their age says they are getting stale.
func TestCheckFailureShown(t *testing.T) {
	t.Parallel()

	wifi := aconn.DeviceState{Wifi: true}
	tests := []struct {
		name     string
		failures int32
		state    aconn.DeviceState
		want     bool
	}{
		{"first failure on wifi", 1, wifi, false},
		{"second failure on wifi", 2, wifi, true},
		{"second failure on mobile", 2, aconn.DeviceState{Cellular: true}, true},
		{"offline", 5, aconn.DeviceState{}, false},
		{"sign-in portal", 5, aconn.DeviceState{Wifi: true, Captive: true}, false},
	}
	for _, tc := range tests {
		if got := checkFailureShown(tc.failures, tc.state); got != tc.want {
			t.Errorf("%s: checkFailureShown = %t, want %t", tc.name, got, tc.want)
		}
	}
}

func TestBuiltInListsReportTheirReleaseDate(t *testing.T) {
	serveDir := t.TempDir()
	if err := GenerateMockFolder(serveDir, "Test", "1.0.0", testPublished); err != nil {
		t.Fatal(err)
	}
	u := newHealthyIntel(t, serveDir)

	if !u.BuiltInListsInUse() {
		t.Fatal("lists in place before any download not reported as built in")
	}
	if got := u.InstalledPublished(); !got.Equal(testPublished) {
		t.Fatalf("published %s, want %s", got, testPublished)
	}

	markDownloaded(t, u)
	if u.BuiltInListsInUse() {
		t.Fatal("lists still reported as built in after a download")
	}
}

func TestOtherUpdatersHaveNoBuiltInLists(t *testing.T) {
	u := &Updater{}
	if u.BuiltInListsInUse() {
		t.Fatal("an updater that is not intel reported built-in lists")
	}
	if got := u.InstalledPublished(); !got.IsZero() {
		t.Fatalf("published %s without an index, want zero", got)
	}
}

// A download the user approved continues without asking again after the app
// was killed, as soon as the next check runs.
func TestApprovedDownloadContinuesAfterRestart(t *testing.T) {
	serveDir := t.TempDir()
	if err := GenerateMockFolder(serveDir, "Test", "1.0.1", testPublished.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	u := newHealthyIntel(t, serveDir)
	setMetered(t, false)
	u.noteApproval()

	runStage(t, u)

	if state := u.Progress().State; state == ProgressAwaitingApproval {
		t.Fatal("approved download waited for approval again")
	}
	if got := u.index.Version; got != "1.0.1" {
		t.Fatalf("installed version %q, want 1.0.1", got)
	}
	if u.approvalPending() {
		t.Fatal("approval kept after its download finished")
	}
}

// An approved download that failed is tried again soon, not at the next
// hourly check.
func TestFailedApprovedDownloadIsRetried(t *testing.T) {
	serveDir := t.TempDir()
	// A file the installed version lacks, so it must be downloaded.
	if err := os.WriteFile(filepath.Join(serveDir, "new-list"), []byte("mock:new-list"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := GenerateMockFolder(serveDir, "Test", "1.0.1", testPublished.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	u := newHealthyIntel(t, serveDir)
	// The index is served, but its files are not, as when the network drops
	// after the index arrived.
	setArtifactURLs(t, serveDir, u.cfg.IndexURLs[0]+"/gone")
	setMetered(t, false)
	u.noteApproval()

	runStage(t, u)

	if got := u.index.Version; got != "1.0.0" {
		t.Fatalf("installed version %q, want 1.0.0", got)
	}
	if !u.approvalPending() {
		t.Fatal("approval dropped after a failed download")
	}
	if status := u.updateCheckWorkerMgr.Status(); status != "delayed" {
		t.Fatalf("check worker %q, want a retry scheduled", status)
	}
	if got := time.Duration(u.approvedRetryDelay.Load()); got != approvedRetryFirst {
		t.Fatalf("retry after %s, want %s", got, approvedRetryFirst)
	}
}

// Without a network the check itself fails; the approved download is still
// tried again soon.
func TestApprovedDownloadIsRetriedWhenTheCheckFails(t *testing.T) {
	serveDir := t.TempDir()
	if err := GenerateMockFolder(serveDir, "Test", "1.0.1", testPublished.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	u := newHealthyIntel(t, serveDir)
	if err := os.Remove(filepath.Join(serveDir, "index.json")); err != nil {
		t.Fatal(err)
	}
	setMetered(t, false)
	u.noteApproval()

	runStage(t, u)

	if status := u.updateCheckWorkerMgr.Status(); status != "delayed" {
		t.Fatalf("check worker %q, want a retry scheduled", status)
	}
}

// newIdleIntel returns an intel updater with intact lists and a server that
// offers the same version.
func newIdleIntel(t *testing.T) *Updater {
	t.Helper()
	serveDir := t.TempDir()
	if err := GenerateMockFolder(serveDir, "Test", "1.0.0", testPublished); err != nil {
		t.Fatal(err)
	}
	return newHealthyIntel(t, serveDir)
}

func TestApprovedDownloadRetriesWaitLongerUntilUsedUp(t *testing.T) {
	u := newIdleIntel(t)
	u.noteApproval()

	var waits []time.Duration
	for range 6 {
		u.endApprovedAttempt(errors.New("network down"))
		waits = append(waits, time.Duration(u.approvedRetryDelay.Load()))
	}

	want := []time.Duration{
		30 * time.Second, time.Minute, 2 * time.Minute, 4 * time.Minute,
		8 * time.Minute, 16 * time.Minute,
	}
	for i, w := range want {
		if waits[i] != w {
			t.Fatalf("wait %d is %s, want %s (all: %v)", i, waits[i], w, waits)
		}
	}
}

// A failure that keeps coming back ends the approval, so later checks ask the
// user again instead of downloading.
func TestApprovalEndsAfterRepeatedFailures(t *testing.T) {
	u := newIdleIntel(t)
	u.noteApproval()

	for range 7 {
		u.endApprovedAttempt(errors.New("bad signature"))
	}

	if u.approvalPending() {
		t.Fatal("approval kept after repeated failures")
	}
}

func TestOldApprovalIsIgnored(t *testing.T) {
	u := newIdleIntel(t)
	u.noteApproval()
	old := time.Now().Add(-approvalMaxAge - time.Minute)
	if err := os.Chtimes(u.approvalMarker(), old, old); err != nil {
		t.Fatal(err)
	}

	if u.approvalPending() {
		t.Fatal("approval from over a day ago still used")
	}
}

func TestNoRetryWhileAutomaticChecksAreOff(t *testing.T) {
	u := newIdleIntel(t)
	u.cfg.AutoCheck = false
	u.noteApproval()

	u.endApprovedAttempt(errors.New("network down"))

	if status := u.updateCheckWorkerMgr.Status(); status == "delayed" {
		t.Fatal("retry scheduled with automatic checks off")
	}
}

// The first download, with no lists installed, continues after a restart too.
func TestApprovedFirstDownloadContinuesWithoutLists(t *testing.T) {
	serveDir := t.TempDir()
	if err := GenerateMockFolder(serveDir, "Test", "1.0.1", testPublished.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.FileServer(http.Dir(serveDir)))
	t.Cleanup(srv.Close)
	setArtifactURLs(t, serveDir, srv.URL)
	u, err := New(&testInstance{}, "Test", Config{
		Name:              intelUpdaterName,
		Directory:         t.TempDir(),
		DownloadDirectory: t.TempDir(),
		PurgeDirectory:    t.TempDir(),
		IndexFile:         "index.json",
		AutoCheck:         true,
		AutoDownload:      true,
		AutoApply:         true,
	})
	if err != nil {
		t.Fatal(err)
	}
	u.cfg.IndexURLs = []string{srv.URL + "/index.json"}
	setMetered(t, false)
	u.noteApproval()

	runStage(t, u)

	if u.index == nil || u.index.Version != "1.0.1" {
		t.Fatal("approved first download did not continue")
	}
}

// Approving on Wi-Fi does not approve mobile data: there the user is asked again.
func TestApprovedDownloadWaitsForApprovalOnMeteredNetwork(t *testing.T) {
	serveDir := t.TempDir()
	generateNewerLists(t, serveDir)
	u := newHealthyIntel(t, serveDir)
	setMetered(t, true)
	u.noteApproval()

	runStage(t, u)

	if state := u.Progress().State; state != ProgressAwaitingApproval {
		t.Fatalf("state %q, want %q", state, ProgressAwaitingApproval)
	}
	if got := u.index.Version; got != "1.0.0" {
		t.Fatalf("installed version %q on mobile data, want 1.0.0", got)
	}
}

func TestApprovalEndsWhenNothingIsNewer(t *testing.T) {
	serveDir := t.TempDir()
	if err := GenerateMockFolder(serveDir, "Test", "1.0.0", testPublished); err != nil {
		t.Fatal(err)
	}
	u := newHealthyIntel(t, serveDir)
	setMetered(t, false)
	u.noteApproval()

	runStage(t, u)

	if u.approvalPending() {
		t.Fatal("approval kept with nothing to download")
	}
}

func TestFullDiskEndsTheApproval(t *testing.T) {
	u := newIdleIntel(t)
	u.noteApproval()

	u.endApprovedAttempt(fmt.Errorf("write: %w", syscall.ENOSPC))

	if u.approvalPending() {
		t.Fatal("approval kept after the disk filled up")
	}
	if status := u.updateCheckWorkerMgr.Status(); status == "delayed" {
		t.Fatal("retry scheduled on a full disk")
	}
}

// The size asked for is what will download: files already installed with the
// same content are copied from disk, not fetched.
func TestStagedSizeLeavesOutInstalledFiles(t *testing.T) {
	serveDir := t.TempDir()
	newList := []byte("mock:new-list")
	if err := os.WriteFile(filepath.Join(serveDir, "new-list"), newList, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := GenerateMockFolder(serveDir, "Test", "1.0.1", testPublished.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	u := newHealthyIntel(t, serveDir)
	setMetered(t, true)

	runStage(t, u)

	progress := u.Progress()
	if progress.State != ProgressAwaitingApproval {
		t.Fatalf("state %q, want %q", progress.State, ProgressAwaitingApproval)
	}
	if progress.TotalBytes != int64(len(newList)) {
		t.Fatalf("staged %d bytes, want %d for the one new file", progress.TotalBytes, len(newList))
	}

	m := mgr.New("updates approve test")
	if err := m.Do("approve", func(w *mgr.WorkerCtx) error {
		return u.updateAndUpgrade(w, u.getIndexURLsWithLock(), false, true)
	}); err != nil {
		t.Fatal(err)
	}
	progress = u.Progress()
	if progress.DownloadedBytes != int64(len(newList)) || progress.TotalBytes != int64(len(newList)) {
		t.Fatalf("downloaded %d of %d bytes, want %d of %d",
			progress.DownloadedBytes, progress.TotalBytes, len(newList), len(newList))
	}
}

// Broken installed files are fetched again, so they count toward the size.
func TestStagedSizeCountsBrokenInstalledFiles(t *testing.T) {
	serveDir := t.TempDir()
	if err := GenerateMockFolder(serveDir, "Test", "1.0.1", testPublished.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	u, srv := newCorruptedIntel(t, serveDir)
	setArtifactURLs(t, serveDir, srv.URL)

	runStage(t, u)

	var want int64
	for _, name := range []string{"portmaster", "portmaster-core", "portmaster.zip", "assets.zip"} {
		info, err := os.Stat(filepath.Join(serveDir, name))
		if err != nil {
			t.Fatal(err)
		}
		want += info.Size()
	}
	if progress := u.Progress(); progress.TotalBytes != want {
		t.Fatalf("staged %d bytes, want %d for every file (state %q)", progress.TotalBytes, want, progress.State)
	}
}

// A newer index whose files are all installed costs no download, so it is
// installed without asking, even on mobile data.
func TestNewerIndexWithInstalledFilesNeedsNoApproval(t *testing.T) {
	serveDir := t.TempDir()
	if err := GenerateMockFolder(serveDir, "Test", "1.0.1", testPublished.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	u := newHealthyIntel(t, serveDir)
	setMetered(t, true)

	runStage(t, u)

	if state := u.Progress().State; state == ProgressAwaitingApproval {
		t.Fatal("asked to approve a download of nothing")
	}
	if got := u.index.Version; got != "1.0.1" {
		t.Fatalf("installed version %q, want 1.0.1", got)
	}
}

// A file kept from an earlier attempt is copied, not fetched, and still moves
// the bar, so it reaches the staged total.
func TestFileFromAnEarlierAttemptCountsTowardTheStagedTotal(t *testing.T) {
	serveDir := t.TempDir()
	newList := []byte("mock:new-list")
	if err := os.WriteFile(filepath.Join(serveDir, "new-list"), newList, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := GenerateMockFolder(serveDir, "Test", "1.0.1", testPublished.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	u := newHealthyIntel(t, serveDir)
	setMetered(t, true)
	runStage(t, u)
	if err := os.WriteFile(filepath.Join(u.cfg.DownloadDirectory, "new-list"), newList, 0o600); err != nil {
		t.Fatal(err)
	}

	m := mgr.New("updates approve test")
	if err := m.Do("approve", func(w *mgr.WorkerCtx) error {
		return u.updateAndUpgrade(w, u.getIndexURLsWithLock(), false, true)
	}); err != nil {
		t.Fatal(err)
	}
	progress := u.Progress()
	if progress.DownloadedBytes != progress.TotalBytes || progress.TotalBytes != int64(len(newList)) {
		t.Fatalf("downloaded %d of %d bytes, want %d of %d",
			progress.DownloadedBytes, progress.TotalBytes, len(newList), len(newList))
	}
}

func TestBundleStorageShortfallReportsStorageInsteadOfOfferingDownload(t *testing.T) {
	serveDir := t.TempDir()
	generateNewerLists(t, serveDir)
	srv := httptest.NewServer(http.FileServer(http.Dir(serveDir)))
	t.Cleanup(srv.Close)
	setArtifactURLs(t, serveDir, srv.URL)

	installedDir := filepath.Join(t.TempDir(), "intel")
	if err := os.WriteFile(installedDir+".storage", []byte("117000000"), 0o600); err != nil {
		t.Fatal(err)
	}
	u, err := New(&testInstance{}, "Test", Config{
		Name:              intelUpdaterName,
		Directory:         installedDir,
		DownloadDirectory: t.TempDir(),
		PurgeDirectory:    t.TempDir(),
		IndexFile:         "index.json",
		AutoCheck:         true,
		AutoDownload:      true,
		AutoApply:         true,
	})
	if err != nil {
		t.Fatal(err)
	}
	u.cfg.IndexURLs = []string{srv.URL + "/index.json"}
	setMetered(t, false)

	runStage(t, u)

	progress := u.Progress()
	if progress.State != ProgressStorage {
		t.Fatalf("state %q, want %q", progress.State, ProgressStorage)
	}
	if progress.RequiredBytes != 117_000_000 {
		t.Fatalf("requiredBytes %d, want 117000000", progress.RequiredBytes)
	}

	// Once space is freed and the bundled lists are unpacked into place, the
	// updater adopts them without downloading from the server.
	if err := GenerateMockFolder(installedDir, "Test", "1.0.0", testPublished); err != nil {
		t.Fatal(err)
	}
	if !u.adoptBundledIntel() {
		t.Fatal("expected unpacked bundled lists to be adopted")
	}
	if _, err := os.Stat(u.bundleStorageMarker()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("storage marker still present: %v", err)
	}
	if u.index == nil || u.index.Version != "1.0.0" {
		t.Fatalf("adopted index %v, want version 1.0.0", u.index)
	}
}

func TestStageDoesNotOfferDownloadLargerThanAvailableStorage(t *testing.T) {
	serveDir := t.TempDir()
	generateNewerLists(t, serveDir)
	srv := httptest.NewServer(http.FileServer(http.Dir(serveDir)))
	t.Cleanup(srv.Close)
	setArtifactURLs(t, serveDir, srv.URL)

	u, err := New(&testInstance{}, "Test", Config{
		Name:              intelUpdaterName,
		Directory:         t.TempDir(),
		DownloadDirectory: t.TempDir(),
		PurgeDirectory:    t.TempDir(),
		IndexFile:         "index.json",
		AutoCheck:         true,
		AutoDownload:      true,
		AutoApply:         true,
	})
	if err != nil {
		t.Fatal(err)
	}
	u.cfg.IndexURLs = []string{srv.URL + "/index.json"}
	setMetered(t, false)

	prev := availableDiskBytes
	availableDiskBytes = func(string) (int64, bool) { return 8, true }
	t.Cleanup(func() { availableDiskBytes = prev })

	runStage(t, u)

	progress := u.Progress()
	if progress.State != ProgressStorage {
		t.Fatalf("state %q, want %q", progress.State, ProgressStorage)
	}
	if progress.RequiredBytes <= 8 {
		t.Fatalf("requiredBytes %d, want download size > 8", progress.RequiredBytes)
	}
}
