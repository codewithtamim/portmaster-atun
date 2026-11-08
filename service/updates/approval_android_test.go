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
