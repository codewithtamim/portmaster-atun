//go:build android

package log

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestLogRingKeepsTheLastLinesInOrder(t *testing.T) {
	t.Parallel()

	ring := newLogRing(3)
	if got := ring.snapshot(); len(got) != 0 {
		t.Fatalf("a new ring holds %v", got)
	}

	ring.add("one")
	ring.add("two")
	if got := ring.snapshot(); !slices.Equal(got, []string{"one", "two"}) {
		t.Fatalf("before wrapping: %v", got)
	}

	for _, line := range []string{"three", "four", "five", "six", "seven"} {
		ring.add(line)
	}
	if got := ring.snapshot(); !slices.Equal(got, []string{"five", "six", "seven"}) {
		t.Fatalf("after wrapping: %v", got)
	}

	ring.clear()
	if got := ring.snapshot(); len(got) != 0 {
		t.Fatalf("a cleared ring holds %v", got)
	}
}

func TestLogRingCutsLongLines(t *testing.T) {
	t.Parallel()

	ring := newLogRing(1)
	ring.add(strings.Repeat("x", recentLogMaxLine*3))
	got := ring.snapshot()[0]
	if len(got) > recentLogMaxLine+16 || !strings.HasSuffix(got, "[cut]") {
		t.Fatalf("a long line was kept at %d bytes", len(got))
	}
}

// Every line the engine writes is kept for the report, also the routine ones
// a release build keeps out of logcat.
func TestWrittenLinesAreKeptWhateverLogcatDrops(t *testing.T) { //nolint:paralleltest // package state.
	recentLogs.clear()
	previous := logcatFilter
	logcatFilter = func(Severity, string) (string, bool) { return "", false }
	t.Cleanup(func() {
		logcatFilter = previous
		recentLogs.clear()
	})

	writeAndroidLog(InfoLevel, "routine line")
	writeAndroidLog(WarningLevel, "warning line")

	if got := GetRecentLogs(); !slices.Equal(got, []string{"routine line", "warning line"}) {
		t.Fatalf("kept %v", got)
	}
}

// A retry storm writes one line over and over, differing only in numbers. It
// must take one slot, so the lines before it stay in the report.
func TestLogRingCollapsesRepeatedLines(t *testing.T) {
	t.Parallel()

	ring := newLogRing(4)
	ring.add("251115 10:00:00.000 INF filter:123 ▶ 001 engine started")
	for i := range 500 {
		ring.add(fmt.Sprintf("251115 10:00:%02d.000 INF filter:123 ▶ %03d connection app:%d -> 203.0.113.7:443 blocked", i%60, i, 40000+i))
	}
	ring.add("251115 10:01:00.000 WRN core:9 ▶ 600 something else")

	got := ring.snapshot()
	if len(got) != 3 {
		t.Fatalf("held %d lines, want 3: %v", len(got), got)
	}
	if !strings.HasSuffix(got[0], "engine started") {
		t.Errorf("the line before the storm was lost: %q", got[0])
	}
	if !strings.HasSuffix(got[1], "(and 499 more like it)") {
		t.Errorf("the storm was not counted: %q", got[1])
	}
	if !strings.HasSuffix(got[2], "something else") {
		t.Errorf("the line after the storm is missing: %q", got[2])
	}
}

// Lines that differ in words are different events and both stay.
func TestLogRingKeepsDifferentLines(t *testing.T) {
	t.Parallel()

	ring := newLogRing(4)
	ring.add("connection to one.example blocked")
	ring.add("connection to two.example blocked")
	if got := ring.snapshot(); len(got) != 2 {
		t.Fatalf("held %v", got)
	}
}
