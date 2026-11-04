package ivpn

// This file has no build constraint, so that platforms without IVPN interop
// (Android) can still reference the database keys.

// Database key used to persist the user's choice to suppress the IVPN detected notification.
const Notification_DB_ID_IvpnDetectSuppressed = "core:notifications/interop/ivpn/suppressed"
