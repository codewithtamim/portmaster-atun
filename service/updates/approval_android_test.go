//go:build android

package updates

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	if err := GenerateMockFolder(serveDir, "Test", "1.0.1", testPublished.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
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
	if err := GenerateMockFolder(serveDir, "Test", "1.0.1", testPublished.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
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
	if err := GenerateMockFolder(serveDir, "Test", "1.0.1", testPublished.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
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
	if err := GenerateMockFolder(serveDir, "Test", "1.0.1", testPublished.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
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
