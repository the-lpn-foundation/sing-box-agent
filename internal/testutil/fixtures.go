package testutil

import (
	"os"
	"path/filepath"
)

// FixtureLoader provides helpers for loading test fixtures from files.
type FixtureLoader struct {
	baseDir string
}

// NewFixtureLoader creates a new fixture loader with the given base directory.
func NewFixtureLoader(baseDir string) *FixtureLoader {
	return &FixtureLoader{baseDir: baseDir}
}

// LoadFixture loads a fixture file from the fixtures directory.
func (fl *FixtureLoader) LoadFixture(filename string) ([]byte, error) {
	path := filepath.Join(fl.baseDir, filename)
	return os.ReadFile(path)
}

// LoadFixtureString loads a fixture file and returns its content as a string.
func (fl *FixtureLoader) LoadFixtureString(filename string) (string, error) {
	data, err := fl.LoadFixture(filename)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
