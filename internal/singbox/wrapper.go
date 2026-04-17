package singbox

import (
	"context"
	"os"
	"sync"
	"time"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
)

// SingBox defines the interface for sing-box operations.
// This interface allows for both mock and real implementations.
type SingBox interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Reload(ctx context.Context, config *Config) error
	Status() Status
}

// Wrapper manages the sing-box instance lifecycle.
// It provides thread-safe operations on the sing-box instance.
type Wrapper struct {
	mu         sync.RWMutex
	instance   *box.Box
	configPath string
	config     *Config
	options    option.Options
	state      State
	startTime  time.Time
	configHash string
	lastError  string
}

// NewWrapper creates a new sing-box wrapper.
func NewWrapper(configPath string) *Wrapper {
	return &Wrapper{
		configPath: configPath,
		state:      StateStopped,
	}
}

func parseOptions(ctx context.Context, configJSON []byte) (option.Options, error) {
	var opts option.Options
	if err := opts.UnmarshalJSONContext(include.Context(ctx), configJSON); err != nil {
		return option.Options{}, err
	}
	return opts, nil
}

// Start starts the sing-box instance.
// Returns ErrAlreadyRunning if the instance is already running.
// The context is used to cancel the operation.
func (w *Wrapper) Start(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.state == StateRunning {
		return WrapError("start", ErrAlreadyRunning)
	}

	w.state = StateStarting

	config, err := LoadConfig(w.configPath)
	if err != nil {
		w.state = StateStopped
		w.lastError = err.Error()
		return WrapError("start", err)
	}

	configHash, err := config.Hash()
	if err != nil {
		w.state = StateStopped
		w.lastError = err.Error()
		return WrapError("start", err)
	}

	configJSON, err := os.ReadFile(w.configPath)
	if err != nil {
		w.state = StateStopped
		w.lastError = err.Error()
		return WrapError("start", err)
	}

	options, err := parseOptions(ctx, configJSON)
	if err != nil {
		w.state = StateStopped
		w.lastError = err.Error()
		return WrapError("start", ErrConfigInvalid)
	}

	w.config = config
	w.configHash = configHash
	w.options = options

	select {
	case <-ctx.Done():
		w.state = StateStopped
		w.lastError = ctx.Err().Error()
		return WrapError("start", ErrContextCanceled)
	default:
	}

	boxCtx := include.Context(ctx)

	instance, err := box.New(box.Options{Options: options, Context: boxCtx})
	if err != nil {
		w.state = StateStopped
		w.lastError = err.Error()
		return WrapError("start", ErrStartFailed)
	}

	if err := instance.Start(); err != nil {
		_ = instance.Close()
		w.state = StateStopped
		w.lastError = err.Error()
		return WrapError("start", ErrStartFailed)
	}

	w.instance = instance

	w.state = StateRunning
	w.startTime = time.Now()
	w.lastError = ""

	return nil
}

// Stop stops the sing-box instance.
// Returns ErrNotRunning if the instance is not running.
// The context is used to cancel the operation.
func (w *Wrapper) Stop(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.state != StateRunning {
		return WrapError("stop", ErrNotRunning)
	}

	w.state = StateStopping

	select {
	case <-ctx.Done():
		w.state = StateRunning
		w.lastError = ctx.Err().Error()
		return WrapError("stop", ErrContextCanceled)
	default:
	}

	if w.instance == nil {
		w.state = StateStopped
		w.startTime = time.Time{}
		w.lastError = ""
		return nil
	}

	if err := w.instance.Close(); err != nil {
		w.state = StateRunning
		w.lastError = err.Error()
		return WrapError("stop", ErrStopFailed)
	}

	w.instance = nil

	w.state = StateStopped
	w.startTime = time.Time{}
	w.lastError = ""

	return nil
}

// Reload reloads the sing-box configuration without a full restart.
// Returns ErrNotRunning if the instance is not running.
// The context is used to cancel the operation.
func (w *Wrapper) Reload(ctx context.Context, config *Config) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.state != StateRunning {
		return WrapError("reload", ErrNotRunning)
	}

	w.state = StateReloading

	configHash, err := config.Hash()
	if err != nil {
		w.state = StateRunning
		w.lastError = err.Error()
		return WrapError("reload", err)
	}

	if err := config.Validate(); err != nil {
		w.state = StateRunning
		w.lastError = err.Error()
		return WrapError("reload", ErrConfigInvalid)
	}

	configJSON, err := config.ToJSON()
	if err != nil {
		w.state = StateRunning
		w.lastError = err.Error()
		return WrapError("reload", ErrConfigInvalid)
	}

	newOptions, err := parseOptions(ctx, configJSON)
	if err != nil {
		w.state = StateRunning
		w.lastError = err.Error()
		return WrapError("reload", ErrConfigInvalid)
	}

	select {
	case <-ctx.Done():
		w.state = StateRunning
		w.lastError = ctx.Err().Error()
		return WrapError("reload", ErrContextCanceled)
	default:
	}

	if w.instance == nil {
		w.state = StateStopped
		w.lastError = ErrNotRunning.Error()
		return WrapError("reload", ErrNotRunning)
	}

	oldConfig := w.config
	oldOptions := w.options

	if err := w.instance.Close(); err != nil {
		w.state = StateRunning
		w.lastError = err.Error()
		return WrapError("reload", ErrReloadFailed)
	}

	boxCtx := include.Context(ctx)

	newInstance, err := box.New(box.Options{Options: newOptions, Context: boxCtx})
	if err != nil {
		rollbackInstance, rollbackErr := box.New(box.Options{Options: oldOptions, Context: boxCtx})
		if rollbackErr == nil {
			if startErr := rollbackInstance.Start(); startErr == nil {
				w.instance = rollbackInstance
				w.state = StateRunning
				w.lastError = err.Error()
				return WrapError("reload", ErrReloadFailed)
			}
			_ = rollbackInstance.Close()
		}

		w.instance = nil
		w.config = oldConfig
		w.state = StateStopped
		w.lastError = err.Error()
		return WrapError("reload", ErrReloadFailed)
	}

	if err := newInstance.Start(); err != nil {
		_ = newInstance.Close()

		rollbackInstance, rollbackErr := box.New(box.Options{Options: oldOptions, Context: boxCtx})
		if rollbackErr == nil {
			if startErr := rollbackInstance.Start(); startErr == nil {
				w.instance = rollbackInstance
				w.state = StateRunning
				w.lastError = err.Error()
				return WrapError("reload", ErrReloadFailed)
			}
			_ = rollbackInstance.Close()
		}

		w.instance = nil
		w.config = oldConfig
		w.state = StateStopped
		w.lastError = err.Error()
		return WrapError("reload", ErrReloadFailed)
	}

	w.instance = newInstance

	w.config = config
	w.options = newOptions
	w.configHash = configHash
	w.state = StateRunning
	w.lastError = ""

	return nil
}

// Status returns the current status of the sing-box instance.
func (w *Wrapper) Status() Status {
	w.mu.RLock()
	defer w.mu.RUnlock()

	uptime := time.Duration(0)
	if w.state == StateRunning && !w.startTime.IsZero() {
		uptime = time.Since(w.startTime)
	}

	return Status{
		State:      w.state,
		Running:    w.state == StateRunning,
		Uptime:     uptime,
		StartTime:  w.startTime,
		ConfigHash: w.configHash,
		LastError:  w.lastError,
	}
}

type MockSingBox struct {
	running bool
	mu      sync.Mutex
}

// Start starts the mock sing-box instance.
func (m *MockSingBox) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return ErrAlreadyRunning
	}

	m.running = true
	return nil
}

// Stop stops the mock sing-box instance.
func (m *MockSingBox) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return ErrNotRunning
	}

	m.running = false
	return nil
}

// Reload reloads the mock sing-box configuration.
func (m *MockSingBox) Reload(ctx context.Context, config *Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return ErrNotRunning
	}

	return nil
}

// Status returns the status of the mock sing-box instance.
func (m *MockSingBox) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()

	return Status{
		State:   StateRunning,
		Running: m.running,
	}
}
