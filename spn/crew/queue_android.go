//go:build android

package crew

// connectQueueSize is the flow-control window, in messages of up to
// readBufSize (1500) bytes, that a connection asks the SPN for.
//
// The terminal default of 50000 preallocates two pointer channels of 50000
// slots for every connection, about 800 KB each, which on a phone added up to
// hundreds of MB that the GC scans on every cycle. The window is also how much
// data can queue in front of everything else on the link: with 4096 messages
// (6 MB) per connection, heavy browsing queued over 15 s of data, so DNS
// lookups over the SPN timed out and the home hub ping failed. 1024 messages
// (1.5 MB) still allows about 60 Mbit/s per connection at a 200 ms multi-hop
// round trip.
const connectQueueSize = 1024
