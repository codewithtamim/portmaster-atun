// Package anotif delivers Portmaster warnings to Android.
// Desktop keeps the notifications module. Android registers a listener
// and never posts through that module.
package anotif

// Listener receives one warning after the incident threshold is reached.
type Listener interface {
	OnNotification(eventID string, title string, message string)
}
