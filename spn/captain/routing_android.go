//go:build android

package captain

import (
	"encoding/json"
	"errors"
	"net/http"
	"sync"

	"github.com/safing/portmaster/base/api"
	"github.com/safing/portmaster/base/config"
	"github.com/safing/portmaster/service/profile"
	"github.com/safing/portmaster/spn/crew"
	"github.com/safing/portmaster/spn/navigator"
)

const spnRoutingAPIPath = "spn/routing"

// Errors.
var errBadRoutingAlgorithm = errors.New("routingAlgorithm must be single-hop, double-hop, triple-hop or home")

// RoutingSnapshot is GET /v1/spn/routing.
type RoutingSnapshot struct {
	Enable               bool   `json:"enable"`
	UseSPN               bool   `json:"useSpn"`
	RoutingAlgorithm     string `json:"routingAlgorithm"`
	Status               string `json:"status"`
	HomeHubID            string `json:"homeHubId"`
	HomeHubName          string `json:"homeHubName"`
	ConnectedIP          string `json:"connectedIp"`
	ConnectedCountryCode string `json:"connectedCountry"`
	ConnectedCountryName string `json:"connectedCountryName"`
	// StatusHops is the hop count of the last route that was established.
	// Zero means no connection has been routed yet, so the UI must not invent
	// a count from the selected algorithm.
	StatusHops int `json:"statusHops"`
	// ExitCountryName is the country of the exit hub on that route.
	ExitCountryName string `json:"exitCountryName"`
}

type routingPutBody struct {
	Enable           *bool   `json:"enable"`
	UseSPN           *bool   `json:"useSpn"`
	RoutingAlgorithm *string `json:"routingAlgorithm"`
}

func registerRoutingAPIEndpoint() error {
	crew.NoteRoute = rememberLiveRoute
	return api.RegisterEndpoint(api.Endpoint{
		Name:        "SPN routing",
		Description: "GET SPN enable, lockdown, hops and runtime status. PUT enable, use, algorithm.",
		Path:        spnRoutingAPIPath,
		Read:        api.PermitUser,
		ReadMethod:  http.MethodGet,
		Write:       api.PermitUser,
		WriteMethod: http.MethodPut,
		StructFunc:  handleSPNRouting,
	})
}

func handleSPNRouting(ar *api.Request) (any, error) {
	if ar.Method == http.MethodPut || ar.Method == http.MethodPost {
		var body routingPutBody
		if len(ar.InputData) > 0 {
			err := json.Unmarshal(ar.InputData, &body)
			if err != nil {
				return nil, api.ErrorWithStatus(err, http.StatusBadRequest)
			}
		}
		if body.Enable != nil {
			err := config.SetConfigOption(CfgOptionEnableSPNKey, *body.Enable)
			if err != nil {
				return nil, err
			}
		}
		if body.UseSPN != nil {
			err := config.SetConfigOption(profile.CfgOptionUseSPNKey, *body.UseSPN)
			if err != nil {
				return nil, err
			}
		}
		if body.RoutingAlgorithm != nil {
			algo := *body.RoutingAlgorithm
			switch algo {
			case navigator.RoutingProfileSingleHopID,
				navigator.RoutingProfileDoubleHopID,
				navigator.RoutingProfileTripleHopID,
				navigator.RoutingProfileHomeID:
			default:
				return nil, api.ErrorWithStatus(errBadRoutingAlgorithm, http.StatusBadRequest)
			}
			err := config.SetConfigOption(profile.CfgOptionRoutingAlgorithmKey, algo)
			if err != nil {
				return nil, err
			}
			// The previous path was chosen for the old algorithm. Forget it so
			// the next connection is found again, and stop advertising it.
			crew.ClearStickyHubs()
			rememberLiveRoute(0, "")
		}
	}
	return currentRoutingSnapshot(), nil
}

func currentRoutingSnapshot() RoutingSnapshot {
	st := GetSPNStatus()
	snap := RoutingSnapshot{
		Enable:           config.GetAsBool(CfgOptionEnableSPNKey, false)(),
		UseSPN:           config.GetAsBool(profile.CfgOptionUseSPNKey, true)(),
		RoutingAlgorithm: config.GetAsString(profile.CfgOptionRoutingAlgorithmKey, profile.DefaultRoutingProfileID)(),
		Status:           string(st.Status),
		HomeHubID:        st.HomeHubID,
		HomeHubName:      st.HomeHubName,
		ConnectedIP:      st.ConnectedIP,
	}
	if st.ConnectedCountry != nil {
		snap.ConnectedCountryCode = st.ConnectedCountry.Code
		snap.ConnectedCountryName = st.ConnectedCountry.Name
	}
	snap.StatusHops, snap.ExitCountryName = liveRoute()
	return snap
}

var (
	liveRouteMu     sync.Mutex
	liveHops        int
	liveExitCountry string
)

func rememberLiveRoute(hops int, exitCountry string) {
	liveRouteMu.Lock()
	changed := liveHops != hops || liveExitCountry != exitCountry
	liveHops = hops
	liveExitCountry = exitCountry
	liveRouteMu.Unlock()
	if !changed || spnStatusPushFunc == nil {
		return
	}
	// pushSPNStatusUpdate requires spnStatus to be locked.
	spnStatus.Lock()
	pushSPNStatusUpdate()
	spnStatus.Unlock()
}

func liveRoute() (int, string) {
	liveRouteMu.Lock()
	defer liveRouteMu.Unlock()
	return liveHops, liveExitCountry
}
