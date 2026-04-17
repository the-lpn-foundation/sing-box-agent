package singbox

import "time"

// State represents the current state of the sing-box instance.
type State int

const (
	// StateStopped indicates the sing-box instance is not running.
	StateStopped State = iota

	// StateStarting indicates the sing-box instance is currently starting.
	StateStarting

	// StateRunning indicates the sing-box instance is fully operational.
	StateRunning

	// StateStopping indicates the sing-box instance is currently stopping.
	StateStopping

	// StateReloading indicates the sing-box instance is currently reloading configuration.
	StateReloading
)

// String returns the string representation of the state.
func (s State) String() string {
	switch s {
	case StateStopped:
		return "stopped"
	case StateStarting:
		return "starting"
	case StateRunning:
		return "running"
	case StateStopping:
		return "stopping"
	case StateReloading:
		return "reloading"
	default:
		return "unknown"
	}
}

// Status represents the current status of the sing-box instance.
type Status struct {
	// State is the current operational state.
	State State

	// Running is true if the instance is in a running state.
	Running bool

	// Uptime is the duration since the instance started.
	// Zero if the instance is not running.
	Uptime time.Duration

	// StartTime is the time when the instance was started.
	// Zero if the instance is not running.
	StartTime time.Time

	// ConfigHash is a hash of the currently loaded configuration.
	// Empty string if no configuration is loaded.
	ConfigHash string

	// LastError holds the last error encountered, if any.
	LastError string
}
