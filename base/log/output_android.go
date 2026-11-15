//go:build android

package log

import (
	"strconv"
	"strings"
	"sync"
)

// ClearLastUnexpectedLogs empties the in-memory log rings: the unexpected
// lines and the recent lines.
func ClearLastUnexpectedLogs() {
	lastUnexpectedLogsLock.Lock()
	for i := range lastUnexpectedLogs {
		lastUnexpectedLogs[i] = ""
	}
	lastUnexpectedLogsIndex = 0
	lastUnexpectedLogsLock.Unlock()

	recentLogs.clear()
}

// Recent log lines, kept in memory for the diagnostics report.
//
// The ten unexpected lines the report had are too few to work out what led to
// a problem, and a release build keeps routine lines out of logcat. This ring
// holds the last lines at whatever log level is set, so raising the level in
// the app gives a report with more in it. It is bounded in lines and in line
// length, and it is never written to disk.
const (
	recentLogCapacity = 300
	// A line with a long trace is cut, so one entry cannot hold megabytes.
	recentLogMaxLine = 2048
)

type logRing struct {
	lock    sync.Mutex
	entries []logRingEntry
	next    int
	full    bool
	// last points at the entry written last, or nil when the ring is empty.
	last *logRingEntry
}

// logRingEntry is one line, with how many lines like it followed directly.
type logRingEntry struct {
	line    string
	shape   string
	repeats int
}

func newLogRing(capacity int) *logRing {
	return &logRing{entries: make([]logRingEntry, capacity)}
}

// add stores a line. A line that reads like the one before it, apart from its
// numbers, is counted on that entry instead: an app retrying a blocked
// connection writes the same line thousands of times, and would otherwise
// push everything else out of the ring.
func (r *logRing) add(line string) {
	if len(line) > recentLogMaxLine {
		line = line[:recentLogMaxLine] + " [cut]"
	}
	shape := lineShape(line)

	r.lock.Lock()
	defer r.lock.Unlock()

	if r.last != nil && r.last.shape == shape {
		r.last.repeats++
		return
	}

	r.entries[r.next] = logRingEntry{line: line, shape: shape}
	r.last = &r.entries[r.next]
	r.next++
	if r.next == len(r.entries) {
		r.next = 0
		r.full = true
	}
}

// snapshot returns the held lines, oldest first.
func (r *logRing) snapshot() []string {
	r.lock.Lock()
	defer r.lock.Unlock()

	ordered := r.entries[:r.next]
	if r.full {
		ordered = append(append([]logRingEntry(nil), r.entries[r.next:]...), r.entries[:r.next]...)
	}
	out := make([]string, 0, len(ordered))
	for _, entry := range ordered {
		line := entry.line
		if entry.repeats > 0 {
			line += " (and " + strconv.Itoa(entry.repeats) + " more like it)"
		}
		out = append(out, line)
	}
	return out
}

func (r *logRing) clear() {
	r.lock.Lock()
	defer r.lock.Unlock()

	for i := range r.entries {
		r.entries[i] = logRingEntry{}
	}
	r.next = 0
	r.full = false
	r.last = nil
}

// lineShape is a line without what changes between repeats of the same
// event: the time and counter in front, and every number in it (ports,
// addresses, durations).
func lineShape(line string) string {
	if _, rest, found := strings.Cut(line, rightArrow); found {
		line = rest
	}
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return '#'
		}
		return r
	}, line)
}

var recentLogs = newLogRing(recentLogCapacity)

// GetRecentLogs returns the last log lines written at the current log level,
// oldest first. They are as written, so the caller redacts them before they
// leave the process.
func GetRecentLogs() []string {
	return recentLogs.snapshot()
}
