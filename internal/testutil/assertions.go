package testutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// NewAssert creates a new assert.Assertions for the given testing.TB.
func NewAssert(t testing.TB) *assert.Assertions {
	return assert.New(t)
}
