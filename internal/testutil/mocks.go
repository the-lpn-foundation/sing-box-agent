package testutil

import (
	"github.com/stretchr/testify/mock"
)

// MockHelper provides common mock patterns for testing.
type MockHelper struct {
	mock.Mock
}

// On calls mock.On and returns the Call for chaining.
func (m *MockHelper) On(method string, args ...interface{}) *mock.Call {
	return m.Mock.On(method, args...)
}

// Expect calls mock.On and returns the MockHelper for chaining.
func (m *MockHelper) Expect(method string, args ...interface{}) *MockHelper {
	m.Mock.On(method, args...)
	return m
}
