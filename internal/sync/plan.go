package sync

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrPlanExecutionFailed = errors.New("plan execution failed")
	ErrRollbackFailed      = errors.New("rollback failed")
)

// Step represents a single step in a sync plan.
type Step interface {
	Execute(ctx context.Context) error
	Rollback(ctx context.Context) error
	Description() string
}

// Plan represents a sync plan with ordered steps.
type Plan struct {
	InboundChanges []InboundChange
	UserChanges    []UserChange
	configManager  *ConfigManager
	steps          []Step
}

// NewPlan creates a new plan from inbound and user changes.
func NewPlan(inboundChanges []InboundChange, userChanges []UserChange) *Plan {
	return NewPlanWithConfigManager(inboundChanges, userChanges, nil)
}

func NewPlanWithConfigManager(inboundChanges []InboundChange, userChanges []UserChange, configManager *ConfigManager) *Plan {
	p := &Plan{
		InboundChanges: inboundChanges,
		UserChanges:    userChanges,
		configManager:  configManager,
	}

	p.buildSteps()
	return p
}

// buildSteps converts changes into executable steps in order.
func (p *Plan) buildSteps() {
	p.steps = make([]Step, 0, len(p.InboundChanges)+len(p.UserChanges))

	for _, change := range p.InboundChanges {
		p.steps = append(p.steps, &InboundStep{change: change, configManager: p.configManager})
	}

	for _, change := range p.UserChanges {
		p.steps = append(p.steps, &UserStep{change: change, configManager: p.configManager})
	}
}

// Execute runs all steps in the plan.
// If any step fails, it attempts to rollback all executed steps.
func (p *Plan) Execute(ctx context.Context) error {
	executed := make([]Step, 0, len(p.steps))

	for i, step := range p.steps {
		executed = append(executed, step)
		if err := step.Execute(ctx); err != nil {
			rollbackErr := p.rollbackExecuted(ctx, executed)
			if rollbackErr != nil {
				return fmt.Errorf("%w: %v (rollback also failed: %v)", ErrPlanExecutionFailed, err, rollbackErr)
			}
			return fmt.Errorf("%w: step %d (%s): %v", ErrPlanExecutionFailed, i, step.Description(), err)
		}
	}

	if len(executed) > 0 {
		if p.configManager == nil {
			return fmt.Errorf("%w: config manager is required to apply and reload changes", ErrPlanExecutionFailed)
		}

		if err := p.configManager.Reload(ctx); err != nil {
			rollbackErr := p.rollbackExecuted(ctx, executed)
			if rollbackErr != nil {
				return fmt.Errorf("%w: reload failed: %v (rollback also failed: %v)", ErrPlanExecutionFailed, err, rollbackErr)
			}

			if rollbackReloadErr := p.configManager.Reload(ctx); rollbackReloadErr != nil {
				return fmt.Errorf("%w: reload failed: %v (rollback reload also failed: %v)", ErrPlanExecutionFailed, err, rollbackReloadErr)
			}

			return fmt.Errorf("%w: reload failed: %v", ErrPlanExecutionFailed, err)
		}
	}

	return nil
}

// rollbackExecuted rolls back steps that were already executed.
func (p *Plan) rollbackExecuted(ctx context.Context, executed []Step) error {
	for i := len(executed) - 1; i >= 0; i-- {
		step := executed[i]
		if err := step.Rollback(ctx); err != nil {
			return fmt.Errorf("%w at step %d (%s): %v", ErrRollbackFailed, i, step.Description(), err)
		}
	}
	return nil
}

// Rollback rolls back all steps in reverse order.
func (p *Plan) Rollback(ctx context.Context) error {
	for i := len(p.steps) - 1; i >= 0; i-- {
		step := p.steps[i]
		if err := step.Rollback(ctx); err != nil {
			return fmt.Errorf("%w at step %d (%s): %v", ErrRollbackFailed, i, step.Description(), err)
		}
	}
	return nil
}

// StepsCount returns the number of steps in the plan.
func (p *Plan) StepsCount() int {
	return len(p.steps)
}

// InboundStep implements Step for inbound changes.
type InboundStep struct {
	change        InboundChange
	configManager *ConfigManager
}

func (s *InboundStep) Execute(ctx context.Context) error {
	switch s.change.Type {
	case ChangeCreate:
		return s.executeCreate(ctx)
	case ChangeUpdate:
		return s.executeUpdate(ctx)
	case ChangeDelete:
		return s.executeDelete(ctx)
	default:
		return fmt.Errorf("unknown change type: %s", s.change.Type)
	}
}

func (s *InboundStep) Rollback(ctx context.Context) error {
	switch s.change.Type {
	case ChangeCreate:
		return s.rollbackCreate(ctx)
	case ChangeUpdate:
		return s.rollbackUpdate(ctx)
	case ChangeDelete:
		return s.rollbackDelete(ctx)
	default:
		return fmt.Errorf("unknown change type: %s", s.change.Type)
	}
}

func (s *InboundStep) Description() string {
	return fmt.Sprintf("inbound %s %s", s.change.Type, s.change.Inbound.Tag)
}

func (s *InboundStep) executeCreate(ctx context.Context) error {
	if s.configManager == nil {
		return errors.New("config manager is not configured")
	}

	return s.configManager.AddInbound(s.change.Inbound)
}

func (s *InboundStep) rollbackCreate(ctx context.Context) error {
	if s.configManager == nil {
		return errors.New("config manager is not configured")
	}

	return s.configManager.DeleteInbound(s.change.Inbound.Tag)
}

func (s *InboundStep) executeUpdate(ctx context.Context) error {
	if s.configManager == nil {
		return errors.New("config manager is not configured")
	}

	return s.configManager.UpdateInbound(s.change.Inbound)
}

func (s *InboundStep) rollbackUpdate(ctx context.Context) error {
	if s.configManager == nil {
		return errors.New("config manager is not configured")
	}
	if s.change.Previous == nil {
		return errors.New("missing previous inbound for rollback")
	}

	return s.configManager.UpdateInbound(*s.change.Previous)
}

func (s *InboundStep) executeDelete(ctx context.Context) error {
	if s.configManager == nil {
		return errors.New("config manager is not configured")
	}

	return s.configManager.DeleteInbound(s.change.Inbound.Tag)
}

func (s *InboundStep) rollbackDelete(ctx context.Context) error {
	if s.configManager == nil {
		return errors.New("config manager is not configured")
	}
	if s.change.Previous == nil {
		return errors.New("missing previous inbound for rollback")
	}

	return s.configManager.AddInbound(*s.change.Previous)
}

// UserStep implements Step for user changes.
type UserStep struct {
	change        UserChange
	configManager *ConfigManager
}

func (s *UserStep) Execute(ctx context.Context) error {
	switch s.change.Type {
	case ChangeCreate:
		return s.executeCreate(ctx)
	case ChangeUpdate:
		return s.executeUpdate(ctx)
	case ChangeDelete:
		return s.executeDelete(ctx)
	default:
		return fmt.Errorf("unknown change type: %s", s.change.Type)
	}
}

func (s *UserStep) Rollback(ctx context.Context) error {
	switch s.change.Type {
	case ChangeCreate:
		return s.rollbackCreate(ctx)
	case ChangeUpdate:
		return s.rollbackUpdate(ctx)
	case ChangeDelete:
		return s.rollbackDelete(ctx)
	default:
		return fmt.Errorf("unknown change type: %s", s.change.Type)
	}
}

func (s *UserStep) Description() string {
	return fmt.Sprintf("user %s %s", s.change.Type, s.change.User.SubID)
}

func (s *UserStep) executeCreate(ctx context.Context) error {
	if s.configManager == nil {
		return errors.New("config manager is not configured")
	}

	return s.configManager.AddUser(s.change.User.InboundTag, s.change.User)
}

func (s *UserStep) rollbackCreate(ctx context.Context) error {
	if s.configManager == nil {
		return errors.New("config manager is not configured")
	}

	return s.configManager.DeleteUser(s.change.User.InboundTag, s.change.User.SubID)
}

func (s *UserStep) executeUpdate(ctx context.Context) error {
	if s.configManager == nil {
		return errors.New("config manager is not configured")
	}

	return s.configManager.UpdateUser(s.change.User.InboundTag, s.change.User)
}

func (s *UserStep) rollbackUpdate(ctx context.Context) error {
	if s.configManager == nil {
		return errors.New("config manager is not configured")
	}
	if s.change.Previous == nil {
		return errors.New("missing previous user for rollback")
	}

	return s.configManager.UpdateUser(s.change.Previous.InboundTag, *s.change.Previous)
}

func (s *UserStep) executeDelete(ctx context.Context) error {
	if s.configManager == nil {
		return errors.New("config manager is not configured")
	}

	return s.configManager.DeleteUser(s.change.User.InboundTag, s.change.User.SubID)
}

func (s *UserStep) rollbackDelete(ctx context.Context) error {
	if s.configManager == nil {
		return errors.New("config manager is not configured")
	}
	if s.change.Previous == nil {
		return errors.New("missing previous user for rollback")
	}

	return s.configManager.AddUser(s.change.Previous.InboundTag, *s.change.Previous)
}
