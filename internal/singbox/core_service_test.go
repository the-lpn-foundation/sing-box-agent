package singbox

import (
	"context"
	"sync"
	"testing"
	"time"
)

// fakeReloader is a test double that records Reload invocations instead of
// shelling out to systemctl.
type fakeReloader struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (f *fakeReloader) Reload(_ context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.err
}

func (f *fakeReloader) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// TestCoreServiceAdapter_Reload_UsesInjectedReloader ensures POST /core/reload
// goes through the injected reloader (respecting reload_strategy) instead of
// the hardcoded systemctl path.
func TestCoreServiceAdapter_Reload_UsesInjectedReloader(t *testing.T) {
	reloader := &fakeReloader{}
	adapter := NewCoreServiceAdapter("/tmp/test-config.json").WithReloader(reloader)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := adapter.Reload(ctx); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}

	if got := reloader.callCount(); got != 1 {
		t.Errorf("injected reloader called %d times, want 1", got)
	}
}

// TestCoreServiceAdapter_Reload_InjectedReloaderError ensures the reloader's
// error is propagated to the caller.
func TestCoreServiceAdapter_Reload_InjectedReloaderError(t *testing.T) {
	reloader := &fakeReloader{err: context.DeadlineExceeded}
	adapter := NewCoreServiceAdapter("/tmp/test-config.json").WithReloader(reloader)

	err := adapter.Reload(context.Background())
	if err == nil {
		t.Fatal("Reload() expected error from injected reloader, got nil")
	}

	if got := reloader.callCount(); got != 1 {
		t.Errorf("injected reloader called %d times, want 1", got)
	}
}

// TestCoreServiceAdapter_WithReloaderChaining ensures WithReloader supports
// chaining like WithPIDFile.
func TestCoreServiceAdapter_WithReloaderChaining(t *testing.T) {
	adapter := NewCoreServiceAdapter("/tmp/test-config.json").
		WithReloader(&fakeReloader{}).
		WithPIDFile("/tmp/sing-box.pid")

	if adapter.pidFile != "/tmp/sing-box.pid" {
		t.Errorf("pidFile = %q, want /tmp/sing-box.pid", adapter.pidFile)
	}
	if adapter.reloader == nil {
		t.Error("reloader should be set after WithReloader")
	}
}
