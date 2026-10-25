//go:build android

package access

import (
	"encoding/json"
	"net/http"

	"github.com/safing/portmaster/base/api"
	"github.com/safing/portmaster/base/log"
)

type loginJSONBody struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// platformLoginCredentials reads SPN credentials from the JSON request body.
// The Android UI sends the PortAPI token in the Authorization header, so it
// cannot also use HTTP Basic for the SPN account.
func platformLoginCredentials(r *http.Request) (username, password string) {
	ar := api.GetAPIRequest(r)
	if ar == nil || len(ar.InputData) == 0 {
		return "", ""
	}

	var body loginJSONBody
	if err := json.Unmarshal(ar.InputData, &body); err != nil {
		log.Debugf("access: failed to parse SPN login body: %s", err)
		return "", ""
	}

	return body.Username, body.Password
}
