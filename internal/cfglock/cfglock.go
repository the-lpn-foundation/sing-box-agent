// Package cfglock provides process-wide per-path mutexes used to serialise
// read-modify-write cycles on the sing-box config file across independent
// components (REST CRUD client and desired-state manager) that would
// otherwise lose each other's updates.
package cfglock

import (
	"path/filepath"
	"sync"
)

var (
	muRegistry sync.RWMutex
	registry   = map[string]*sync.Mutex{}
)

// For returns the process-wide mutex guarding the config file at path.
// Paths are normalised (filepath.Abs) so the same file always maps to the
// same mutex, regardless of how the caller spells the path.
func For(path string) *sync.Mutex {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}

	muRegistry.RLock()
	m, ok := registry[abs]
	muRegistry.RUnlock()
	if ok {
		return m
	}

	muRegistry.Lock()
	defer muRegistry.Unlock()
	// Re-check under the write lock: another goroutine may have inserted
	// the mutex between the RUnlock and this Lock.
	if m, ok := registry[abs]; ok {
		return m
	}
	m = &sync.Mutex{}
	registry[abs] = m
	return m
}
