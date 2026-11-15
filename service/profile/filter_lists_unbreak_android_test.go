//go:build android

package profile

import (
	"slices"
	"testing"
)

// A stored filter-list selection always carries the unbreak category, whoever
// writes it. An empty selection stays empty.
func TestFilterListsWithUnbreak(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   interface{}
		want []string
	}{
		{"unbreak is added", []interface{}{"TRAC", "MAL"}, []string{"TRAC", "MAL", "UNBREAK"}},
		{"unbreak is not repeated", []string{"UNBREAK", "TRAC"}, []string{"UNBREAK", "TRAC"}},
		{"an empty selection stays empty", []interface{}{}, []string{}},
		{"a value that is not a list is empty", "TRAC", []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := filterListsWithUnbreak(tt.in); !slices.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
