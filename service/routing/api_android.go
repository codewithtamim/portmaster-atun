//go:build android

package routing

import (
	"encoding/json"
	"net/http"

	"github.com/safing/portmaster/base/api"
)

const routingAPIPath = "routing"

// Snapshot is GET /v1/routing.
type Snapshot struct {
	Target    string   `json:"target"`
	Available []string `json:"available"`
}

type putBody struct {
	Target *string `json:"target"`
}

func (r *Routing) registerAPIEndpoints() error {
	return api.RegisterEndpoint(api.Endpoint{
		Name:        "Routing target",
		Description: "GET the routing target and the targets this build supports. PUT target to change it.",
		Path:        routingAPIPath,
		Read:        api.PermitUser,
		ReadMethod:  http.MethodGet,
		Write:       api.PermitUser,
		WriteMethod: http.MethodPut,
		StructFunc:  handleRouting,
	})
}

func handleRouting(ar *api.Request) (interface{}, error) {
	if ar.Method == http.MethodPut || ar.Method == http.MethodPost {
		var body putBody
		if len(ar.InputData) > 0 {
			if err := json.Unmarshal(ar.InputData, &body); err != nil {
				return nil, api.ErrorWithStatus(err, http.StatusBadRequest)
			}
		}
		if body.Target != nil {
			if err := SetTarget(Target(*body.Target)); err != nil {
				return nil, api.ErrorWithStatus(err, http.StatusBadRequest)
			}
		}
	}

	available := make([]string, 0, len(Targets))
	for _, target := range Targets {
		available = append(available, target.String())
	}

	return Snapshot{
		Target:    Current().String(),
		Available: available,
	}, nil
}
