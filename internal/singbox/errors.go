package singbox

import "errors"

// Error definitions for sing-box operations.
var (
	// ErrNotRunning is returned when trying to stop a sing-box instance that is not running.
	ErrNotRunning = errors.New("sing-box is not running")

	// ErrAlreadyRunning is returned when trying to start a sing-box instance that is already running.
	ErrAlreadyRunning = errors.New("sing-box is already running")

	// ErrConfigInvalid is returned when the sing-box configuration is invalid.
	ErrConfigInvalid = errors.New("invalid sing-box configuration")

	// ErrReloadFailed is returned when a reload operation fails.
	ErrReloadFailed = errors.New("sing-box reload failed")

	// ErrStartFailed is returned when starting sing-box fails.
	ErrStartFailed = errors.New("sing-box start failed")

	// ErrStopFailed is returned when stopping sing-box fails.
	ErrStopFailed = errors.New("sing-box stop failed")

	// ErrContextCanceled is returned when a context is canceled during an operation.
	ErrContextCanceled = errors.New("operation canceled")
)

// OperationError wraps an error with additional context about the operation that failed.
type OperationError struct {
	Operation string
	Err       error
}

func (e *OperationError) Error() string {
	return e.Operation + ": " + e.Err.Error()
}

func (e *OperationError) Unwrap() error {
	return e.Err
}

// WrapError wraps an error with operation context.
// If the error is nil, it returns nil.
func WrapError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return &OperationError{
		Operation: operation,
		Err:       err,
	}
}
