package singbox

// Regression tests for cross-writer config races: ConfigClient (REST CRUD)
// and sync.ConfigManager (desired-state) both mutate the same config file
// with independent internal mutexes, so they must serialise through
// cfglock.For(path) to avoid lost updates.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/the-lpn-foundation/sing-box-agent/internal/models"
	pkgsync "github.com/the-lpn-foundation/sing-box-agent/internal/sync"
)

// Parallel CRUD + desired-state writes to one file must not lose updates:
// after 10 CreateUser and 10 AddUser calls with distinct subIDs, all 20
// users must be present in the file.
func TestConfigClient_And_ConfigManager_NoLostUpdate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(path, []byte(`{
		"inbounds": [
			{"type": "vless", "tag": "in-1", "users": []}
		]
	}`), 0o644))

	client := NewConfigClient(path)
	client.WithReloader(&mockReloader{})
	manager := pkgsync.NewConfigManager(path, nil)

	const perWriter = 10
	var wg sync.WaitGroup
	errCh := make(chan error, 2*perWriter)

	for i := 0; i < perWriter; i++ {
		wg.Add(2)
		go func(idx int) {
			defer wg.Done()
			errCh <- client.CreateUser(context.Background(), modelsUserForIdx(idx, "crud"))
		}(i)
		go func(idx int) {
			defer wg.Done()
			errCh <- manager.AddUser("in-1", modelsUserForIdx(idx, "mgr"))
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}

	names := fileUserNames(t, path)
	for i := 0; i < perWriter; i++ {
		assert.True(t, names[fmt.Sprintf("crud-%d", i)], "user crud-%d lost", i)
		assert.True(t, names[fmt.Sprintf("mgr-%d", i)], "user mgr-%d lost", i)
	}
	assert.Len(t, names, 2*perWriter)
}

// performDebouncedReload must not roll back the config when a concurrent
// writer replaced the file after the debounce window's last write.
// Scenario: window 1 writes via CRUD, an external writer overwrites the
// file before the window closes, then window 2 writes on top of the
// external version. Reloads always fail; the file must end up with the
// external version (window 1 keeps it, window 2 legitimately restores it).
func TestConfigClient_DebounceReload_SkipsRollbackWhenFileChangedExternally(t *testing.T) {
	path := writeDebounceTestConfig(t, `{
		"inbounds": [
			{"type": "vless", "tag": "in-1", "users": []}
		]
	}`)

	reloader := &mockReloader{err: errors.New("sing-box reload failed")}
	client := NewConfigClient(path)
	client.SetDebounceInterval(80 * time.Millisecond)
	client.WithReloader(reloader)

	// Window 1: CRUD write schedules the debounced reload.
	require.NoError(t, client.CreateUser(context.Background(), models.User{
		InboundTag: "in-1",
		SubID:      "crud-user-1",
		UUID:       "11111111-1111-1111-1111-111111111111",
	}))

	// A concurrent writer replaces the config before the window closes.
	externalConfig := `{"inbounds": [{"type": "vless", "tag": "in-1", "users": [{"name": "external-user", "uuid": "22222222-2222-2222-2222-222222222222"}]}]}`
	require.NoError(t, os.WriteFile(path, []byte(externalConfig), 0o600))

	// Window 1 closes: reload fails; rollback must be skipped because the
	// on-disk bytes are no longer what window 1 wrote (2 reload calls).
	waitForReload(t, reloader, 2, 3*time.Second)
	time.Sleep(100 * time.Millisecond)

	// Window 2: another CRUD write on top of the externally written config.
	require.NoError(t, client.CreateUser(context.Background(), models.User{
		InboundTag: "in-1",
		SubID:      "crud-user-2",
		UUID:       "33333333-3333-3333-3333-333333333333",
	}))

	// Window 2 closes: reload fails; here the file still matches window 2's
	// bytes, so the rollback is legitimate and restores the external version
	// (2 more reload calls).
	waitForReload(t, reloader, 4, 3*time.Second)
	time.Sleep(100 * time.Millisecond)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, externalConfig, string(data),
		"debounced rollback must not clobber a config written after the window's last write")
}

// fileUserNames reads the config file and returns the set of user names
// found across all inbounds.
// fileUserNames reads the config file and returns the set of user
// identifiers (name or subId, depending on which writer produced the entry)
// found across all inbounds.
func fileUserNames(t *testing.T, path string) map[string]bool {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var root map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &root))

	names := make(map[string]bool)
	for _, inbound := range getInbounds(root) {
		for _, rawUser := range getInboundUsers(inbound) {
			userMap, ok := rawUser.(map[string]interface{})
			if !ok {
				continue
			}
			if name := getString(userMap, "name"); name != "" {
				names[name] = true
			}
			if subID := getString(userMap, "subId"); subID != "" {
				names[subID] = true
			}
		}
	}
	return names
}

// modelsUserForIdx builds a distinct user for the concurrency tests so the
// two writers never collide on the same subID.
func modelsUserForIdx(idx int, prefix string) models.User {
	return models.User{
		InboundTag: "in-1",
		SubID:      fmt.Sprintf("%s-%d", prefix, idx),
		UUID:       fmt.Sprintf("abababab-abab-abab-abab-%012d", idx),
	}
}
