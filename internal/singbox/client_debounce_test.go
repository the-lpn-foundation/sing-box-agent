package singbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oglenyaboss/sing-box-agent/internal/models"
)

// mockReloader records Reload invocations and returns a configurable error,
// standing in for sync.SystemctlReloader / sync.SignalReloader.
type mockReloader struct {
	mu      sync.Mutex
	calls   int
	err     error
	lastCtx context.Context
}

func (m *mockReloader) Reload(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	m.lastCtx = ctx
	return m.err
}

func (m *mockReloader) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

// waitForReload polls until the mock reloader has been called want times or
// the timeout expires.
func waitForReload(t *testing.T, m *mockReloader, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if m.callCount() >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("reloader called %d times, want %d", m.callCount(), want)
}

func writeDebounceTestConfig(t *testing.T, config string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(path, []byte(config), 0o644))
	return path
}

// Debounce coalescing: multiple rapid saveAndReload calls within the window
// must result in a single reload.
func TestConfigClient_DebounceReload_CoalescesMultipleSaves(t *testing.T) {
	path := writeDebounceTestConfig(t, `{
		"inbounds": [
			{"type": "vless", "tag": "in-1", "users": []}
		]
	}`)

	reloader := &mockReloader{}
	client := NewConfigClient(path, &Wrapper{})
	client.SetDebounceInterval(200 * time.Millisecond)
	client.WithReloader(reloader)

	for i := 0; i < 5; i++ {
		user := models.User{
			InboundTag: "in-1",
			SubID:      fmt.Sprintf("user-%d", i),
			UUID:       fmt.Sprintf("00000000-0000-0000-0000-%012d", i),
		}
		require.NoError(t, client.CreateUser(context.Background(), user))
	}

	// Window closes ~100ms after the last write; reload fires exactly once.
	waitForReload(t, reloader, 1, 3*time.Second)
	assert.Equal(t, 1, reloader.callCount())

	// Every write persisted to disk even though reload happened once.
	users, err := client.GetUsers(context.Background())
	require.NoError(t, err)
	assert.Len(t, users, 5)
}

// WithReloader injection: doReload must call the injected reloader, not the
// hardcoded systemctl path.
func TestConfigClient_DebounceReload_WithReloaderInjection(t *testing.T) {
	path := writeDebounceTestConfig(t, `{"inbounds": []}`)
	client := NewConfigClient(path, nil)
	ok := &mockReloader{}
	client.WithReloader(ok)

	err := client.doReload(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, ok.callCount())

	// A reloader error must propagate unchanged — reloadSystemService could
	// never produce this sentinel error, proving the injected path was used.
	sentinel := errors.New("sing-box reload failed")
	failing := &mockReloader{err: sentinel}
	client.WithReloader(failing)

	err = client.doReload(context.Background())
	require.ErrorIs(t, err, sentinel)
	assert.Equal(t, 1, failing.callCount())
}

// Debounce rollback on failure: when the deferred reload fails, the config is
// restored to the pre-window snapshot and the reload is retried.
func TestConfigClient_DebounceReload_RollbackOnFailure(t *testing.T) {
	const originalConfig = `{
		"inbounds": [
			{"type": "vless", "tag": "in-1", "users": []}
		]
	}`
	path := writeDebounceTestConfig(t, originalConfig)

	reloader := &mockReloader{err: errors.New("sing-box reload failed")}
	client := NewConfigClient(path, &Wrapper{})
	client.SetDebounceInterval(100 * time.Millisecond)
	client.WithReloader(reloader)

	user := models.User{
		InboundTag: "in-1",
		SubID:      "user1",
		UUID:       "12345678-1234-1234-1234-123456789012",
	}
	require.NoError(t, client.CreateUser(context.Background(), user))

	// First attempt fails → rollback to pre-window config → retry.
	waitForReload(t, reloader, 2, 3*time.Second)
	assert.Equal(t, 2, reloader.callCount())

	// Config file must be byte-identical to the pre-window snapshot.
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, originalConfig, string(data))
}

// Reload strategy respected: a SignalReloader-like mock injected via
// WithReloader must be used end-to-end instead of systemctl.
func TestConfigClient_DebounceReload_ReloadStrategyRespected(t *testing.T) {
	path := writeDebounceTestConfig(t, `{
		"inbounds": [
			{"type": "vless", "tag": "in-1", "users": []}
		]
	}`)

	reloader := &mockReloader{}
	client := NewConfigClient(path, &Wrapper{})
	client.SetDebounceInterval(100 * time.Millisecond)
	client.WithReloader(reloader)

	user := models.User{
		InboundTag: "in-1",
		SubID:      "user1",
		UUID:       "12345678-1234-1234-1234-123456789012",
	}
	require.NoError(t, client.CreateUser(context.Background(), user))

	waitForReload(t, reloader, 1, 3*time.Second)
	assert.Equal(t, 1, reloader.callCount())

	// The mock (like SignalReloader) received a live context — a deadline was
	// attached by performDebouncedReload. Had doReload fallen through to
	// systemctl, the injected reloader would never have been invoked.
	require.NotNil(t, reloader.lastCtx)
	_, ok := reloader.lastCtx.Deadline()
	assert.True(t, ok, "reloader should receive a context with a deadline")
}
