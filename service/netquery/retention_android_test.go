//go:build android

package netquery

import "testing"

func TestCappedRetentionDaysCapsAtThirty(t *testing.T) {
	t.Parallel()

	cases := map[int64]int64{
		-1:  30,
		0:   30,
		1:   1,
		7:   7,
		30:  30,
		31:  30,
		365: 30,
	}
	for in, want := range cases {
		if got := cappedRetentionDays(in); got != want {
			t.Errorf("cappedRetentionDays(%d) = %d, want %d", in, got, want)
		}
	}
}
