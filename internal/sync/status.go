package sync

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	syncStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "singbox_agent_sync_status",
			Help: "Current sync status (0=synced, 1=syncing, 2=error)",
		},
		[]string{"state"},
	)

	syncTimestamp = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "singbox_agent_sync_timestamp_seconds",
			Help: "Unix timestamp of the last successful sync",
		},
	)

	changesApplied = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "singbox_agent_sync_changes_applied_total",
			Help: "Total number of changes applied during sync",
		},
	)

	syncErrors = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "singbox_agent_sync_errors_total",
			Help: "Total number of sync errors",
		},
	)
)

func init() {
	prometheus.MustRegister(syncStatus)
	prometheus.MustRegister(syncTimestamp)
	prometheus.MustRegister(changesApplied)
	prometheus.MustRegister(syncErrors)
}

// SyncState represents the state of the sync process.
type SyncState string

const (
	SyncStateSynced  SyncState = "synced"
	SyncStateSyncing SyncState = "syncing"
	SyncStateError   SyncState = "error"
)

// SyncStatus represents the current status of the sync engine.
type SyncStatus struct {
	mu             sync.RWMutex
	LastSync       time.Time
	State          SyncState
	Error          string
	CurrentVersion int
	ChangesApplied int
	InSync         bool
}

// NewSyncStatus creates a new SyncStatus.
func NewSyncStatus() *SyncStatus {
	return &SyncStatus{
		State:          SyncStateSynced,
		CurrentVersion: -1,
		InSync:         true,
	}
}

// Get returns the current sync status.
func (s *SyncStatus) Get() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return map[string]interface{}{
		"last_sync":       s.LastSync.Format(time.RFC3339),
		"state":           s.State,
		"error":           s.Error,
		"current_version": s.CurrentVersion,
		"changes_applied": s.ChangesApplied,
		"in_sync":         s.InSync,
	}
}

// UpdateSync updates the status after a successful sync.
func (s *SyncStatus) UpdateSync(version, changes int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.LastSync = time.Now()
	s.State = SyncStateSynced
	s.Error = ""
	s.CurrentVersion = version
	s.ChangesApplied = changes
	s.InSync = true

	s.updateMetrics()
}

// UpdateError updates the status with an error.
func (s *SyncStatus) UpdateError(err string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.State = SyncStateError
	s.Error = err
	s.InSync = false

	s.updateMetrics()
}

// UpdateSyncing sets the state to syncing.
func (s *SyncStatus) UpdateSyncing() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.State = SyncStateSyncing

	s.updateMetrics()
}

// updateMetrics updates Prometheus metrics based on current status.
func (s *SyncStatus) updateMetrics() {
	syncStatus.Reset()
	syncStatus.WithLabelValues(string(s.State)).Set(1)

	if !s.LastSync.IsZero() {
		syncTimestamp.Set(float64(s.LastSync.Unix()))
	}
}

// IncrementErrors increments the error counter.
func IncrementErrors() {
	syncErrors.Inc()
}

// IncrementChangesApplied increments the changes applied counter.
func IncrementChangesApplied() {
	changesApplied.Inc()
}
