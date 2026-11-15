//go:build android

package status

import (
	"net/http"
	"slices"
	"strings"

	"github.com/safing/portmaster/base/api"
	"github.com/safing/portmaster/service/mgr"
)

// HealthProblem is one error a part of the engine reports while it runs.
// The app shows that the engine is not fully working; the IDs go into
// support reports, never into the app's own text.
type HealthProblem struct {
	Module string `json:"module"`
	ID     string `json:"id"`
}

// Health lists the engine's current errors.
type Health struct {
	Problems []HealthProblem `json:"problems"`
}

// shownElsewhere are errors the app already reports on their own, with their
// own wording and fix: the SPN screen covers SPN, the dashboard covers failing
// DNS servers and damaged filter list files.
var shownElsewhere = []string{
	"resolver:all-configured-resolvers-failed",
	"updates:corrupt-installation",
}

func (s *Status) registerPlatformAPI() error {
	return api.RegisterEndpoint(api.Endpoint{
		Name:        "Engine health",
		Description: "Errors parts of the engine report while running.",
		Path:        "engine/health",
		Read:        api.PermitUser,
		ReadMethod:  http.MethodGet,
		StructFunc: func(_ *api.Request) (any, error) {
			s.statesLock.Lock()
			defer s.statesLock.Unlock()
			updates := make([]mgr.StateUpdate, 0, len(s.states))
			for _, update := range s.states {
				updates = append(updates, update)
			}
			return Health{Problems: healthProblems(updates)}, nil
		},
	})
}

// healthProblems picks the error states the app should report as the engine
// not working fully, in a stable order.
func healthProblems(updates []mgr.StateUpdate) []HealthProblem {
	problems := []HealthProblem{}
	for _, update := range updates {
		for _, state := range update.States {
			if state.Type != mgr.StateTypeError ||
				strings.HasPrefix(state.ID, "spn:") ||
				slices.Contains(shownElsewhere, state.ID) {
				continue
			}
			problems = append(problems, HealthProblem{Module: update.Module, ID: state.ID})
		}
	}
	slices.SortFunc(problems, func(a, b HealthProblem) int {
		if c := strings.Compare(a.Module, b.Module); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	return problems
}
