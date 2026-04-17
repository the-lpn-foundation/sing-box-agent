package server

import (
	"context"
	"log/slog"
	"time"

	"github.com/lenya/sing-box-agent/internal/client"
	"github.com/lenya/sing-box-agent/internal/models"
	syncpkg "github.com/lenya/sing-box-agent/internal/sync"
)

const (
	// DefaultHeartbeatInterval is the interval between heartbeat calls.
	DefaultHeartbeatInterval = 30 * time.Second
	// DefaultDriftCheckInterval is the interval between drift checks (desired-state re-pull).
	DefaultDriftCheckInterval = 5 * time.Minute
)

// Lifecycle manages background tasks: heartbeat, desired-state fetch, drift checks.
type Lifecycle struct {
	fastify    *client.Client
	syncEngine *syncpkg.Engine
	logger     *slog.Logger
	version    string
	cancel     context.CancelFunc
}

// NewLifecycle creates a new lifecycle manager.
// If fastify is nil, lifecycle operations are no-ops.
func NewLifecycle(fastify *client.Client, syncEngine *syncpkg.Engine, logger *slog.Logger, version string) *Lifecycle {
	return &Lifecycle{
		fastify:    fastify,
		syncEngine: syncEngine,
		logger:     logger,
		version:    version,
	}
}

// Start begins background tasks: initial desired-state fetch, heartbeat loop, drift check loop.
// This is non-blocking — it spawns goroutines and returns immediately.
func (l *Lifecycle) Start(ctx context.Context) {
	if l.fastify == nil {
		l.logger.Info("fastify client not configured, skipping lifecycle tasks")
		return
	}

	ctx, l.cancel = context.WithCancel(ctx)

	// Initial desired-state fetch on startup
	go l.fetchAndApplyDesiredState(ctx)

	// Heartbeat loop
	go l.heartbeatLoop(ctx)

	// Drift check loop
	go l.driftCheckLoop(ctx)

	l.logger.Info("lifecycle tasks started",
		slog.Duration("heartbeat_interval", DefaultHeartbeatInterval),
		slog.Duration("drift_check_interval", DefaultDriftCheckInterval),
	)
}

// Stop cancels all background tasks.
func (l *Lifecycle) Stop() {
	if l.cancel != nil {
		l.cancel()
	}
}

// fetchAndApplyDesiredState fetches desired state from central API and applies it.
func (l *Lifecycle) fetchAndApplyDesiredState(ctx context.Context) {
	l.logger.Info("fetching initial desired state from central API")

	cfg, err := l.fastify.FetchConfig(ctx)
	if err != nil {
		l.logger.Error("failed to fetch desired state", slog.Any("error", err))
		return
	}

	desired := l.configToDesiredState(cfg)

	plan, err := l.syncEngine.Reconcile(ctx, desired)
	if err != nil {
		l.logger.Error("failed to reconcile desired state", slog.Any("error", err))
		return
	}

	if plan.StepsCount() > 0 {
		if err := l.syncEngine.Apply(ctx, plan, desired.Version); err != nil {
			l.logger.Error("failed to apply desired state", slog.Any("error", err))
			return
		}
		l.logger.Info("initial desired state applied",
			slog.Int("version", desired.Version),
			slog.Int("changes", plan.StepsCount()),
		)
	} else {
		l.logger.Info("no changes needed from desired state", slog.Int("version", desired.Version))
	}

	// Report success status
	l.reportStatus(ctx, "synced", desired.Version, "initial sync completed")
}

// heartbeatLoop sends periodic heartbeats to the central API.
func (l *Lifecycle) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(DefaultHeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			l.logger.Debug("heartbeat loop stopped")
			return
		case <-ticker.C:
			if err := l.fastify.Heartbeat(ctx); err != nil {
				l.logger.Warn("heartbeat failed", slog.Any("error", err))
			} else {
				l.logger.Debug("heartbeat sent")
			}
		}
	}
}

// driftCheckLoop periodically re-fetches desired state and reconciles.
func (l *Lifecycle) driftCheckLoop(ctx context.Context) {
	ticker := time.NewTicker(DefaultDriftCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			l.logger.Debug("drift check loop stopped")
			return
		case <-ticker.C:
			l.logger.Debug("running drift check")
			l.fetchAndApplyDesiredState(ctx)
		}
	}
}

// reportStatus reports agent status to the central API.
func (l *Lifecycle) reportStatus(ctx context.Context, state string, version int, message string) {
	status := client.Status{
		State:     state,
		Version:   version,
		Message:   message,
		UpdatedAt: time.Now(),
	}

	if err := l.fastify.ReportStatus(ctx, status); err != nil {
		l.logger.Warn("failed to report status", slog.Any("error", err))
	}
}

// configToDesiredState converts a Fastify config response to a DesiredState.
func (l *Lifecycle) configToDesiredState(cfg *client.Config) *models.DesiredState {
	desired := &models.DesiredState{
		Version:   cfg.Version,
		Timestamp: cfg.Timestamp,
	}

	for _, ib := range cfg.Inbounds {
		desired.Inbounds = append(desired.Inbounds, models.Inbound{
			Tag:     ib.Tag,
			Type:    ib.Type,
			Listen:  ib.Listen,
			Port:    ib.Port,
			Options: ib.Options,
		})
	}

	for _, u := range cfg.Users {
		desired.Users = append(desired.Users, models.User{
			SubID:      u.SubID,
			UUID:       u.UUID,
			InboundTag: u.InboundTag,
			Email:      u.Email,
			Enabled:    u.Enabled,
			Flow:       u.Flow,
		})
	}

	return desired
}
