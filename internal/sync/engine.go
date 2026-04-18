package sync

import (
	"context"
	"fmt"

	"github.com/oglenyaboss/sing-box-agent/internal/models"
)

var ErrVersionConflict = fmt.Errorf("version conflict")

// Engine handles desired-state reconciliation.
type Engine struct {
	singbox       SingBoxClient
	configManager *ConfigManager
	status        *SyncStatus
}

// SingBoxClient defines the interface for sing-box operations.
// This allows the engine to work with both real and mock implementations.
type SingBoxClient interface {
	GetInbounds(ctx context.Context) ([]models.Inbound, error)
	GetUsers(ctx context.Context) ([]models.User, error)
	CreateInbound(ctx context.Context, inbound models.Inbound) error
	UpdateInbound(ctx context.Context, inbound models.Inbound) error
	DeleteInbound(ctx context.Context, tag string) error
	CreateUser(ctx context.Context, user models.User) error
	UpdateUser(ctx context.Context, user models.User) error
	DeleteUser(ctx context.Context, subID string) error
}

// NewEngine creates a new sync engine.
func NewEngine(singbox SingBoxClient) *Engine {
	return &Engine{
		singbox: singbox,
		status:  NewSyncStatus(),
	}
}

func NewEngineWithConfigManager(singbox SingBoxClient, configManager *ConfigManager) *Engine {
	return &Engine{
		singbox:       singbox,
		configManager: configManager,
		status:        NewSyncStatus(),
	}
}

// Reconcile compares desired state with current state and generates a plan.
func (e *Engine) Reconcile(ctx context.Context, desired *models.DesiredState) (*Plan, error) {
	e.status.UpdateSyncing()

	currentInbounds, err := e.singbox.GetInbounds(ctx)
	if err != nil {
		e.status.UpdateError(err.Error())
		return nil, fmt.Errorf("failed to get current inbounds: %w", err)
	}

	currentUsers, err := e.singbox.GetUsers(ctx)
	if err != nil {
		e.status.UpdateError(err.Error())
		return nil, fmt.Errorf("failed to get current users: %w", err)
	}

	inboundChanges := DiffInbounds(desired.Inbounds, currentInbounds)
	userChanges := DiffUsers(desired.Users, currentUsers)

	return NewPlanWithConfigManager(inboundChanges, userChanges, e.configManager), nil
}

// Apply executes a plan and updates the sync status.
func (e *Engine) Apply(ctx context.Context, plan *Plan, version int) error {
	e.status.UpdateSyncing()

	if err := plan.Execute(ctx); err != nil {
		e.status.UpdateError(err.Error())
		IncrementErrors()
		return err
	}

	changesCount := plan.StepsCount()
	e.status.UpdateSync(version, changesCount)

	for i := 0; i < changesCount; i++ {
		IncrementChangesApplied()
	}

	return nil
}

// DryRun generates a plan without applying changes.
func (e *Engine) DryRun(ctx context.Context, desired *models.DesiredState) (*Plan, error) {
	return e.Reconcile(ctx, desired)
}

// Status returns the current sync status.
func (e *Engine) Status() *SyncStatus {
	return e.status
}
