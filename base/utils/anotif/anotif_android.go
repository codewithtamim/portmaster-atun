//go:build android

package anotif

import (
	"sync"

	"github.com/safing/portmaster/base/utils"
)

var (
	callbackMu sync.Mutex
	listener   Listener
	delivered  = map[string]struct{}{}
)

// RegisterListener must be called before the engine starts.
// A nil listener clears the callback.
func RegisterListener(l Listener) {
	callbackMu.Lock()
	defer callbackMu.Unlock()
	if utils.IsNilCallback(l) {
		listener = nil
		return
	}
	listener = l
}

// Handles reports that this process delivers warnings itself.
func Handles() bool { return true }

// AlreadyDelivered is true after Deliver invoked the listener for eventID.
func AlreadyDelivered(eventID string) bool {
	callbackMu.Lock()
	defer callbackMu.Unlock()
	_, ok := delivered[eventID]
	return ok
}

// Deliver invokes the listener once per eventID.
// It returns false when no listener is registered, and does not mark the id delivered.
func Deliver(eventID, title, message string) bool {
	callbackMu.Lock()
	l := listener
	if utils.IsNilCallback(l) {
		callbackMu.Unlock()
		return false
	}
	if _, ok := delivered[eventID]; ok {
		callbackMu.Unlock()
		return true
	}
	delivered[eventID] = struct{}{}
	callbackMu.Unlock()
	l.OnNotification(eventID, title, message)
	return true
}

// Retract forgets eventID and tells the listener the warning is gone.
func Retract(eventID string) {
	callbackMu.Lock()
	l := listener
	delete(delivered, eventID)
	callbackMu.Unlock()
	if utils.IsNilCallback(l) {
		return
	}
	l.OnNotification(eventID, "", "")
}
