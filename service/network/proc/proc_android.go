//go:build android

// Package proc parses the Linux /proc filesystem to attribute sockets to
// processes.
//
// On Android only this file is built: an unprivileged app may neither read
// /proc/net/{tcp,udp}[6] nor the file descriptors of other UIDs. Android
// attribution goes through base/utils/aconn instead, which asks
// ConnectivityManager.getConnectionOwnerUid for the owning UID.
package proc
