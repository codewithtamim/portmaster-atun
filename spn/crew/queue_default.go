//go:build !android

package crew

import "github.com/safing/portmaster/spn/terminal"

// connectQueueSize is the flow-control window a connection asks the SPN for.
const connectQueueSize = terminal.DefaultQueueSize
