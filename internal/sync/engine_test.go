package sync

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oglenyaboss/sing-box-agent/internal/models"
)

type MockSingBoxClient struct {
	inbounds []models.Inbound
	users    []models.User
	failOn   string
}

func (m *MockSingBoxClient) GetInbounds(ctx context.Context) ([]models.Inbound, error) {
	if m.failOn == "GetInbounds" {
		return nil, errors.New("mock error")
	}
	return m.inbounds, nil
}

func (m *MockSingBoxClient) GetUsers(ctx context.Context) ([]models.User, error) {
	if m.failOn == "GetUsers" {
		return nil, errors.New("mock error")
	}
	return m.users, nil
}

func (m *MockSingBoxClient) CreateInbound(ctx context.Context, inbound models.Inbound) error {
	return nil
}

func (m *MockSingBoxClient) UpdateInbound(ctx context.Context, inbound models.Inbound) error {
	return nil
}

func (m *MockSingBoxClient) DeleteInbound(ctx context.Context, tag string) error {
	return nil
}

func (m *MockSingBoxClient) CreateUser(ctx context.Context, user models.User) error {
	return nil
}

func (m *MockSingBoxClient) UpdateUser(ctx context.Context, user models.User) error {
	return nil
}

func (m *MockSingBoxClient) DeleteUser(ctx context.Context, subID string) error {
	return nil
}

func TestDiffInbounds_Create(t *testing.T) {
	desired := []models.Inbound{
		{Tag: "vless-1", Type: "vless", Port: 443},
	}
	current := []models.Inbound{}

	changes := DiffInbounds(desired, current)

	require.Len(t, changes, 1)
	assert.Equal(t, ChangeCreate, changes[0].Type)
	assert.Equal(t, "vless-1", changes[0].Inbound.Tag)
}

func TestDiffInbounds_Update(t *testing.T) {
	desired := []models.Inbound{
		{Tag: "vless-1", Type: "vless", Port: 444},
	}
	current := []models.Inbound{
		{Tag: "vless-1", Type: "vless", Port: 443},
	}

	changes := DiffInbounds(desired, current)

	require.Len(t, changes, 1)
	assert.Equal(t, ChangeUpdate, changes[0].Type)
	assert.Equal(t, "vless-1", changes[0].Inbound.Tag)
	assert.Equal(t, 444, changes[0].Inbound.Port)
	assert.Equal(t, 443, changes[0].Previous.Port)
}

func TestDiffInbounds_Delete(t *testing.T) {
	desired := []models.Inbound{}
	current := []models.Inbound{
		{Tag: "vless-1", Type: "vless", Port: 443},
	}

	changes := DiffInbounds(desired, current)

	require.Len(t, changes, 1)
	assert.Equal(t, ChangeDelete, changes[0].Type)
	assert.Equal(t, "vless-1", changes[0].Inbound.Tag)
}

func TestDiffInbounds_NoChange(t *testing.T) {
	desired := []models.Inbound{
		{Tag: "vless-1", Type: "vless", Port: 443},
	}
	current := []models.Inbound{
		{Tag: "vless-1", Type: "vless", Port: 443},
	}

	changes := DiffInbounds(desired, current)

	assert.Len(t, changes, 0)
}

func TestDiffInbounds_Options(t *testing.T) {
	desired := []models.Inbound{
		{
			Tag:     "vless-1",
			Type:    "vless",
			Options: map[string]interface{}{"key": "value1"},
		},
	}
	current := []models.Inbound{
		{
			Tag:     "vless-1",
			Type:    "vless",
			Options: map[string]interface{}{"key": "value2"},
		},
	}

	changes := DiffInbounds(desired, current)

	require.Len(t, changes, 1)
	assert.Equal(t, ChangeUpdate, changes[0].Type)
}

func TestDiffUsers_Create(t *testing.T) {
	desired := []models.User{
		{SubID: "user1", UUID: "uuid-1", InboundTag: "vless-1"},
	}
	current := []models.User{}

	changes := DiffUsers(desired, current)

	require.Len(t, changes, 1)
	assert.Equal(t, ChangeCreate, changes[0].Type)
	assert.Equal(t, "user1", changes[0].User.SubID)
}

func TestDiffUsers_Update(t *testing.T) {
	desired := []models.User{
		{SubID: "user1", UUID: "uuid-1", InboundTag: "vless-1", Enabled: true},
	}
	current := []models.User{
		{SubID: "user1", UUID: "uuid-1", InboundTag: "vless-1", Enabled: false},
	}

	changes := DiffUsers(desired, current)

	require.Len(t, changes, 1)
	assert.Equal(t, ChangeUpdate, changes[0].Type)
	assert.Equal(t, "user1", changes[0].User.SubID)
	assert.True(t, changes[0].User.Enabled)
	assert.False(t, changes[0].Previous.Enabled)
}

func TestDiffUsers_Delete(t *testing.T) {
	desired := []models.User{}
	current := []models.User{
		{SubID: "user1", UUID: "uuid-1", InboundTag: "vless-1"},
	}

	changes := DiffUsers(desired, current)

	require.Len(t, changes, 1)
	assert.Equal(t, ChangeDelete, changes[0].Type)
	assert.Equal(t, "user1", changes[0].User.SubID)
}

func TestDiffUsers_NoChange(t *testing.T) {
	desired := []models.User{
		{SubID: "user1", UUID: "uuid-1", InboundTag: "vless-1"},
	}
	current := []models.User{
		{SubID: "user1", UUID: "uuid-1", InboundTag: "vless-1"},
	}

	changes := DiffUsers(desired, current)

	assert.Len(t, changes, 0)
}

func TestDiffUsers_ResetAt(t *testing.T) {
	now := time.Now()
	desired := []models.User{
		{SubID: "user1", UUID: "uuid-1", InboundTag: "vless-1", ResetAt: &now},
	}
	current := []models.User{
		{SubID: "user1", UUID: "uuid-1", InboundTag: "vless-1", ResetAt: nil},
	}

	changes := DiffUsers(desired, current)

	require.Len(t, changes, 1)
	assert.Equal(t, ChangeUpdate, changes[0].Type)
}

func TestNewPlan(t *testing.T) {
	inboundChanges := []InboundChange{{Type: ChangeCreate}}
	userChanges := []UserChange{{Type: ChangeCreate}}

	plan := NewPlan(inboundChanges, userChanges)

	assert.Equal(t, inboundChanges, plan.InboundChanges)
	assert.Equal(t, userChanges, plan.UserChanges)
	assert.Equal(t, 2, plan.StepsCount())
}

func TestPlan_Execute_Success(t *testing.T) {
	plan := NewPlan([]InboundChange{}, []UserChange{})

	err := plan.Execute(context.Background())
	assert.NoError(t, err)
}

func TestPlan_Execute_Rollback(t *testing.T) {
	step := &MockStep{failExecute: true}
	plan := NewPlan([]InboundChange{}, []UserChange{})
	plan.steps = []Step{step}

	err := plan.Execute(context.Background())
	assert.Error(t, err)
	assert.True(t, step.rollbackCalled)
}

func TestPlan_Rollback(t *testing.T) {
	step := &MockStep{}
	plan := NewPlan([]InboundChange{}, []UserChange{})
	plan.steps = []Step{step}

	err := plan.Rollback(context.Background())
	assert.NoError(t, err)
	assert.True(t, step.rollbackCalled)
}

type MockStep struct {
	failExecute    bool
	rollbackCalled bool
}

func (m *MockStep) Execute(ctx context.Context) error {
	if m.failExecute {
		return errors.New("mock error")
	}
	return nil
}

func (m *MockStep) Rollback(ctx context.Context) error {
	m.rollbackCalled = true
	return nil
}

func (m *MockStep) Description() string {
	return "mock step"
}

func TestEngine_Reconcile(t *testing.T) {
	client := &MockSingBoxClient{
		inbounds: []models.Inbound{
			{Tag: "vless-1", Type: "vless", Port: 443},
		},
		users: []models.User{
			{SubID: "user1", UUID: "uuid-1", InboundTag: "vless-1"},
		},
	}
	engine := NewEngine(client)

	desired := &models.DesiredState{
		Version: 1,
		Inbounds: []models.Inbound{
			{Tag: "vless-2", Type: "vless", Port: 443},
		},
		Users: []models.User{
			{SubID: "user2", UUID: "uuid-2", InboundTag: "vless-2"},
		},
		Timestamp: time.Now(),
	}

	plan, err := engine.Reconcile(context.Background(), desired)

	require.NoError(t, err)
	require.NotNil(t, plan)
	assert.Len(t, plan.InboundChanges, 2)
	assert.Len(t, plan.UserChanges, 2)
}

func TestEngine_Reconcile_Error(t *testing.T) {
	client := &MockSingBoxClient{failOn: "GetInbounds"}
	engine := NewEngine(client)

	desired := &models.DesiredState{
		Version:   1,
		Timestamp: time.Now(),
	}

	plan, err := engine.Reconcile(context.Background(), desired)

	require.Error(t, err)
	assert.Nil(t, plan)
	assert.Contains(t, err.Error(), "failed to get current inbounds")
}

func TestEngine_Apply(t *testing.T) {
	client := &MockSingBoxClient{}
	engine := NewEngine(client)

	plan := NewPlan([]InboundChange{}, []UserChange{})

	err := engine.Apply(context.Background(), plan, 1)

	require.NoError(t, err)
	status := engine.Status()
	statusData := status.Get()
	assert.Equal(t, 1, statusData["current_version"])
	state, ok := statusData["state"].(SyncState)
	require.True(t, ok, "state should be SyncState")
	assert.Equal(t, "synced", string(state))
}

func TestEngine_Apply_Error(t *testing.T) {
	client := &MockSingBoxClient{}
	engine := NewEngine(client)

	step := &MockStep{failExecute: true}
	plan := NewPlan([]InboundChange{}, []UserChange{})
	plan.steps = []Step{step}

	err := engine.Apply(context.Background(), plan, 1)

	require.Error(t, err)
	status := engine.Status()
	statusData := status.Get()
	state, ok := statusData["state"].(SyncState)
	require.True(t, ok, "state should be SyncState")
	assert.Equal(t, "error", string(state))
	assert.NotEmpty(t, statusData["error"])
}

func TestEngine_DryRun(t *testing.T) {
	client := &MockSingBoxClient{
		inbounds: []models.Inbound{
			{Tag: "vless-1", Type: "vless", Port: 443},
		},
		users: []models.User{
			{SubID: "user1", UUID: "uuid-1", InboundTag: "vless-1"},
		},
	}
	engine := NewEngine(client)

	desired := &models.DesiredState{
		Version:   1,
		Timestamp: time.Now(),
	}

	plan, err := engine.DryRun(context.Background(), desired)

	require.NoError(t, err)
	require.NotNil(t, plan)
	assert.Len(t, plan.InboundChanges, 1)
	assert.Len(t, plan.UserChanges, 1)
}

func TestSyncStatus_Get(t *testing.T) {
	status := NewSyncStatus()
	status.UpdateSync(1, 5)

	data := status.Get()

	state, ok := data["state"].(SyncState)
	require.True(t, ok, "state should be SyncState")
	assert.Equal(t, "synced", string(state))
	assert.Equal(t, 1, data["current_version"])
	assert.Equal(t, 5, data["changes_applied"])
	inSync, ok := data["in_sync"].(bool)
	require.True(t, ok, "in_sync should be bool")
	assert.True(t, inSync)
	assert.Equal(t, "", data["error"])
}

func TestSyncStatus_UpdateError(t *testing.T) {
	status := NewSyncStatus()
	status.UpdateError("test error")

	data := status.Get()

	state, ok := data["state"].(SyncState)
	require.True(t, ok, "state should be SyncState")
	assert.Equal(t, "error", string(state))
	assert.Equal(t, "test error", data["error"])
	inSync, ok := data["in_sync"].(bool)
	require.True(t, ok, "in_sync should be bool")
	assert.False(t, inSync)
}

func TestSyncStatus_UpdateSyncing(t *testing.T) {
	status := NewSyncStatus()
	status.UpdateSyncing()

	data := status.Get()

	state, ok := data["state"].(SyncState)
	require.True(t, ok, "state should be SyncState")
	assert.Equal(t, "syncing", string(state))
}

// TestNewEngineWithConfigManager tests creating an engine with a config manager
func TestNewEngineWithConfigManager(t *testing.T) {
	client := &MockSingBoxClient{}
	configManager := NewConfigManager("/tmp/config.json", nil)

	engine := NewEngineWithConfigManager(client, configManager)

	require.NotNil(t, engine)
	require.Equal(t, client, engine.singbox)
	require.Equal(t, configManager, engine.configManager)
	require.NotNil(t, engine.status)
}

// TestIncrementChangesApplied tests the global metrics increment function
func TestIncrementChangesApplied(t *testing.T) {
	// This function is a simple counter increment
	// We just verify it doesn't panic
	IncrementChangesApplied()
	IncrementChangesApplied()
	IncrementChangesApplied()
}

// TestInboundStep_Execute tests InboundStep Execute method
func TestInboundStep_Execute(t *testing.T) {
	tests := []struct {
		name        string
		change      InboundChange
		configMgr   *ConfigManager
		wantErr     bool
		errContains string
	}{
		{
			name: "error - config manager not configured for create",
			change: InboundChange{
				Type:    ChangeCreate,
				Inbound: models.Inbound{Tag: "test-in"},
			},
			configMgr:   nil,
			wantErr:     true,
			errContains: "config manager is not configured",
		},
		{
			name: "error - config manager not configured for update",
			change: InboundChange{
				Type:    ChangeUpdate,
				Inbound: models.Inbound{Tag: "test-in"},
			},
			configMgr:   nil,
			wantErr:     true,
			errContains: "config manager is not configured",
		},
		{
			name: "error - config manager not configured for delete",
			change: InboundChange{
				Type:    ChangeDelete,
				Inbound: models.Inbound{Tag: "test-in"},
			},
			configMgr:   nil,
			wantErr:     true,
			errContains: "config manager is not configured",
		},
		{
			name: "error - unknown change type",
			change: InboundChange{
				Type:    "unknown",
				Inbound: models.Inbound{Tag: "test-in"},
			},
			configMgr:   nil,
			wantErr:     true,
			errContains: "unknown change type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := &InboundStep{
				change:        tt.change,
				configManager: tt.configMgr,
			}

			err := step.Execute(context.Background())

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					require.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestInboundStep_Rollback tests InboundStep Rollback method
func TestInboundStep_Rollback(t *testing.T) {
	tests := []struct {
		name        string
		change      InboundChange
		configMgr   *ConfigManager
		wantErr     bool
		errContains string
	}{
		{
			name: "error - config manager not configured for create rollback",
			change: InboundChange{
				Type:    ChangeCreate,
				Inbound: models.Inbound{Tag: "test-in"},
			},
			configMgr:   nil,
			wantErr:     true,
			errContains: "config manager is not configured",
		},
		{
			name: "error - config manager not configured for update rollback",
			change: InboundChange{
				Type:     ChangeUpdate,
				Inbound:  models.Inbound{Tag: "test-in"},
				Previous: &models.Inbound{Tag: "test-in"},
			},
			configMgr:   nil,
			wantErr:     true,
			errContains: "config manager is not configured",
		},
		{
			name: "error - missing previous inbound for update rollback",
			change: InboundChange{
				Type:     ChangeUpdate,
				Inbound:  models.Inbound{Tag: "test-in"},
				Previous: nil,
			},
			configMgr:   &ConfigManager{},
			wantErr:     true,
			errContains: "missing previous inbound for rollback",
		},
		{
			name: "error - config manager not configured for delete rollback",
			change: InboundChange{
				Type:     ChangeDelete,
				Inbound:  models.Inbound{Tag: "test-in"},
				Previous: &models.Inbound{Tag: "test-in"},
			},
			configMgr:   nil,
			wantErr:     true,
			errContains: "config manager is not configured",
		},
		{
			name: "error - missing previous inbound for delete rollback",
			change: InboundChange{
				Type:     ChangeDelete,
				Inbound:  models.Inbound{Tag: "test-in"},
				Previous: nil,
			},
			configMgr:   &ConfigManager{},
			wantErr:     true,
			errContains: "missing previous inbound for rollback",
		},
		{
			name: "error - unknown change type for rollback",
			change: InboundChange{
				Type:    "unknown",
				Inbound: models.Inbound{Tag: "test-in"},
			},
			configMgr:   nil,
			wantErr:     true,
			errContains: "unknown change type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := &InboundStep{
				change:        tt.change,
				configManager: tt.configMgr,
			}

			err := step.Rollback(context.Background())

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					require.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestInboundStep_Description tests InboundStep Description method
func TestInboundStep_Description(t *testing.T) {
	tests := []struct {
		name   string
		change InboundChange
		want   string
	}{
		{
			name: "create inbound",
			change: InboundChange{
				Type:    ChangeCreate,
				Inbound: models.Inbound{Tag: "test-in"},
			},
			want: "inbound create test-in",
		},
		{
			name: "update inbound",
			change: InboundChange{
				Type:    ChangeUpdate,
				Inbound: models.Inbound{Tag: "my-inbound"},
			},
			want: "inbound update my-inbound",
		},
		{
			name: "delete inbound",
			change: InboundChange{
				Type:    ChangeDelete,
				Inbound: models.Inbound{Tag: "old-inbound"},
			},
			want: "inbound delete old-inbound",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := &InboundStep{change: tt.change}
			got := step.Description()
			require.Equal(t, tt.want, got)
		})
	}
}

// TestUserStep_Execute tests UserStep Execute method
func TestUserStep_Execute(t *testing.T) {
	tests := []struct {
		name        string
		change      UserChange
		configMgr   *ConfigManager
		wantErr     bool
		errContains string
	}{
		{
			name: "error - config manager not configured for create",
			change: UserChange{
				Type: ChangeCreate,
				User: models.User{SubID: "user1", InboundTag: "test-in"},
			},
			configMgr:   nil,
			wantErr:     true,
			errContains: "config manager is not configured",
		},
		{
			name: "error - config manager not configured for update",
			change: UserChange{
				Type: ChangeUpdate,
				User: models.User{SubID: "user1", InboundTag: "test-in"},
			},
			configMgr:   nil,
			wantErr:     true,
			errContains: "config manager is not configured",
		},
		{
			name: "error - config manager not configured for delete",
			change: UserChange{
				Type: ChangeDelete,
				User: models.User{SubID: "user1", InboundTag: "test-in"},
			},
			configMgr:   nil,
			wantErr:     true,
			errContains: "config manager is not configured",
		},
		{
			name: "error - unknown change type",
			change: UserChange{
				Type: "unknown",
				User: models.User{SubID: "user1"},
			},
			configMgr:   nil,
			wantErr:     true,
			errContains: "unknown change type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := &UserStep{
				change:        tt.change,
				configManager: tt.configMgr,
			}

			err := step.Execute(context.Background())

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					require.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestUserStep_Rollback tests UserStep Rollback method
func TestUserStep_Rollback(t *testing.T) {
	tests := []struct {
		name        string
		change      UserChange
		configMgr   *ConfigManager
		wantErr     bool
		errContains string
	}{
		{
			name: "error - config manager not configured for create rollback",
			change: UserChange{
				Type: ChangeCreate,
				User: models.User{SubID: "user1", InboundTag: "test-in"},
			},
			configMgr:   nil,
			wantErr:     true,
			errContains: "config manager is not configured",
		},
		{
			name: "error - config manager not configured for update rollback",
			change: UserChange{
				Type:     ChangeUpdate,
				User:     models.User{SubID: "user1", InboundTag: "test-in"},
				Previous: &models.User{SubID: "user1", InboundTag: "test-in"},
			},
			configMgr:   nil,
			wantErr:     true,
			errContains: "config manager is not configured",
		},
		{
			name: "error - missing previous user for update rollback",
			change: UserChange{
				Type:     ChangeUpdate,
				User:     models.User{SubID: "user1", InboundTag: "test-in"},
				Previous: nil,
			},
			configMgr:   &ConfigManager{},
			wantErr:     true,
			errContains: "missing previous user for rollback",
		},
		{
			name: "error - config manager not configured for delete rollback",
			change: UserChange{
				Type:     ChangeDelete,
				User:     models.User{SubID: "user1", InboundTag: "test-in"},
				Previous: &models.User{SubID: "user1", InboundTag: "test-in"},
			},
			configMgr:   nil,
			wantErr:     true,
			errContains: "config manager is not configured",
		},
		{
			name: "error - missing previous user for delete rollback",
			change: UserChange{
				Type:     ChangeDelete,
				User:     models.User{SubID: "user1", InboundTag: "test-in"},
				Previous: nil,
			},
			configMgr:   &ConfigManager{},
			wantErr:     true,
			errContains: "missing previous user for rollback",
		},
		{
			name: "error - unknown change type for rollback",
			change: UserChange{
				Type: "unknown",
				User: models.User{SubID: "user1"},
			},
			configMgr:   nil,
			wantErr:     true,
			errContains: "unknown change type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := &UserStep{
				change:        tt.change,
				configManager: tt.configMgr,
			}

			err := step.Rollback(context.Background())

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					require.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestUserStep_Description tests UserStep Description method
func TestUserStep_Description(t *testing.T) {
	tests := []struct {
		name   string
		change UserChange
		want   string
	}{
		{
			name: "create user",
			change: UserChange{
				Type: ChangeCreate,
				User: models.User{SubID: "user1"},
			},
			want: "user create user1",
		},
		{
			name: "update user",
			change: UserChange{
				Type: ChangeUpdate,
				User: models.User{SubID: "my-user"},
			},
			want: "user update my-user",
		},
		{
			name: "delete user",
			change: UserChange{
				Type: ChangeDelete,
				User: models.User{SubID: "old-user"},
			},
			want: "user delete old-user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := &UserStep{change: tt.change}
			got := step.Description()
			require.Equal(t, tt.want, got)
		})
	}
}

// TestPlan_Execute_ReloadError tests Plan.Execute when reload fails
func TestPlan_Execute_ReloadError(t *testing.T) {
	tests := []struct {
		name        string
		plan        *Plan
		wantErr     bool
		errContains string
	}{
		{
			name: "error - config manager is nil with steps",
			plan: func() *Plan {
				p := NewPlanWithConfigManager(
					[]InboundChange{{Type: ChangeCreate, Inbound: models.Inbound{Tag: "test-in"}}},
					[]UserChange{},
					nil,
				)
				// Replace the step with a mock that succeeds
				p.steps = []Step{&MockStep{failExecute: false}}
				return p
			}(),
			wantErr:     true,
			errContains: "config manager is required to apply and reload changes",
		},
		{
			name: "success - no steps to execute",
			plan: NewPlanWithConfigManager(
				[]InboundChange{},
				[]UserChange{},
				&ConfigManager{},
			),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.plan.Execute(context.Background())

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					require.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestPlan_RollbackError tests Plan.Rollback when a step fails to rollback
func TestPlan_RollbackError(t *testing.T) {
	step := &MockStepWithRollbackFailure{failRollback: true}

	plan := NewPlan([]InboundChange{}, []UserChange{})
	plan.steps = []Step{step}

	err := plan.Rollback(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "rollback failed")
}

// TestEngine_Status tests Engine.Status method
func TestEngine_Status(t *testing.T) {
	client := &MockSingBoxClient{}
	engine := NewEngine(client)

	status := engine.Status()
	require.NotNil(t, status)

	// Verify status is the same instance
	require.Equal(t, engine.status, status)
}

// MockStepWithRollbackFailure is a mock step that fails on rollback
type MockStepWithRollbackFailure struct {
	failRollback bool
}

func (m *MockStepWithRollbackFailure) Execute(ctx context.Context) error {
	return nil
}

func (m *MockStepWithRollbackFailure) Rollback(ctx context.Context) error {
	if m.failRollback {
		return errors.New("rollback failed")
	}
	return nil
}

func (m *MockStepWithRollbackFailure) Description() string {
	return "mock step with rollback failure"
}
