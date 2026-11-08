//go:build android

package atun

import (
	"testing"
	"time"
)

func resetFailFast() {
	failFastLock.Lock()
	failFastBuckets = make(map[int]*failFastBucket)
	failFastSwept = time.Time{}
	failFastLock.Unlock()
}

func TestFailFastBudgetThenDrops(t *testing.T) { //nolint:paralleltest // package state.
	resetFailFast()
	now := time.Now()

	for i := range failFastBurst {
		if !failFastAt(10312, now) {
			t.Fatalf("refusal %d within the burst was dropped", i+1)
		}
	}
	if failFastAt(10312, now) {
		t.Fatal("a refusal past the burst was not dropped")
	}

	// Another app keeps its own budget.
	if !failFastAt(10400, now) {
		t.Fatal("another app lost its budget")
	}

	// The budget comes back over time.
	if !failFastAt(10312, now.Add(time.Second)) {
		t.Fatal("the budget did not refill after a second")
	}
}

func TestFailFastUnknownOwnerAlwaysRefused(t *testing.T) { //nolint:paralleltest // package state.
	resetFailFast()
	for range failFastBurst * 3 {
		if !failFast(nil) {
			t.Fatal("a flow without a known owner was dropped")
		}
	}
}

func TestFailFastForgetsIdleApps(t *testing.T) { //nolint:paralleltest // package state.
	resetFailFast()
	now := time.Now()
	failFastAt(10312, now)
	failFastAt(10400, now.Add(2*failFastIdle))

	failFastLock.Lock()
	_, kept := failFastBuckets[10312]
	failFastLock.Unlock()
	if kept {
		t.Fatal("an idle app's budget was kept")
	}
}
