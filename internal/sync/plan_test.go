package sync

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oglenyaboss/sing-box-agent/internal/models"
)

// writePlanTestConfig creates a temporary sing-box config file.
func writePlanTestConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

// readPlanTestConfig loads the config file for assertions.
func readPlanTestConfig(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var cfg map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &cfg))
	return cfg
}

// configInbounds returns the inbounds slice from a parsed config.
func configInbounds(t *testing.T, cfg map[string]interface{}) []interface{} {
	t.Helper()
	raw, ok := cfg["inbounds"].([]interface{})
	require.True(t, ok, "inbounds must be a list")
	return raw
}

// TestPlan_ExecuteDeleteOrdering verifies that user deletes run before inbound
// deletes: deleting an inbound removes its users from the config, so any
// subsequent DeleteUser for that inbound would fail with "inbound not found".
func TestPlan_ExecuteDeleteOrdering(t *testing.T) {
	configPath := writePlanTestConfig(t, `{
		"inbounds": [{
			"type": "vless",
			"tag": "vless-in",
			"port": 443,
			"users": [
				{"name": "user1", "uuid": "uuid-1"},
				{"name": "user2", "uuid": "uuid-2"}
			]
		}]
	}`)

	client := &MockSingBoxClient{
		inbounds: []models.Inbound{
			{Tag: "vless-in", Type: "vless", Port: 443},
		},
		users: []models.User{
			{SubID: "user1", UUID: "uuid-1", InboundTag: "vless-in"},
			{SubID: "user2", UUID: "uuid-2", InboundTag: "vless-in"},
		},
	}

	configManager := NewConfigManager(configPath, &mockReloader{})
	engine := NewEngineWithConfigManager(client, configManager)

	// Empty desired state: the inbound and both users must be removed.
	desired := &models.DesiredState{Version: 1}

	plan, err := engine.Reconcile(context.Background(), desired)
	require.NoError(t, err)
	require.NotNil(t, plan)
	require.Len(t, plan.InboundChanges, 1)
	require.Len(t, plan.UserChanges, 2)

	// Guard the dependency order in the built plan itself.
	userDeleteIdx := -1
	inboundDeleteIdx := -1
	for i, step := range plan.steps {
		if s, ok := step.(*UserStep); ok && s.change.Type == ChangeDelete {
			userDeleteIdx = i
		}
		if s, ok := step.(*InboundStep); ok && s.change.Type == ChangeDelete {
			inboundDeleteIdx = i
		}
	}
	require.NotEqual(t, -1, userDeleteIdx)
	require.NotEqual(t, -1, inboundDeleteIdx)
	require.Less(t, userDeleteIdx, inboundDeleteIdx, "user deletes must run before inbound deletes")

	err = engine.Apply(context.Background(), plan, desired.Version)
	require.NoError(t, err)

	cfg := readPlanTestConfig(t, configPath)
	assert.Empty(t, configInbounds(t, cfg), "inbound must be deleted")
}

// TestPlan_ExecuteMixedCreateAndDelete verifies a plan where one inbound is
// created with a user in it while another inbound with a user is deleted —
// everything must apply without errors thanks to dependency-ordered steps.
func TestPlan_ExecuteMixedCreateAndDelete(t *testing.T) {
	configPath := writePlanTestConfig(t, `{
		"inbounds": [{
			"type": "vless",
			"tag": "old-in",
			"port": 8443,
			"users": [
				{"name": "old-user", "uuid": "uuid-old"}
			]
		}]
	}`)

	client := &MockSingBoxClient{
		inbounds: []models.Inbound{
			{Tag: "old-in", Type: "vless", Port: 8443},
		},
		users: []models.User{
			{SubID: "old-user", UUID: "uuid-old", InboundTag: "old-in"},
		},
	}

	configManager := NewConfigManager(configPath, &mockReloader{})
	engine := NewEngineWithConfigManager(client, configManager)

	desired := &models.DesiredState{
		Version: 2,
		Inbounds: []models.Inbound{
			{Tag: "new-in", Type: "vless", Port: 443},
		},
		Users: []models.User{
			{SubID: "new-user", UUID: "uuid-new", InboundTag: "new-in"},
		},
	}

	plan, err := engine.Reconcile(context.Background(), desired)
	require.NoError(t, err)
	require.Len(t, plan.InboundChanges, 2)
	require.Len(t, plan.UserChanges, 2)

	err = engine.Apply(context.Background(), plan, desired.Version)
	require.NoError(t, err)

	cfg := readPlanTestConfig(t, configPath)
	inbounds := configInbounds(t, cfg)
	require.Len(t, inbounds, 1)

	inbound, ok := inbounds[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "new-in", inbound["tag"])

	users, ok := inbound["users"].([]interface{})
	require.True(t, ok, "created inbound must keep its user")
	require.Len(t, users, 1)
	user, ok := users[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "new-user", user["subId"])
}
