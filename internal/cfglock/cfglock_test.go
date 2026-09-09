package cfglock

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// Parallel goroutines incrementing a shared counter under For(samePath)
// must produce a deterministic result — the mutex actually serialises them.
func TestFor_SerialisesConcurrentAccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	lock := For(path)

	const goroutines = 10
	const iterations = 100

	var wg sync.WaitGroup
	counter := 0
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				lock.Lock()
				counter++
				lock.Unlock()
			}
		}()
	}
	wg.Wait()

	if counter != goroutines*iterations {
		t.Fatalf("counter = %d, want %d", counter, goroutines*iterations)
	}
}

// Different paths must map to different mutexes; the same path must always
// map to the same mutex.
func TestFor_MapsPathsToMutexes(t *testing.T) {
	a := For(filepath.Join(t.TempDir(), "a.json"))
	b := For(filepath.Join(t.TempDir(), "b.json"))

	if a == b {
		t.Fatal("different paths must map to different mutexes")
	}

	abs := filepath.Join(t.TempDir(), "config.json")
	first := For(abs)
	second := For(abs)
	if first != second {
		t.Fatal("same path must map to the same mutex")
	}
}

// Paths are normalised: a relative spelling resolving to the same file must
// share the mutex with the absolute spelling of that file.
func TestFor_NormalisesRelativePaths(t *testing.T) {
	dir := t.TempDir()
	abs := filepath.Join(dir, "config.json")
	if err := os.WriteFile(abs, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	// Resolve symlinks (e.g. /var/folders -> /private/var/folders on macOS),
	// since os.Getwd reports the physical path used by filepath.Abs.
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		t.Fatalf("evalsymlinks: %v", err)
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() {
		if err := os.Chdir(oldWd); err != nil {
			t.Errorf("restore wd: %v", err)
		}
	}()

	if For("config.json") != For(resolved) {
		t.Fatal("relative and absolute spellings of one path must share a mutex")
	}
}
