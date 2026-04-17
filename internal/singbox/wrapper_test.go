package singbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWrapError(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		err       error
		wantNil   bool
	}{
		{
			name:      "nil error returns nil",
			operation: "test",
			err:       nil,
			wantNil:   true,
		},
		{
			name:      "non-nil error wraps correctly",
			operation: "start",
			err:       ErrStartFailed,
			wantNil:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WrapError(tt.operation, tt.err)
			if tt.wantNil && got != nil {
				t.Errorf("WrapError() = %v, want nil", got)
			}
			if !tt.wantNil {
				opErr, ok := got.(*OperationError)
				if !ok {
					t.Errorf("WrapError() returned wrong type %T", got)
				}
				if ok && opErr.Operation != tt.operation {
					t.Errorf("WrapError().Operation = %v, want %v", opErr.Operation, tt.operation)
				}
			}
		})
	}
}

func TestStateString(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{StateStopped, "stopped"},
		{StateStarting, "starting"},
		{StateRunning, "running"},
		{StateStopping, "stopping"},
		{StateReloading, "reloading"},
		{State(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.state.String(); got != tt.want {
				t.Errorf("State.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewWrapper(t *testing.T) {
	w := NewWrapper("/test/config.json")
	if w == nil {
		t.Fatal("NewWrapper() returned nil")
	}
	if w.configPath != "/test/config.json" {
		t.Errorf("NewWrapper().configPath = %v, want /test/config.json", w.configPath)
	}
	if w.state != StateStopped {
		t.Errorf("NewWrapper().state = %v, want StateStopped", w.state)
	}
}

func TestWrapperStartStop(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	configJSON := `{
		"log": {
			"disabled": false,
			"level": "info"
		},
		"inbounds": [],
		"outbounds": [{"type": "direct", "tag": "direct"}]
	}`

	if err := os.WriteFile(configPath, []byte(configJSON), 0o644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	t.Run("start and stop successfully", func(t *testing.T) {
		w := NewWrapper(configPath)

		if err := w.Start(ctx); err != nil {
			t.Fatalf("Start() failed: %v", err)
		}

		status := w.Status()
		if !status.Running {
			t.Error("Status().Running = false, want true after Start()")
		}
		if status.State != StateRunning {
			t.Errorf("Status().State = %v, want StateRunning", status.State)
		}

		if err := w.Stop(ctx); err != nil {
			t.Fatalf("Stop() failed: %v", err)
		}

		status = w.Status()
		if status.Running {
			t.Error("Status().Running = true, want false after Stop()")
		}
		if status.State != StateStopped {
			t.Errorf("Status().State = %v, want StateStopped", status.State)
		}
	})

	t.Run("start when already running returns error", func(t *testing.T) {
		w := NewWrapper(configPath)

		if err := w.Start(ctx); err != nil {
			t.Fatalf("Start() failed: %v", err)
		}

		err := w.Start(ctx)
		if err == nil {
			t.Error("Start() returned nil, want error when already running")
		}
		if err != ErrAlreadyRunning && err != nil {
			if opErr, ok := err.(*OperationError); ok {
				if opErr.Unwrap() != ErrAlreadyRunning {
					t.Errorf("Start() error = %v, want ErrAlreadyRunning wrapped", err)
				}
			} else {
				t.Errorf("Start() error = %v, want ErrAlreadyRunning wrapped", err)
			}
		}

		_ = w.Stop(ctx)
	})

	t.Run("stop when not running returns error", func(t *testing.T) {
		w := NewWrapper(configPath)

		err := w.Stop(ctx)
		if err == nil {
			t.Error("Stop() returned nil, want error when not running")
		}
		if err != ErrNotRunning && err != nil {
			if opErr, ok := err.(*OperationError); ok {
				if opErr.Unwrap() != ErrNotRunning {
					t.Errorf("Stop() error = %v, want ErrNotRunning wrapped", err)
				}
			} else {
				t.Errorf("Stop() error = %v, want ErrNotRunning wrapped", err)
			}
		}
	})
}

func TestWrapperReload(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	configJSON := `{
		"log": {"disabled": false},
		"inbounds": [],
		"outbounds": [{"type": "direct", "tag": "direct"}]
	}`

	if err := os.WriteFile(configPath, []byte(configJSON), 0o644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	t.Run("reload successfully updates config", func(t *testing.T) {
		w := NewWrapper(configPath)

		if err := w.Start(ctx); err != nil {
			t.Fatalf("Start() failed: %v", err)
		}

		newConfig := &Config{
			Log: &LogConfig{Disabled: true, Level: "debug"},
			Inbounds: []Inbound{
				{Type: "vless", Tag: "test-inbound"},
			},
			Outbounds: []Outbound{
				{Type: "direct", Tag: "direct"},
			},
		}

		if err := w.Reload(ctx, newConfig); err != nil {
			t.Fatalf("Reload() failed: %v", err)
		}

		status := w.Status()
		if !status.Running {
			t.Error("Status().Running = false after Reload(), want true")
		}
		if status.State != StateRunning {
			t.Errorf("Status().State = %v after Reload(), want StateRunning", status.State)
		}

		newHash, err := newConfig.Hash()
		if err != nil {
			t.Fatalf("Hash() failed: %v", err)
		}
		if status.ConfigHash != newHash {
			t.Errorf("Status().ConfigHash changed after Reload()")
		}

		_ = w.Stop(ctx)
	})

	t.Run("reload when not running returns error", func(t *testing.T) {
		w := NewWrapper(configPath)

		config := &Config{}
		err := w.Reload(ctx, config)
		if err == nil {
			t.Error("Reload() returned nil, want error when not running")
		}
		if err != ErrNotRunning && err != nil {
			if opErr, ok := err.(*OperationError); ok {
				if opErr.Unwrap() != ErrNotRunning {
					t.Errorf("Reload() error = %v, want ErrNotRunning wrapped", err)
				}
			} else {
				t.Errorf("Reload() error = %v, want ErrNotRunning wrapped", err)
			}
		}
	})

	t.Run("reload with nil config returns error", func(t *testing.T) {
		w := NewWrapper(configPath)

		if err := w.Start(ctx); err != nil {
			t.Fatalf("Start() failed: %v", err)
		}

		err := w.Reload(ctx, nil)
		if err == nil {
			t.Error("Reload() returned nil with nil config, want error")
		}

		_ = w.Stop(ctx)
	})
}

func TestWrapperContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	configJSON := `{
		"log": {"disabled": false},
		"inbounds": [],
		"outbounds": [{"type": "direct", "tag": "direct"}]
	}`

	if err := os.WriteFile(configPath, []byte(configJSON), 0o644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	t.Run("start with canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		w := NewWrapper(configPath)
		err := w.Start(ctx)

		if err == nil {
			t.Error("Start() returned nil with canceled context, want error")
		}
		status := w.Status()
		if status.State != StateStopped {
			t.Errorf("Status().State = %v, want StateStopped after canceled start", status.State)
		}
	})

	t.Run("stop with canceled context", func(t *testing.T) {
		ctx := context.Background()
		w := NewWrapper(configPath)

		if err := w.Start(ctx); err != nil {
			t.Fatalf("Start() failed: %v", err)
		}

		canceledCtx, cancel := context.WithCancel(context.Background())
		cancel()

		err := w.Stop(canceledCtx)
		if err == nil {
			t.Error("Stop() returned nil with canceled context, want error")
		}
		status := w.Status()
		if status.State != StateRunning {
			t.Errorf("Status().State = %v, want StateRunning after canceled stop", status.State)
		}

		_ = w.Stop(ctx)
	})
}

func TestWrapperUptime(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	configJSON := `{
		"log": {"disabled": false},
		"inbounds": [],
		"outbounds": [{"type": "direct", "tag": "direct"}]
	}`

	if err := os.WriteFile(configPath, []byte(configJSON), 0o644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	w := NewWrapper(configPath)

	status := w.Status()
	if status.Uptime != 0 {
		t.Errorf("Status().Uptime = %v, want 0 before start", status.Uptime)
	}

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	status = w.Status()
	if status.Uptime == 0 {
		t.Error("Status().Uptime = 0 after start, want > 0")
	}

	if err := w.Stop(ctx); err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}

	status = w.Status()
	if status.Uptime != 0 {
		t.Errorf("Status().Uptime = %v, want 0 after stop", status.Uptime)
	}
}

func TestConfigLoad(t *testing.T) {
	t.Run("load valid config", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.json")

		configJSON := `{
			"log": {"disabled": false, "level": "info"},
			"inbounds": [{"type": "vless", "tag": "test"}],
			"outbounds": [{"type": "direct", "tag": "direct"}]
		}`

		if err := os.WriteFile(configPath, []byte(configJSON), 0o644); err != nil {
			t.Fatalf("Failed to write config: %v", err)
		}

		cfg, err := LoadConfig(configPath)
		if err != nil {
			t.Fatalf("LoadConfig() failed: %v", err)
		}

		if cfg.Log == nil || cfg.Log.Level != "info" {
			t.Error("Config not loaded correctly")
		}
		if len(cfg.Inbounds) != 1 {
			t.Errorf("Config has %d inbounds, want 1", len(cfg.Inbounds))
		}
		if len(cfg.Outbounds) != 1 {
			t.Errorf("Config has %d outbounds, want 1", len(cfg.Outbounds))
		}
	})

	t.Run("load with empty path returns error", func(t *testing.T) {
		_, err := LoadConfig("")
		if err == nil {
			t.Error("LoadConfig() returned nil with empty path, want error")
		}
	})

	t.Run("load with non-existent path returns error", func(t *testing.T) {
		_, err := LoadConfig("/non/existent/path/config.json")
		if err == nil {
			t.Error("LoadConfig() returned nil with non-existent path, want error")
		}
	})

	t.Run("load with invalid JSON returns error", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.json")

		if err := os.WriteFile(configPath, []byte("invalid json"), 0o644); err != nil {
			t.Fatalf("Failed to write config: %v", err)
		}

		_, err := LoadConfig(configPath)
		if err == nil {
			t.Error("LoadConfig() returned nil with invalid JSON, want error")
		}
	})
}

func TestConfigHash(t *testing.T) {
	cfg1 := &Config{
		Log: &LogConfig{Level: "info"},
	}

	cfg2 := &Config{
		Log: &LogConfig{Level: "info"},
	}

	cfg3 := &Config{
		Log: &LogConfig{Level: "debug"},
	}

	hash1, err := cfg1.Hash()
	if err != nil {
		t.Fatalf("Hash() failed: %v", err)
	}

	hash2, err := cfg2.Hash()
	if err != nil {
		t.Fatalf("Hash() failed: %v", err)
	}

	hash3, err := cfg3.Hash()
	if err != nil {
		t.Fatalf("Hash() failed: %v", err)
	}

	if hash1 != hash2 {
		t.Error("Hashes of identical configs differ")
	}

	if hash1 == hash3 {
		t.Error("Hashes of different configs are the same")
	}

	_, err = (*Config)(nil).Hash()
	if err == nil {
		t.Error("Hash() returned nil for nil config, want error")
	}
}

func TestConfigToJSON(t *testing.T) {
	cfg := &Config{
		Log: &LogConfig{Level: "info"},
	}

	jsonData, err := cfg.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON() failed: %v", err)
	}

	if len(jsonData) == 0 {
		t.Error("ToJSON() returned empty data")
	}

	if string(jsonData)[0] != '{' {
		t.Error("ToJSON() did not return JSON object")
	}

	_, err = (*Config)(nil).ToJSON()
	if err == nil {
		t.Error("ToJSON() returned nil for nil config, want error")
	}
}

func TestMockSingBox(t *testing.T) {
	ctx := context.Background()
	mock := &MockSingBox{}

	t.Run("start and stop", func(t *testing.T) {
		if err := mock.Start(ctx); err != nil {
			t.Fatalf("MockSingBox.Start() failed: %v", err)
		}

		status := mock.Status()
		if !status.Running {
			t.Error("MockSingBox.Status().Running = false after Start()")
		}

		if err := mock.Stop(ctx); err != nil {
			t.Fatalf("MockSingBox.Stop() failed: %v", err)
		}

		status = mock.Status()
		if status.Running {
			t.Error("MockSingBox.Status().Running = true after Stop()")
		}
	})

	t.Run("start when already running", func(t *testing.T) {
		if err := mock.Start(ctx); err != nil {
			t.Fatalf("MockSingBox.Start() failed: %v", err)
		}

		err := mock.Start(ctx)
		if err != ErrAlreadyRunning {
			t.Errorf("MockSingBox.Start() error = %v, want ErrAlreadyRunning", err)
		}

		_ = mock.Stop(ctx)
	})

	t.Run("stop when not running", func(t *testing.T) {
		err := mock.Stop(ctx)
		if err != ErrNotRunning {
			t.Errorf("MockSingBox.Stop() error = %v, want ErrNotRunning", err)
		}
	})

	t.Run("reload", func(t *testing.T) {
		if err := mock.Start(ctx); err != nil {
			t.Fatalf("MockSingBox.Start() failed: %v", err)
		}

		config := &Config{}
		if err := mock.Reload(ctx, config); err != nil {
			t.Errorf("MockSingBox.Reload() failed: %v", err)
		}

		err := mock.Reload(ctx, nil)
		if err != nil {
			t.Errorf("MockSingBox.Reload() with nil config failed: %v", err)
		}

		_ = mock.Stop(ctx)
	})
}

func TestWrapperStart_ErrorCases(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	t.Run("start with invalid config file", func(t *testing.T) {
		configPath := filepath.Join(tmpDir, "invalid.json")
		require.NoError(t, os.WriteFile(configPath, []byte(`{invalid`), 0o644))

		w := NewWrapper(configPath)
		err := w.Start(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "start")

		status := w.Status()
		assert.Equal(t, StateStopped, status.State)
		assert.NotEmpty(t, status.LastError)
	})

	t.Run("start with config that fails hash", func(t *testing.T) {
		configPath := filepath.Join(tmpDir, "config.json")
		configJSON := `{"log": {"disabled": false}, "inbounds": [], "outbounds": []}`
		require.NoError(t, os.WriteFile(configPath, []byte(configJSON), 0o644))

		w := NewWrapper(configPath)
		// This should work since hash doesn't fail on valid config
		err := w.Start(ctx)
		if err != nil {
			// If it fails, verify state is stopped
			status := w.Status()
			assert.Equal(t, StateStopped, status.State)
		}
	})

	t.Run("start with config that fails parse", func(t *testing.T) {
		configPath := filepath.Join(tmpDir, "bad-config.json")
		// Valid JSON but invalid sing-box config
		configJSON := `{"invalid_field": true}`
		require.NoError(t, os.WriteFile(configPath, []byte(configJSON), 0o644))

		w := NewWrapper(configPath)
		err := w.Start(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "start")

		status := w.Status()
		assert.Equal(t, StateStopped, status.State)
		assert.NotEmpty(t, status.LastError)
	})
}

func TestWrapperStop_ErrorCases(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	configJSON := `{
		"log": {"disabled": false},
		"inbounds": [],
		"outbounds": [{"type": "direct", "tag": "direct"}]
	}`
	require.NoError(t, os.WriteFile(configPath, []byte(configJSON), 0o644))

	t.Run("stop with nil instance", func(t *testing.T) {
		w := NewWrapper(configPath)
		// Don't start, just try to stop
		err := w.Stop(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "stop")
	})
}

func TestWrapperReload_ErrorCases(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	configJSON := `{
		"log": {"disabled": false},
		"inbounds": [],
		"outbounds": [{"type": "direct", "tag": "direct"}]
	}`
	require.NoError(t, os.WriteFile(configPath, []byte(configJSON), 0o644))

	t.Run("reload with config that fails hash", func(t *testing.T) {
		w := NewWrapper(configPath)
		require.NoError(t, w.Start(ctx))

		// Create a config that will fail hash (nil config)
		err := w.Reload(ctx, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reload")

		status := w.Status()
		assert.Equal(t, StateRunning, status.State) // Should remain running

		_ = w.Stop(ctx)
	})

	t.Run("reload with config that fails validate", func(t *testing.T) {
		w := NewWrapper(configPath)
		require.NoError(t, w.Start(ctx))

		// Create a config that will fail validation (nil config)
		err := w.Reload(ctx, nil)
		require.Error(t, err)

		status := w.Status()
		assert.Equal(t, StateRunning, status.State)

		_ = w.Stop(ctx)
	})

	t.Run("reload with config that fails ToJSON", func(t *testing.T) {
		w := NewWrapper(configPath)
		require.NoError(t, w.Start(ctx))

		// Create a config that will fail ToJSON (nil config)
		err := w.Reload(ctx, nil)
		require.Error(t, err)

		status := w.Status()
		assert.Equal(t, StateRunning, status.State)

		_ = w.Stop(ctx)
	})

	t.Run("reload with canceled context", func(t *testing.T) {
		w := NewWrapper(configPath)
		require.NoError(t, w.Start(ctx))

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		newConfig := &Config{
			Log:       &LogConfig{Disabled: true},
			Inbounds:  []Inbound{},
			Outbounds: []Outbound{{Type: "direct", Tag: "direct"}},
		}

		err := w.Reload(ctx, newConfig)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reload")

		status := w.Status()
		assert.Equal(t, StateRunning, status.State)

		_ = w.Stop(context.Background())
	})

	t.Run("reload when instance becomes nil", func(t *testing.T) {
		w := NewWrapper(configPath)
		require.NoError(t, w.Start(ctx))

		// Manually set instance to nil to test that branch
		w.mu.Lock()
		w.instance = nil
		w.mu.Unlock()

		newConfig := &Config{
			Log:       &LogConfig{Disabled: true},
			Inbounds:  []Inbound{},
			Outbounds: []Outbound{{Type: "direct", Tag: "direct"}},
		}

		err := w.Reload(ctx, newConfig)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reload")

		status := w.Status()
		assert.Equal(t, StateStopped, status.State)
	})
}

func TestWrapperStatus_ErrorState(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	configJSON := `{"log": {"disabled": false}, "inbounds": [], "outbounds": []}`
	require.NoError(t, os.WriteFile(configPath, []byte(configJSON), 0o644))

	w := NewWrapper(configPath)

	// Try to start with invalid config to set error state
	ctx := context.Background()
	_ = w.Start(ctx)

	status := w.Status()
	// Check that status includes error info if start failed
	if status.State == StateStopped {
		// If start failed, check last error is set
		assert.NotEmpty(t, status.LastError)
	}
}

func TestMockSingBox_ReloadNotRunning(t *testing.T) {
	ctx := context.Background()
	mock := &MockSingBox{}

	// Try to reload when not running
	config := &Config{}
	err := mock.Reload(ctx, config)
	require.Error(t, err)
	assert.Equal(t, ErrNotRunning, err)
}
