package ivpn

// This file has no build constraint, so that platforms without IVPN interop
// (Android) can still reference the database keys.

// Database key used to persist the user's choice to suppress the IVPN detected notification.
const NotificationDBIDIvpnDetectSuppressed = "core:notifications/interop/ivpn/suppressed"
