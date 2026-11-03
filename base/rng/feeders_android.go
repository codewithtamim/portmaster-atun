//go:build android

package rng

// runTickFeeder is off on Android. The tick feeder wakes every ~29 ms
// forever to harvest timing jitter, about 34 wakeups a second, which keeps
// the CPU and the Go runtime from sleeping and costs battery. The OS feeder
// already reseeds with full-strength entropy from getrandom.
const runTickFeeder = false
