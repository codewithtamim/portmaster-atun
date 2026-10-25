//go:build android

package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/safing/portmaster/base/api"
	"github.com/safing/portmaster/base/config"
	"github.com/safing/portmaster/base/log"
	"github.com/safing/portmaster/base/utils/debug"
	"github.com/safing/portmaster/service/intel/filterlists"
	"github.com/safing/portmaster/service/netenv"
	"github.com/safing/portmaster/service/profile"
	"github.com/safing/portmaster/service/resolver"
	"github.com/safing/portmaster/spn/captain"
)

const (
	debugReportPath    = "debug/report"
	debugLogLevelPath  = "debug/log-level"
	debugLogsClearPath = "debug/logs/clear"
	configResetPath    = "config/reset"
)

type logLevelSnapshot struct {
	Level string `json:"level"`
}

type logLevelBody struct {
	Level string `json:"level"`
}

var errBadLogLevel = errors.New("level must be warning, info, or debug")

func registerAndroidDebugEndpoints() error {
	if err := api.RegisterEndpoint(api.Endpoint{
		Name:        "Android diagnostics report",
		Description: "Markdown support report without history, rules, keys, or account.",
		Path:        debugReportPath,
		Read:        api.PermitUser,
		ReadMethod:  http.MethodGet,
		DataFunc:    androidDebugReport,
	}); err != nil {
		return err
	}

	if err := api.RegisterEndpoint(api.Endpoint{
		Name:        "Log level",
		Description: "GET or PUT the engine log level: warning, info, or debug.",
		Path:        debugLogLevelPath,
		Read:        api.PermitUser,
		ReadMethod:  http.MethodGet,
		Write:       api.PermitUser,
		WriteMethod: http.MethodPut,
		StructFunc:  handleLogLevel,
	}); err != nil {
		return err
	}

	if err := api.RegisterEndpoint(api.Endpoint{
		Name:        "Clear unexpected logs",
		Description: "Clears the in-memory unexpected logs used by the diagnostics report.",
		Path:        debugLogsClearPath,
		Write:       api.PermitUser,
		WriteMethod: http.MethodPost,
		ActionFunc:  clearUnexpectedLogs,
	}); err != nil {
		return err
	}

	return api.RegisterEndpoint(api.Endpoint{
		Name:        "Reset Portmaster",
		Description: "Clears local app overlays and restores known global settings to defaults.",
		Path:        configResetPath,
		Write:       api.PermitUser,
		WriteMethod: http.MethodPost,
		StructFunc:  handleConfigReset,
	})
}

func androidDebugReport(ar *api.Request) ([]byte, error) {
	di := new(debug.Info)
	if ar != nil && ar.Request != nil {
		di.Style = ar.Request.URL.Query().Get("style")
	}

	di.AddVersionInfo()
	if ar != nil {
		di.AddPlatformInfo(ar.Context())
	}

	addRedactedUnexpectedLogs(di)
	addProtectionSection(di)
	addSPNSection(di)
	addFilterListsSection(di)
	addLogLevelSection(di)
	addSettingsSection(di)

	return di.Bytes(), nil
}

func handleLogLevel(ar *api.Request) (any, error) {
	if ar.Method == http.MethodPut || ar.Method == http.MethodPost {
		var body logLevelBody
		if len(ar.InputData) > 0 {
			if err := json.Unmarshal(ar.InputData, &body); err != nil {
				return nil, api.ErrorWithStatus(err, http.StatusBadRequest)
			}
		}
		level := log.ParseLevel(body.Level)
		switch level {
		case log.WarningLevel, log.InfoLevel, log.DebugLevel:
			log.SetLogLevel(level)
		default:
			return nil, api.ErrorWithStatus(errBadLogLevel, http.StatusBadRequest)
		}
	}
	return currentLogLevel(), nil
}

func clearUnexpectedLogs(_ *api.Request) (string, error) {
	log.ClearLastUnexpectedLogs()
	return "logs cleared", nil
}

func handleConfigReset(_ *api.Request) (any, error) {
	if err := profile.ResetAllLocalOverlays(); err != nil {
		return nil, err
	}
	keys := []string{
		profile.CfgOptionDefaultActionKey,
		profile.CfgOptionFilterListsKey,
		profile.CfgOptionEndpointsKey,
		profile.CfgOptionBlockOnMobileKey,
		profile.CfgOptionBlockOnLockKey,
		profile.CfgOptionEnableHistoryKey,
		profile.CfgOptionUseSPNKey,
		profile.CfgOptionRoutingAlgorithmKey,
		captain.CfgOptionEnableSPNKey,
		resolver.CfgOptionNameServersKey,
		resolver.CfgOptionUseSPNKey,
	}
	for _, key := range keys {
		if _, err := config.GetOption(key); err != nil {
			continue
		}
		// nil clears the user value so the platform default applies again.
		if err := config.SetConfigOption(key, nil); err != nil {
			return nil, err
		}
	}
	return map[string]string{"status": "reset"}, nil
}

func currentLogLevel() logLevelSnapshot {
	return logLevelSnapshot{Level: log.GetLogLevel().Name()}
}

func addRedactedUnexpectedLogs(di *debug.Info) {
	lines := log.GetLastUnexpectedLogs()
	if len(lines) == 0 {
		di.AddSection("No Unexpected Logs", debug.NoFlags)
		return
	}
	redacted := make([]string, 0, len(lines)+1)
	for _, line := range lines {
		redacted = append(redacted, redactForReport(line))
	}
	redacted = append(redacted, fmt.Sprintf("%s CURRENT TIME", time.Now().Format("060102 15:04:05.000")))
	di.AddSection("Unexpected Logs", debug.UseCodeSection|debug.AddContentLineBreaks, redacted...)
}

func addProtectionSection(di *debug.Info) {
	di.AddSection(
		fmt.Sprintf("Protection: %s", netenv.GetOnlineStatus()),
		debug.UseCodeSection|debug.AddContentLineBreaks,
		fmt.Sprintf("online: %s", netenv.GetOnlineStatus()),
	)
}

func addSPNSection(di *debug.Info) {
	enabled := config.GetAsBool(captain.CfgOptionEnableSPNKey, false)()
	st := captain.GetSPNStatus()
	lines := []string{
		fmt.Sprintf("enable: %v", enabled),
		fmt.Sprintf("status: %s", st.Status),
	}
	if st.ConnectedCountry != nil && st.ConnectedCountry.Code != "" {
		lines = append(lines, fmt.Sprintf("country: %s", st.ConnectedCountry.Code))
	}
	di.AddSection("SPN", debug.UseCodeSection|debug.AddContentLineBreaks, lines...)
}

func addFilterListsSection(di *debug.Info) {
	version, lastUpdated := filterlists.CacheVersion()
	if version == "" {
		version = "none"
	}
	updated := "never"
	if lastUpdated > 0 {
		updated = time.Unix(lastUpdated, 0).UTC().Format("2006-01-02 15:04")
	}
	di.AddSection(
		"Filter Lists",
		debug.UseCodeSection|debug.AddContentLineBreaks,
		fmt.Sprintf("version: %s", version),
		fmt.Sprintf("lastUpdated: %s", updated),
	)
}

func addLogLevelSection(di *debug.Info) {
	di.AddSection(
		"Log Level",
		debug.UseCodeSection,
		log.GetLogLevel().Name(),
	)
}

func addSettingsSection(di *debug.Info) {
	lines := make([]string, 0, 32)
	_ = config.ForEachOption(func(opt *config.Option) error {
		line, ok := settingLine(opt)
		if ok {
			lines = append(lines, line)
		}
		return nil
	})
	sort.Strings(lines)
	if len(lines) == 0 {
		di.AddSection("Settings", debug.NoFlags, "none")
		return
	}
	di.AddSection("Settings", debug.UseCodeSection|debug.AddContentLineBreaks, lines...)
}

func settingLine(opt *config.Option) (string, bool) {
	if opt == nil || opt.Sensitive {
		return "", false
	}
	switch opt.OptType {
	case config.OptTypeBool:
		return fmt.Sprintf("%s: %v", opt.Key, config.GetAsBool(opt.Key, false)()), true
	case config.OptTypeInt:
		return fmt.Sprintf("%s: %d", opt.Key, config.GetAsInt(opt.Key, 0)()), true
	case config.OptTypeString:
		if len(opt.PossibleValues) == 0 {
			return "", false
		}
		return fmt.Sprintf("%s: %s", opt.Key, config.GetAsString(opt.Key, "")()), true
	case config.OptTypeStringArray:
		return "", false
	default:
		return "", false
	}
}
