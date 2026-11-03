//go:build android

package crew

// connectQueueSize is the flow-control window, in messages of up to
// readBufSize (1500) bytes, that a connection asks the SPN for.
//
// The terminal default of 50000 preallocates two pointer channels of 50000
// slots for every connection, about 800 KB each. On a phone that adds up to
// hundreds of MB during normal browsing, and the GC scans all of it on every
// cycle. 4096 messages is a window of about 6 MB, enough for more than
// 100 Mbit/s at a 300 ms multi-hop round trip, at 64 KB per connection.
const connectQueueSize = 4096
