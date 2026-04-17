package metrics

import (
	"fmt"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Additional tests to improve coverage for registerGaugeVec, registerCounterVec, registerGauge

// customErrorRegisterer is a registerer that can return custom errors
type customErrorRegisterer struct {
	*prometheus.Registry
	callCount int
}

func (r *customErrorRegisterer) Register(c prometheus.Collector) error {
	r.callCount++
	// First call succeeds
	if r.callCount == 1 {
		return r.Registry.Register(c)
	}
	// Subsequent calls return a custom error
	return fmt.Errorf("custom registration error")
}

func TestRegisterGaugeVec_ErrorPath(t *testing.T) {
	t.Run("returns error for other registration errors", func(t *testing.T) {
		reg := &customErrorRegisterer{Registry: prometheus.NewRegistry()}

		gauge := prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "test_gauge",
				Help: "test help",
			},
			[]string{"label"},
		)

		// First registration should succeed
		_, err := registerGaugeVec(reg, gauge)
		require.NoError(t, err)

		// Second registration should fail with custom error
		_, err = registerGaugeVec(reg, gauge)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "custom registration error")
	})
}

func TestRegisterCounterVec_ErrorPath(t *testing.T) {
	t.Run("returns error for other registration errors", func(t *testing.T) {
		reg := &customErrorRegisterer{Registry: prometheus.NewRegistry()}

		counter := prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "test_counter",
				Help: "test help",
			},
			[]string{"label"},
		)

		// First registration should succeed
		_, err := registerCounterVec(reg, counter)
		require.NoError(t, err)

		// Second registration should fail with custom error
		_, err = registerCounterVec(reg, counter)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "custom registration error")
	})
}

func TestRegisterGauge_ErrorPath(t *testing.T) {
	t.Run("returns error for other registration errors", func(t *testing.T) {
		reg := &customErrorRegisterer{Registry: prometheus.NewRegistry()}

		gauge := prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "test_gauge",
				Help: "test help",
			},
		)

		// First registration should succeed
		_, err := registerGauge(reg, gauge)
		require.NoError(t, err)

		// Second registration should fail with custom error
		_, err = registerGauge(reg, gauge)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "custom registration error")
	})

	t.Run("returns error for wrong type", func(t *testing.T) {
		reg := prometheus.NewRegistry()
		counter := prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "test_counter",
				Help: "test help",
			},
			[]string{"label"},
		)

		// Register a counter first
		_, err := registerCounterVec(reg, counter)
		require.NoError(t, err)

		// Try to register a gauge with the same name
		gauge := prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "test_counter",
				Help: "test help",
			},
		)

		_, err = registerGauge(reg, gauge)
		require.Error(t, err)
		// Prometheus returns AlreadyRegisteredError with different type message
		assert.Contains(t, err.Error(), "previously registered")
	})
}

// alwaysErrorRegisterer is a registerer that always returns an error
type alwaysErrorRegisterer struct{}

func (r *alwaysErrorRegisterer) Register(c prometheus.Collector) error {
	return fmt.Errorf("registration failed")
}

func (r *alwaysErrorRegisterer) MustRegister(...prometheus.Collector) {
	panic("not implemented")
}

func (r *alwaysErrorRegisterer) Unregister(c prometheus.Collector) bool {
	return false
}

func TestNewCollector_ErrorPaths(t *testing.T) {
	t.Run("returns error when registration fails", func(t *testing.T) {
		reg := &alwaysErrorRegisterer{}

		_, err := NewCollector(reg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "registration failed")
	})
}
