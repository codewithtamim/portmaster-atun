//go:build android

package profile

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/safing/portmaster/base/api"
	"github.com/safing/portmaster/base/config"
	"github.com/safing/portmaster/base/database"
	"github.com/safing/portmaster/base/database/query"
	"github.com/safing/portmaster/base/log"
)

const (
	profilesAPIPath     = "profiles"
	profileAPIPath      = "profiles/{source}/{id}"
	profilesEnsurePath  = "profiles/ensure"
	historySettingsPath = "history/settings"
	devicePolicyPath    = "device-policy"

	globalConfigProfileID = "global-config"
)

var (
	ensureProfileMu = sync.Mutex{}

	errProfileSourceAndID  = errors.New("source and id are required")
	errPackageNameRequired = errors.New("packageName is required")
	errBadDefaultAction    = errors.New("defaultAction must be permit or block")
	errDeleteGlobalConfig  = errors.New("cannot delete global-config")
)

// Snapshot is the HTTP payload for one app profile.
type Snapshot struct {
	Source              string   `json:"source"`
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Description         string   `json:"description"`
	Internal            bool     `json:"internal"`
	PackageName         string   `json:"packageName"`
	SharedUserID        string   `json:"sharedUserId,omitempty"`
	BlockInternet       bool     `json:"blockInternet"`
	BlockInternetSet    bool     `json:"blockInternetSet"`
	BlockOnMobile       bool     `json:"blockOnMobile"`
	BlockOnMobileSet    bool     `json:"blockOnMobileSet"`
	BlockOnLock         bool     `json:"blockOnLock"`
	BlockOnLockSet      bool     `json:"blockOnLockSet"`
	BlockOnWifi         bool     `json:"blockOnWifi"`
	BlockOnWifiSet      bool     `json:"blockOnWifiSet"`
	Endpoints           []string `json:"endpoints"`
	Lists               []string `json:"lists"`
	DefaultAction       string   `json:"defaultAction"`
	DefaultActionSet    bool     `json:"defaultActionSet"`
	UseSPN              bool     `json:"useSpn"`
	UseSPNSet           bool     `json:"useSpnSet"`
	RoutingAlgorithm    string   `json:"routingAlgorithm"`
	RoutingAlgorithmSet bool     `json:"routingAlgorithmSet"`
}

// ListSnapshot is GET /v1/profiles.
type ListSnapshot struct {
	Profiles []Snapshot `json:"profiles"`
}

type profilePutBody struct {
	Reset     bool                   `json:"reset"`
	Config    map[string]interface{} `json:"config"`
	ClearKeys []string               `json:"clearKeys"`
}

type profileEnsureBody struct {
	PackageName  string   `json:"packageName"`
	Name         string   `json:"name"`
	SharedUserID string   `json:"sharedUserId"`
	Members      []string `json:"members"`
}

// HistorySettingsSnapshot is GET/PUT /v1/history/settings.
type HistorySettingsSnapshot struct {
	Enable bool `json:"enable"`
}

type historySettingsBody struct {
	Enable *bool `json:"enable"`
}

// DevicePolicySnapshot is GET/PUT /v1/device-policy.
type DevicePolicySnapshot struct {
	DefaultAction   string   `json:"defaultAction"`
	BlockOnMobile   bool     `json:"blockOnMobile"`
	BlockOnLock     bool     `json:"blockOnLock"`
	GlobalEndpoints []string `json:"globalEndpoints"`
}

type devicePolicyBody struct {
	DefaultAction   *string  `json:"defaultAction"`
	BlockOnMobile   *bool    `json:"blockOnMobile"`
	BlockOnLock     *bool    `json:"blockOnLock"`
	GlobalEndpoints []string `json:"globalEndpoints"`
}

func registerProfileAPIEndpoints() error {
	if err := api.RegisterEndpoint(api.Endpoint{
		Name:        "App profiles",
		Description: "GET local and special app profiles with their config overlay.",
		Path:        profilesAPIPath,
		Read:        api.PermitUser,
		ReadMethod:  http.MethodGet,
		StructFunc:  handleProfileList,
	}); err != nil {
		return err
	}

	if err := api.RegisterEndpoint(api.Endpoint{
		Name:        "App profile",
		Description: "GET, PUT overlay, or DELETE one profile.",
		Path:        profileAPIPath,
		Read:        api.PermitUser,
		ReadMethod:  http.MethodGet,
		Write:       api.PermitUser,
		WriteMethod: http.MethodPut,
		StructFunc:  handleProfile,
	}); err != nil {
		return err
	}

	if err := api.RegisterEndpoint(api.Endpoint{
		Name:        "Ensure Android app profile",
		Description: "POST packageName to find or create a local profile with the android-package fingerprint.",
		Path:        profilesEnsurePath,
		Write:       api.PermitUser,
		WriteMethod: http.MethodPost,
		StructFunc:  handleEnsureProfile,
	}); err != nil {
		return err
	}

	if err := api.RegisterEndpoint(api.Endpoint{
		Name:        "History settings",
		Description: "GET/PUT history/enable.",
		Path:        historySettingsPath,
		Read:        api.PermitUser,
		ReadMethod:  http.MethodGet,
		Write:       api.PermitUser,
		WriteMethod: http.MethodPut,
		StructFunc:  handleHistorySettings,
	}); err != nil {
		return err
	}

	return api.RegisterEndpoint(api.Endpoint{
		Name:        "Device policy",
		Description: "GET/PUT defaultAction, block-on-mobile, block-on-lock, and global endpoints.",
		Path:        devicePolicyPath,
		Read:        api.PermitUser,
		ReadMethod:  http.MethodGet,
		Write:       api.PermitUser,
		WriteMethod: http.MethodPut,
		StructFunc:  handleDevicePolicy,
	})
}

func handleProfileList(_ *api.Request) (interface{}, error) {
	return ListSnapshot{Profiles: listSnapshots()}, nil
}

func handleProfile(ar *api.Request) (interface{}, error) {
	source := ProfileSource(ar.URLVars["source"])
	id := ar.URLVars["id"]
	if source == "" || id == "" {
		return nil, api.ErrorWithStatus(errProfileSourceAndID, http.StatusBadRequest)
	}

	switch ar.Method {
	case http.MethodPut, http.MethodPost:
		return putProfile(source, id, ar)
	case http.MethodDelete:
		return deleteProfileByID(source, id)
	default:
		snap, err := getSnapshot(source, id)
		if err != nil {
			return nil, err
		}
		return snap, nil
	}
}

func handleEnsureProfile(ar *api.Request) (interface{}, error) {
	var body profileEnsureBody
	if len(ar.InputData) > 0 {
		if err := json.Unmarshal(ar.InputData, &body); err != nil {
			return nil, api.ErrorWithStatus(err, http.StatusBadRequest)
		}
	}
	packageName := strings.TrimSpace(body.PackageName)
	if packageName == "" {
		return nil, api.ErrorWithStatus(errPackageNameRequired, http.StatusBadRequest)
	}
	sharedUserID := strings.TrimSpace(body.SharedUserID)
	if sharedUserID != "" {
		return EnsureSharedAndroidProfile(sharedUserID, strings.TrimSpace(body.Name), body.Members)
	}
	return ensureAndroidPackageProfile(packageName, strings.TrimSpace(body.Name))
}

func handleHistorySettings(ar *api.Request) (interface{}, error) {
	if ar.Method == http.MethodPut || ar.Method == http.MethodPost {
		var body historySettingsBody
		if len(ar.InputData) > 0 {
			if err := json.Unmarshal(ar.InputData, &body); err != nil {
				return nil, api.ErrorWithStatus(err, http.StatusBadRequest)
			}
		}
		if body.Enable != nil {
			if err := config.SetConfigOption(CfgOptionEnableHistoryKey, *body.Enable); err != nil {
				return nil, err
			}
		}
	}
	return currentHistorySettings(), nil
}

func handleDevicePolicy(ar *api.Request) (interface{}, error) {
	if ar.Method == http.MethodPut || ar.Method == http.MethodPost {
		var body devicePolicyBody
		if len(ar.InputData) > 0 {
			if err := json.Unmarshal(ar.InputData, &body); err != nil {
				return nil, api.ErrorWithStatus(err, http.StatusBadRequest)
			}
		}
		if body.DefaultAction != nil {
			action := *body.DefaultAction
			if action != DefaultActionPermitValue && action != DefaultActionBlockValue {
				return nil, api.ErrorWithStatus(errBadDefaultAction, http.StatusBadRequest)
			}
			if err := config.SetConfigOption(CfgOptionDefaultActionKey, action); err != nil {
				return nil, err
			}
		}
		if body.BlockOnMobile != nil {
			if err := config.SetConfigOption(CfgOptionBlockOnMobileKey, *body.BlockOnMobile); err != nil {
				return nil, err
			}
		}
		if body.BlockOnLock != nil {
			if err := config.SetConfigOption(CfgOptionBlockOnLockKey, *body.BlockOnLock); err != nil {
				return nil, err
			}
		}
		if body.GlobalEndpoints != nil {
			if err := config.SetConfigOption(CfgOptionEndpointsKey, body.GlobalEndpoints); err != nil {
				return nil, err
			}
		}
	}
	return currentDevicePolicy(), nil
}

func currentHistorySettings() HistorySettingsSnapshot {
	return HistorySettingsSnapshot{
		Enable: config.GetAsBool(CfgOptionEnableHistoryKey, true)(),
	}
}

func currentDevicePolicy() DevicePolicySnapshot {
	endpoints := config.GetAsStringArray(CfgOptionEndpointsKey, []string{})()
	if endpoints == nil {
		endpoints = []string{}
	}
	return DevicePolicySnapshot{
		DefaultAction:   config.GetAsString(CfgOptionDefaultActionKey, DefaultActionPermitValue)(),
		BlockOnMobile:   config.GetAsBool(CfgOptionBlockOnMobileKey, false)(),
		BlockOnLock:     config.GetAsBool(CfgOptionBlockOnLockKey, false)(),
		GlobalEndpoints: endpoints,
	}
}

// eachLocalProfile walks every live local profile. The callback returns false
// to stop early. Records that fail to load are logged and skipped; a query
// error, including one that ended the walk early, is returned.
func eachLocalProfile(fn func(*Profile) bool) error {
	it, err := profileDB.Query(query.New(ProfilesDBPath))
	if err != nil {
		log.Warningf("profile: failed to list profiles: %s", err)
		return err
	}
	defer it.Cancel()

	for r := range it.Next {
		if r.Meta() != nil && r.Meta().IsDeleted() {
			continue
		}
		p, err := EnsureProfile(r)
		if err != nil {
			log.Warningf("profile: failed to load profile %s: %s", r.DatabaseKey(), err)
			continue
		}
		if p.Source != SourceLocal || p.Internal {
			continue
		}
		if !fn(p) {
			return nil
		}
	}
	return it.Err()
}

func listSnapshots() []Snapshot {
	out := make([]Snapshot, 0, 16)
	_ = eachLocalProfile(func(p *Profile) bool {
		out = append(out, snapshotFromProfile(p))
		return true
	})
	return out
}

func getSnapshot(source ProfileSource, id string) (Snapshot, error) {
	p, err := getProfile(MakeScopedID(source, id))
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return Snapshot{}, api.ErrorWithStatus(err, http.StatusNotFound)
		}
		return Snapshot{}, err
	}
	return snapshotFromProfile(p), nil
}

func putProfile(source ProfileSource, id string, ar *api.Request) (interface{}, error) {
	if source == SourceSpecial && id == globalConfigProfileID {
		return putGlobalConfigProfile(ar)
	}

	p, err := getProfile(MakeScopedID(source, id))
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, api.ErrorWithStatus(err, http.StatusNotFound)
		}
		return nil, err
	}

	var body profilePutBody
	if len(ar.InputData) > 0 {
		if err := json.Unmarshal(ar.InputData, &body); err != nil {
			return nil, api.ErrorWithStatus(err, http.StatusBadRequest)
		}
	}

	p.Lock()
	if p.Config == nil || body.Reset {
		p.Config = make(map[string]interface{})
	}
	for _, key := range body.ClearKeys {
		deleteHierarchicalKey(p.Config, key)
	}
	for key, value := range flattenIncomingConfig(body.Config) {
		if value == nil {
			deleteHierarchicalKey(p.Config, key)
			continue
		}
		config.PutValueIntoHierarchicalConfig(p.Config, key, value)
	}
	p.dataParsed = false
	p.LastEdited = time.Now().Unix()
	p.savedInternally = false
	if err := p.parseConfig(); err != nil {
		p.Unlock()
		return nil, err
	}
	p.Unlock()

	if err := p.Save(); err != nil {
		return nil, err
	}
	return snapshotFromProfile(p), nil
}

func putGlobalConfigProfile(ar *api.Request) (interface{}, error) {
	var body profilePutBody
	if len(ar.InputData) > 0 {
		if err := json.Unmarshal(ar.InputData, &body); err != nil {
			return nil, api.ErrorWithStatus(err, http.StatusBadRequest)
		}
	}
	flat := flattenIncomingConfig(body.Config)
	if v, ok := flat[CfgOptionEndpointsKey]; ok {
		if err := config.SetConfigOption(CfgOptionEndpointsKey, asStringSlice(v)); err != nil {
			return nil, err
		}
	}
	if v, ok := flat[CfgOptionFilterListsKey]; ok {
		if err := config.SetConfigOption(CfgOptionFilterListsKey, asStringSlice(v)); err != nil {
			return nil, err
		}
	}
	if v, ok := flat[CfgOptionDefaultActionKey]; ok {
		if s, ok := v.(string); ok {
			if err := config.SetConfigOption(CfgOptionDefaultActionKey, s); err != nil {
				return nil, err
			}
		}
	}
	return getSnapshot(SourceSpecial, globalConfigProfileID)
}

func deleteProfileByID(source ProfileSource, id string) (interface{}, error) {
	if source == SourceSpecial && id == globalConfigProfileID {
		return nil, api.ErrorWithStatus(errDeleteGlobalConfig, http.StatusBadRequest)
	}
	p, err := getProfile(MakeScopedID(source, id))
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, api.ErrorWithStatus(err, http.StatusNotFound)
		}
		return nil, err
	}
	if err := p.delete(); err != nil {
		return nil, err
	}
	return map[string]string{"status": "deleted"}, nil
}

func snapshotFromProfile(p *Profile) Snapshot {
	p.RLock()
	defer p.RUnlock()

	flat := map[string]interface{}{}
	if p.Config != nil {
		flat = config.Flatten(p.Config)
	}

	blockInternet, blockInternetSet := overlayBool(flat, CfgOptionBlockScopeInternetKey)
	blockOnMobile, blockOnMobileSet := overlayBool(flat, CfgOptionBlockOnMobileKey)
	blockOnLock, blockOnLockSet := overlayBool(flat, CfgOptionBlockOnLockKey)
	blockOnWifi, blockOnWifiSet := overlayBool(flat, CfgOptionBlockOnWifiKey)
	useSPN, useSPNSet := overlayBool(flat, CfgOptionUseSPNKey)
	routing, routingSet := overlayString(flat, CfgOptionRoutingAlgorithmKey)
	defaultAction, defaultActionSet := overlayString(flat, CfgOptionDefaultActionKey)

	if !blockInternetSet {
		blockInternet = config.GetAsBool(CfgOptionBlockScopeInternetKey, false)()
	}
	if !blockOnMobileSet {
		blockOnMobile = config.GetAsBool(CfgOptionBlockOnMobileKey, false)()
	}
	if !blockOnLockSet {
		blockOnLock = config.GetAsBool(CfgOptionBlockOnLockKey, false)()
	}
	if !blockOnWifiSet {
		blockOnWifi = config.GetAsBool(CfgOptionBlockOnWifiKey, false)()
	}
	if !useSPNSet {
		useSPN = config.GetAsBool(CfgOptionUseSPNKey, true)()
	}
	if !routingSet {
		routing = config.GetAsString(CfgOptionRoutingAlgorithmKey, DefaultRoutingProfileID)()
	}
	if !defaultActionSet {
		defaultAction = config.GetAsString(CfgOptionDefaultActionKey, DefaultActionPermitValue)()
	}

	endpoints := overlayStringSlice(flat, CfgOptionEndpointsKey)
	if endpoints == nil {
		endpoints = []string{}
	}
	lists := overlayStringSlice(flat, CfgOptionFilterListsKey)
	if lists == nil {
		lists = []string{}
	}

	return Snapshot{
		Source:              string(p.Source),
		ID:                  p.ID,
		Name:                p.Name,
		Description:         p.Description,
		Internal:            p.Internal,
		PackageName:         fingerprintValue(p.Fingerprints, AndroidPackageTagKey),
		SharedUserID:        fingerprintValue(p.Fingerprints, AndroidSharedUserTagKey),
		BlockInternet:       blockInternet,
		BlockInternetSet:    blockInternetSet,
		BlockOnMobile:       blockOnMobile,
		BlockOnMobileSet:    blockOnMobileSet,
		BlockOnLock:         blockOnLock,
		BlockOnLockSet:      blockOnLockSet,
		BlockOnWifi:         blockOnWifi,
		BlockOnWifiSet:      blockOnWifiSet,
		Endpoints:           endpoints,
		Lists:               lists,
		DefaultAction:       defaultAction,
		DefaultActionSet:    defaultActionSet,
		UseSPN:              useSPN,
		UseSPNSet:           useSPNSet,
		RoutingAlgorithm:    routing,
		RoutingAlgorithmSet: routingSet,
	}
}

// fingerprintValue returns the value of the tag-ID fingerprint with key.
func fingerprintValue(prints []Fingerprint, key string) string {
	for _, fp := range prints {
		if fp.Type == FingerprintTypeTagID && fp.Key == key {
			return fp.Value
		}
	}
	return ""
}

// EnsureSharedAndroidProfile finds or creates the one profile for a sharedUserId
// and keeps an existing per-app block when a member is reinstalled.
func EnsureSharedAndroidProfile(sharedUserID, name string, members []string) (Snapshot, error) {
	ensureProfileMu.Lock()
	defer ensureProfileMu.Unlock()

	existing := findProfileByTag(AndroidSharedUserTagKey, sharedUserID)
	if existing == nil {
		display := name
		if display == "" {
			display = sharedUserID
		}
		existing = New(&Profile{
			Source: SourceLocal,
			Name:   display,
			Fingerprints: []Fingerprint{{
				Type:      FingerprintTypeTagID,
				Key:       AndroidSharedUserTagKey,
				Operation: FingerprintOperationEqualsID,
				Value:     sharedUserID,
			}},
			UsePresentationPath: false,
		})
		if err := existing.Save(); err != nil {
			return Snapshot{}, err
		}
	} else if err := renameProfile(existing, name); err != nil {
		return Snapshot{}, err
	}
	if err := adoptMemberOverlay(existing, members); err != nil {
		return Snapshot{}, err
	}
	dropMemberPackageProfiles(existing, members)
	return snapshotFromProfile(existing), nil
}

func renameProfile(p *Profile, name string) error {
	if p == nil || name == "" || p.Name == name {
		return nil
	}
	p.Lock()
	p.Name = name
	p.Unlock()
	return p.Save()
}

// adoptMemberOverlay copies a per-package block onto the group profile once.
// A later reinstall must not replace a block the group already has.
func adoptMemberOverlay(shared *Profile, members []string) error {
	shared.RLock()
	empty := len(shared.Config) == 0
	shared.RUnlock()
	if !empty {
		return nil
	}
	wanted := map[string]struct{}{}
	for _, member := range members {
		member = strings.TrimSpace(member)
		if member != "" {
			wanted[member] = struct{}{}
		}
	}
	if len(wanted) == 0 {
		return nil
	}
	var chosen map[string]interface{}
	for _, p := range listLocalProfiles() {
		if p.ScopedID() == shared.ScopedID() {
			continue
		}
		pkg := fingerprintValue(p.Fingerprints, AndroidPackageTagKey)
		if _, ok := wanted[pkg]; !ok {
			continue
		}
		p.RLock()
		if len(p.Config) == 0 {
			p.RUnlock()
			continue
		}
		copied := make(map[string]interface{}, len(p.Config))
		for k, v := range p.Config {
			copied[k] = v
		}
		p.RUnlock()
		flat := config.Flatten(copied)
		if blocked, _ := flat[CfgOptionBlockScopeInternetKey].(bool); blocked {
			chosen = copied
			break
		}
		if chosen == nil {
			chosen = copied
		}
	}
	if chosen == nil {
		return nil
	}
	shared.Lock()
	shared.Config = chosen
	shared.dataParsed = false
	shared.LastEdited = time.Now().Unix()
	err := shared.parseConfig()
	shared.Unlock()
	if err != nil {
		return err
	}
	return shared.Save()
}

func dropMemberPackageProfiles(shared *Profile, members []string) {
	wanted := map[string]struct{}{}
	for _, member := range members {
		member = strings.TrimSpace(member)
		if member != "" {
			wanted[member] = struct{}{}
		}
	}
	for _, p := range listLocalProfiles() {
		if p.ScopedID() == shared.ScopedID() {
			continue
		}
		if fingerprintValue(p.Fingerprints, AndroidSharedUserTagKey) != "" {
			continue
		}
		if !profileIsSharedMember(p, wanted) {
			continue
		}
		if err := p.delete(); err != nil {
			log.Warningf("profile: failed to drop per-package profile %s: %s", p.ScopedID(), err)
		}
	}
}

func findProfileByTag(key, value string) *Profile {
	if key == "" || value == "" {
		return nil
	}
	for _, p := range listLocalProfiles() {
		for _, fp := range p.Fingerprints {
			if fp.Type == FingerprintTypeTagID && fp.Key == key && fp.Value == value {
				return p
			}
		}
	}
	return nil
}

func listLocalProfiles() []*Profile {
	var out []*Profile
	_ = eachLocalProfile(func(p *Profile) bool {
		out = append(out, p)
		return true
	})
	return out
}

func ensureAndroidPackageProfile(packageName, name string) (Snapshot, error) {
	ensureProfileMu.Lock()
	defer ensureProfileMu.Unlock()

	if existing := findProfileByAndroidPackage(packageName); existing != nil {
		return snapshotFromProfile(existing), nil
	}

	display := name
	if display == "" {
		display = packageName
	}
	profile := New(&Profile{
		Source: SourceLocal,
		Name:   display,
		Fingerprints: []Fingerprint{{
			Type:      FingerprintTypeTagID,
			Key:       AndroidPackageTagKey,
			Operation: FingerprintOperationEqualsID,
			Value:     packageName,
		}},
		UsePresentationPath: false,
	})
	if err := profile.Save(); err != nil {
		return Snapshot{}, err
	}
	return snapshotFromProfile(profile), nil
}

func findProfileByAndroidPackage(packageName string) *Profile {
	if packageName == "" {
		return nil
	}
	var found *Profile
	_ = eachLocalProfile(func(p *Profile) bool {
		if fingerprintValue(p.Fingerprints, AndroidPackageTagKey) == packageName {
			found = p
			return false
		}
		return true
	})
	return found
}

// ResetAllLocalOverlays clears user overlays on every local app profile.
func ResetAllLocalOverlays() error {
	var resetErr error
	err := eachLocalProfile(func(p *Profile) bool {
		p.Lock()
		p.Config = make(map[string]interface{})
		p.dataParsed = false
		p.LastEdited = time.Now().Unix()
		p.savedInternally = false
		parseErr := p.parseConfig()
		p.Unlock()
		if parseErr != nil {
			resetErr = parseErr
			return false
		}
		if err := p.Save(); err != nil {
			resetErr = err
			return false
		}
		return true
	})
	if err != nil {
		return err
	}
	return resetErr
}

func flattenIncomingConfig(in map[string]interface{}) map[string]interface{} {
	if in == nil {
		return map[string]interface{}{}
	}
	out := make(map[string]interface{}, len(in))
	for key, value := range in {
		if strings.Contains(key, "/") {
			out[key] = value
			continue
		}
		if nested, ok := value.(map[string]interface{}); ok {
			for nestedKey, nestedVal := range config.Flatten(map[string]interface{}{key: nested}) {
				out[nestedKey] = nestedVal
			}
			continue
		}
		out[key] = value
	}
	return out
}

func overlayBool(flat map[string]interface{}, key string) (bool, bool) {
	v, ok := flat[key]
	if !ok || v == nil {
		return false, false
	}
	b, ok := v.(bool)
	if !ok {
		return false, false
	}
	return b, true
}

func overlayString(flat map[string]interface{}, key string) (string, bool) {
	v, ok := flat[key]
	if !ok || v == nil {
		return "", false
	}
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	return s, true
}

func overlayStringSlice(flat map[string]interface{}, key string) []string {
	v, ok := flat[key]
	if !ok || v == nil {
		return nil
	}
	return asStringSlice(v)
}

func asStringSlice(v interface{}) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []interface{}:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return []string{}
	}
}

func deleteHierarchicalKey(cfg map[string]interface{}, key string) {
	if cfg == nil || key == "" {
		return
	}
	parts := strings.Split(key, "/")
	if len(parts) == 1 {
		delete(cfg, key)
		return
	}
	sub := cfg
	for i, part := range parts {
		if i == len(parts)-1 {
			delete(sub, part)
			return
		}
		next, ok := sub[part].(map[string]interface{})
		if !ok {
			return
		}
		sub = next
	}
}

func profileIsSharedMember(p *Profile, wanted map[string]struct{}) bool {
	if p == nil {
		return false
	}
	pkg := fingerprintValue(p.Fingerprints, AndroidPackageTagKey)
	if _, ok := wanted[pkg]; ok {
		return true
	}
	path := p.LinkedPath + "\n" + p.PresentationPath
	for member := range wanted {
		if member == "" {
			continue
		}
		if strings.Contains(path, "/"+member+"-") || strings.Contains(path, "/"+member+"/") {
			return true
		}
	}
	return false
}
