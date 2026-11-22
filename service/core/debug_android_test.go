//go:build android

package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/safing/portmaster/base/api"
	"github.com/safing/portmaster/base/config"
	"github.com/safing/portmaster/base/log"
)

type configTestInstance struct{ dataDir string }

func (i configTestInstance) DataDir() string                  { return i.dataDir }
func (i configTestInstance) SetCmdLineOperation(func() error) {}

// The level picked in diagnostics must be written to config.json, which the
// engine loads on every start; a level only held in memory is lost when the
// process restarts.
func TestHandleLogLevelPersistsLevel(t *testing.T) {
	dataDir := t.TempDir()
	cfg, err := config.New(configTestInstance{dataDir: dataDir})
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { log.SetLogLevel(log.InfoLevel) })

	req := &api.Request{
		Request:   httptest.NewRequest(http.MethodPut, "/"+debugLogLevelPath, nil),
		InputData: []byte(`{"level":"debug"}`),
	}
	if _, err := handleLogLevel(req); err != nil {
		t.Fatal(err)
	}

	if got := log.GetLogLevel(); got != log.DebugLevel {
		t.Errorf("running level = %s, want debug", got.Name())
	}
	data, err := os.ReadFile(filepath.Join(dataDir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved map[string]any
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	core, _ := saved["core"].(map[string]any)
	logCfg, _ := core["log"].(map[string]any)
	if got := logCfg["level"]; got != "debug" {
		t.Errorf("saved level = %v, want debug (config.json: %s)", got, data)
	}
}
