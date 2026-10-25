//go:build android

package filterlists

import (
	"encoding/json"
	"net/http"

	"github.com/safing/portmaster/base/api"
	"github.com/safing/portmaster/base/config"
)

// These mirror the config keys and defaults owned by service/profile. Importing
// profile here would create an import cycle (profile imports filterlists), so
// keep them in sync with profile/config.go and index.go.
const (
	filterListsAPIPath      = "filterlists"
	cfgOptionFilterListsKey = "filter/lists"
	unbreakCategoryID       = "UNBREAK"
)

var defaultFilterListsValue = []string{"TRAC", "MAL", "BAD", "UNBREAK"}

// Snapshot is the HTTP payload for the Filter Lists screen.
type Snapshot struct {
	Version         string     `json:"version"`
	LastUpdatedUnix int64      `json:"lastUpdatedUnix"`
	Selected        []string   `json:"selected"`
	Categories      []Category `json:"categories"`
	Sources         []Source   `json:"sources"`
}

type selectionBody struct {
	Selected []string `json:"selected"`
}

// startPlatform registers the filter-list API and lets the intel updater
// report whether lists are loaded and when they were last applied.
func startPlatform() error {
	updater := module.instance.IntelUpdates()
	updater.SetFilterListsReadyCheck(isLoaded)
	updater.SetFilterListsLastUpdatedCheck(getCacheLastUpdatedUnix)

	return registerAPIEndpoints()
}

func registerAPIEndpoints() error {
	return api.RegisterEndpoint(api.Endpoint{
		Name:        "Filter Lists",
		Description: "GET the filter-list catalog and selection, PUT selected IDs.",
		Path:        filterListsAPIPath,
		Read:        api.PermitUser,
		ReadMethod:  http.MethodGet,
		Write:       api.PermitUser,
		WriteMethod: http.MethodPut,
		StructFunc:  handleFilterLists,
	})
}

func handleFilterLists(ar *api.Request) (any, error) {
	if ar.Method == http.MethodPut || ar.Method == http.MethodPost {
		return putFilterLists(ar)
	}
	return currentSnapshot(), nil
}

func putFilterLists(ar *api.Request) (any, error) {
	var body selectionBody
	if len(ar.InputData) > 0 {
		if err := json.Unmarshal(ar.InputData, &body); err != nil {
			return nil, api.ErrorWithStatus(err, http.StatusBadRequest)
		}
	}
	selected := ensureUnbreak(body.Selected)
	if err := config.SetConfigOption(cfgOptionFilterListsKey, selected); err != nil {
		return nil, err
	}
	return currentSnapshot(), nil
}

func currentSnapshot() Snapshot {
	selected := config.GetAsStringArray(cfgOptionFilterListsKey, defaultFilterListsValue)()
	if selected == nil {
		selected = []string{}
	}

	snap := Snapshot{
		Selected:        selected,
		LastUpdatedUnix: snapshotLastUpdatedUnix(),
		Categories:      []Category{},
		Sources:         []Source{},
	}

	if ver, err := getCacheDatabaseVersion(); err == nil {
		snap.Version = ver.String()
	}

	index, err := getListIndexFromCache()
	if err != nil {
		return snap
	}

	index.RLock()
	defer index.RUnlock()
	snap.Categories = append([]Category(nil), index.Categories...)
	snap.Sources = append([]Source(nil), index.Sources...)
	if snap.Version == "" {
		snap.Version = index.Version
	}
	return snap
}

func snapshotLastUpdatedUnix() int64 {
	updated := getCacheLastUpdatedUnix()
	if updated > 0 {
		return updated
	}
	verRecord, err := loadCacheVersionRecord()
	if err != nil || verRecord.Meta() == nil {
		return 0
	}
	return verRecord.Meta().Modified
}

func ensureUnbreak(ids []string) []string {
	out := make([]string, 0, len(ids)+1)
	seen := make(map[string]struct{}, len(ids)+1)
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if _, ok := seen[unbreakCategoryID]; !ok {
		out = append(out, unbreakCategoryID)
	}
	return out
}
