package metrics

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCollector(t *testing.T) {
	t.Run("with custom registerer", func(t *testing.T) {
		reg := prometheus.NewRegistry()
		c, err := NewCollector(reg)
		require.NoError(t, err)
		require.NotNil(t, c)
		assert.NotNil(t, c.agentInfo)
		assert.NotNil(t, c.syncStatus)
		assert.NotNil(t, c.syncTimestamp)
		assert.NotNil(t, c.inboundUsers)
		assert.NotNil(t, c.inboundConnections)
		assert.NotNil(t, c.trafficUp)
		assert.NotNil(t, c.trafficDown)
		assert.NotNil(t, c.lastUp)
		assert.NotNil(t, c.lastDown)
	})

	t.Run("with nil registerer uses default", func(t *testing.T) {
		c, err := NewCollector(nil)
		require.NoError(t, err)
		require.NotNil(t, c)
	})

	t.Run("already registered metrics", func(t *testing.T) {
		reg := prometheus.NewRegistry()
		c1, err := NewCollector(reg)
		require.NoError(t, err)
		require.NotNil(t, c1)

		// Second collector should get existing metrics
		c2, err := NewCollector(reg)
		require.NoError(t, err)
		require.NotNil(t, c2)
	})
}

func TestCollector_SetAgentInfo(t *testing.T) {
	reg := prometheus.NewRegistry()
	c, err := NewCollector(reg)
	require.NoError(t, err)

	t.Run("sets agent info metrics", func(t *testing.T) {
		c.SetAgentInfo("1.0.0", "1.8.0")

		metrics, err := reg.Gather()
		require.NoError(t, err)

		var found bool
		for _, m := range metrics {
			if m.GetName() == "singbox_agent_info" {
				found = true
				require.Len(t, m.GetMetric(), 1)
				metric := m.GetMetric()[0]
				assert.Equal(t, 1.0, metric.GetGauge().GetValue())

				labels := make(map[string]string)
				for _, label := range metric.GetLabel() {
					labels[label.GetName()] = label.GetValue()
				}
				assert.Equal(t, "1.0.0", labels["version"])
				assert.Equal(t, "1.8.0", labels["singbox_version"])
			}
		}
		assert.True(t, found, "singbox_agent_info metric not found")
	})

	t.Run("overwrites previous values", func(t *testing.T) {
		c.SetAgentInfo("1.0.0", "1.8.0")
		c.SetAgentInfo("2.0.0", "1.9.0")

		metrics, err := reg.Gather()
		require.NoError(t, err)

		for _, m := range metrics {
			if m.GetName() == "singbox_agent_info" {
				metric := m.GetMetric()[0]
				labels := make(map[string]string)
				for _, label := range metric.GetLabel() {
					labels[label.GetName()] = label.GetValue()
				}
				assert.Equal(t, "2.0.0", labels["version"])
				assert.Equal(t, "1.9.0", labels["singbox_version"])
			}
		}
	})
}

func TestCollector_SetSyncStatus(t *testing.T) {
	reg := prometheus.NewRegistry()
	c, err := NewCollector(reg)
	require.NoError(t, err)

	t.Run("sets sync status with timestamp", func(t *testing.T) {
		lastSync := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
		c.SetSyncStatus("synced", lastSync)

		metrics, err := reg.Gather()
		require.NoError(t, err)

		var statusFound, timestampFound bool
		for _, m := range metrics {
			if m.GetName() == "singbox_agent_sync_status" {
				statusFound = true
				require.Len(t, m.GetMetric(), 1)
				metric := m.GetMetric()[0]
				assert.Equal(t, 1.0, metric.GetGauge().GetValue())

				labels := make(map[string]string)
				for _, label := range metric.GetLabel() {
					labels[label.GetName()] = label.GetValue()
				}
				assert.Equal(t, "synced", labels["state"])
			}
			if m.GetName() == "singbox_agent_sync_timestamp_seconds" {
				timestampFound = true
				require.Len(t, m.GetMetric(), 1)
				metric := m.GetMetric()[0]
				assert.Equal(t, float64(lastSync.Unix()), metric.GetGauge().GetValue())
			}
		}
		assert.True(t, statusFound, "sync status metric not found")
		assert.True(t, timestampFound, "sync timestamp metric not found")
	})

	t.Run("sets sync status with zero time", func(t *testing.T) {
		c.SetSyncStatus("syncing", time.Time{})

		metrics, err := reg.Gather()
		require.NoError(t, err)

		for _, m := range metrics {
			if m.GetName() == "singbox_agent_sync_status" {
				metric := m.GetMetric()[0]
				labels := make(map[string]string)
				for _, label := range metric.GetLabel() {
					labels[label.GetName()] = label.GetValue()
				}
				assert.Equal(t, "syncing", labels["state"])
			}
		}
	})

	t.Run("overwrites previous values", func(t *testing.T) {
		c.SetSyncStatus("synced", time.Now())
		c.SetSyncStatus("error", time.Now())

		metrics, err := reg.Gather()
		require.NoError(t, err)

		for _, m := range metrics {
			if m.GetName() == "singbox_agent_sync_status" {
				metric := m.GetMetric()[0]
				labels := make(map[string]string)
				for _, label := range metric.GetLabel() {
					labels[label.GetName()] = label.GetValue()
				}
				assert.Equal(t, "error", labels["state"])
			}
		}
	})
}

func TestCollector_ObserveInbounds(t *testing.T) {
	reg := prometheus.NewRegistry()
	c, err := NewCollector(reg)
	require.NoError(t, err)

	t.Run("observes single inbound", func(t *testing.T) {
		stats := []InboundStat{
			{
				Tag:         "vless-in",
				Type:        "vless",
				Users:       5,
				Connections: 10,
				UpBytes:     1000,
				DownBytes:   2000,
			},
		}
		c.ObserveInbounds(stats)

		metrics, err := reg.Gather()
		require.NoError(t, err)

		var usersFound, connectionsFound, upFound, downFound bool
		for _, m := range metrics {
			if m.GetName() == "singbox_inbound_users" {
				usersFound = true
				metric := m.GetMetric()[0]
				assert.Equal(t, 5.0, metric.GetGauge().GetValue())
			}
			if m.GetName() == "singbox_inbound_connections" {
				connectionsFound = true
				metric := m.GetMetric()[0]
				assert.Equal(t, 10.0, metric.GetGauge().GetValue())
			}
			if m.GetName() == "singbox_inbound_traffic_up_bytes_total" {
				upFound = true
				metric := m.GetMetric()[0]
				assert.Equal(t, 1000.0, metric.GetCounter().GetValue())
			}
			if m.GetName() == "singbox_inbound_traffic_down_bytes_total" {
				downFound = true
				metric := m.GetMetric()[0]
				assert.Equal(t, 2000.0, metric.GetCounter().GetValue())
			}
		}
		assert.True(t, usersFound, "users metric not found")
		assert.True(t, connectionsFound, "connections metric not found")
		assert.True(t, upFound, "up traffic metric not found")
		assert.True(t, downFound, "down traffic metric not found")
	})

	t.Run("observes multiple inbounds", func(t *testing.T) {
		stats := []InboundStat{
			{
				Tag:         "vless-in",
				Type:        "vless",
				Users:       5,
				Connections: 10,
				UpBytes:     1000,
				DownBytes:   2000,
			},
			{
				Tag:         "hysteria2-in",
				Type:        "hysteria2",
				Users:       3,
				Connections: 7,
				UpBytes:     500,
				DownBytes:   1500,
			},
		}
		c.ObserveInbounds(stats)

		metrics, err := reg.Gather()
		require.NoError(t, err)

		for _, m := range metrics {
			if m.GetName() == "singbox_inbound_users" {
				require.Len(t, m.GetMetric(), 2)
			}
			if m.GetName() == "singbox_inbound_connections" {
				require.Len(t, m.GetMetric(), 2)
			}
		}
	})

	t.Run("skips empty tag", func(t *testing.T) {
		stats := []InboundStat{
			{
				Tag:         "",
				Type:        "vless",
				Users:       5,
				Connections: 10,
				UpBytes:     1000,
				DownBytes:   2000,
			},
			{
				Tag:         "valid-in",
				Type:        "vless",
				Users:       3,
				Connections: 5,
				UpBytes:     500,
				DownBytes:   1000,
			},
		}
		c.ObserveInbounds(stats)

		metrics, err := reg.Gather()
		require.NoError(t, err)

		for _, m := range metrics {
			if m.GetName() == "singbox_inbound_users" {
				// Only one metric should be present (empty tag skipped)
				require.Len(t, m.GetMetric(), 1)
			}
		}
	})

	t.Run("calculates delta for traffic", func(t *testing.T) {
		reg := prometheus.NewRegistry()
		c, err := NewCollector(reg)
		require.NoError(t, err)

		// First observation
		stats1 := []InboundStat{
			{
				Tag:         "vless-in",
				Type:        "vless",
				Users:       5,
				Connections: 10,
				UpBytes:     1000,
				DownBytes:   2000,
			},
		}
		c.ObserveInbounds(stats1)

		// Second observation with increased traffic
		stats2 := []InboundStat{
			{
				Tag:         "vless-in",
				Type:        "vless",
				Users:       7,
				Connections: 15,
				UpBytes:     1500,
				DownBytes:   3000,
			},
		}
		c.ObserveInbounds(stats2)

		metrics, err := reg.Gather()
		require.NoError(t, err)

		for _, m := range metrics {
			if m.GetName() == "singbox_inbound_traffic_up_bytes_total" {
				metric := m.GetMetric()[0]
				// Should be 1000 + (1500-1000) = 1500
				assert.Equal(t, 1500.0, metric.GetCounter().GetValue())
			}
			if m.GetName() == "singbox_inbound_traffic_down_bytes_total" {
				metric := m.GetMetric()[0]
				// Should be 2000 + (3000-2000) = 3000
				assert.Equal(t, 3000.0, metric.GetCounter().GetValue())
			}
		}
	})

	t.Run("handles counter reset", func(t *testing.T) {
		reg := prometheus.NewRegistry()
		c, err := NewCollector(reg)
		require.NoError(t, err)

		// First observation
		stats1 := []InboundStat{
			{
				Tag:         "vless-in",
				Type:        "vless",
				Users:       5,
				Connections: 10,
				UpBytes:     1000,
				DownBytes:   2000,
			},
		}
		c.ObserveInbounds(stats1)

		// Second observation with counter reset (lower values)
		stats2 := []InboundStat{
			{
				Tag:         "vless-in",
				Type:        "vless",
				Users:       7,
				Connections: 15,
				UpBytes:     500,
				DownBytes:   1000,
			},
		}
		c.ObserveInbounds(stats2)

		metrics, err := reg.Gather()
		require.NoError(t, err)

		for _, m := range metrics {
			if m.GetName() == "singbox_inbound_traffic_up_bytes_total" {
				metric := m.GetMetric()[0]
				// Should be 1000 + 500 (reset) = 1500
				assert.Equal(t, 1500.0, metric.GetCounter().GetValue())
			}
			if m.GetName() == "singbox_inbound_traffic_down_bytes_total" {
				metric := m.GetMetric()[0]
				// Should be 2000 + 1000 (reset) = 3000
				assert.Equal(t, 3000.0, metric.GetCounter().GetValue())
			}
		}
	})

	t.Run("resets gauge metrics on each observation", func(t *testing.T) {
		stats1 := []InboundStat{
			{
				Tag:         "vless-in",
				Type:        "vless",
				Users:       5,
				Connections: 10,
				UpBytes:     1000,
				DownBytes:   2000,
			},
		}
		c.ObserveInbounds(stats1)

		// Second observation with different inbound
		stats2 := []InboundStat{
			{
				Tag:         "hysteria2-in",
				Type:        "hysteria2",
				Users:       3,
				Connections: 7,
				UpBytes:     500,
				DownBytes:   1500,
			},
		}
		c.ObserveInbounds(stats2)

		metrics, err := reg.Gather()
		require.NoError(t, err)

		for _, m := range metrics {
			if m.GetName() == "singbox_inbound_users" {
				// Only the second inbound should be present
				require.Len(t, m.GetMetric(), 1)
				metric := m.GetMetric()[0]
				assert.Equal(t, 3.0, metric.GetGauge().GetValue())
			}
		}
	})

	t.Run("handles empty stats slice", func(t *testing.T) {
		c.ObserveInbounds([]InboundStat{})
		// Should not panic
	})

	t.Run("handles nil stats slice", func(t *testing.T) {
		c.ObserveInbounds(nil)
		// Should not panic
	})
}

func TestDeltaCounter(t *testing.T) {
	tests := []struct {
		name     string
		previous uint64
		current  uint64
		expected uint64
	}{
		{
			name:     "zero to positive",
			previous: 0,
			current:  100,
			expected: 100,
		},
		{
			name:     "positive increase",
			previous: 50,
			current:  100,
			expected: 50,
		},
		{
			name:     "counter reset",
			previous: 100,
			current:  50,
			expected: 50,
		},
		{
			name:     "same value",
			previous: 100,
			current:  100,
			expected: 0,
		},
		{
			name:     "large values",
			previous: 1000000,
			current:  2000000,
			expected: 1000000,
		},
		{
			name:     "max uint64 overflow scenario",
			previous: 18446744073709551615,
			current:  100,
			expected: 100,
		},
		{
			name:     "zero to zero",
			previous: 0,
			current:  0,
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := deltaCounter(tt.previous, tt.current)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRegisterGaugeVec(t *testing.T) {
	t.Run("registers new gauge vec", func(t *testing.T) {
		reg := prometheus.NewRegistry()
		gauge := prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "test_gauge",
				Help: "test help",
			},
			[]string{"label"},
		)

		result, err := registerGaugeVec(reg, gauge)
		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("returns existing gauge vec", func(t *testing.T) {
		reg := prometheus.NewRegistry()
		gauge1 := prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "test_gauge",
				Help: "test help",
			},
			[]string{"label"},
		)

		result1, err := registerGaugeVec(reg, gauge1)
		require.NoError(t, err)

		gauge2 := prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "test_gauge",
				Help: "test help",
			},
			[]string{"label"},
		)

		result2, err := registerGaugeVec(reg, gauge2)
		require.NoError(t, err)
		assert.Same(t, result1, result2)
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

		_, err := registerCounterVec(reg, counter)
		require.NoError(t, err)

		gauge := prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "test_counter",
				Help: "test help",
			},
			[]string{"label"},
		)

		_, err = registerGaugeVec(reg, gauge)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected type")
	})
}

func TestRegisterCounterVec(t *testing.T) {
	t.Run("registers new counter vec", func(t *testing.T) {
		reg := prometheus.NewRegistry()
		counter := prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "test_counter",
				Help: "test help",
			},
			[]string{"label"},
		)

		result, err := registerCounterVec(reg, counter)
		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("returns existing counter vec", func(t *testing.T) {
		reg := prometheus.NewRegistry()
		counter1 := prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "test_counter",
				Help: "test help",
			},
			[]string{"label"},
		)

		result1, err := registerCounterVec(reg, counter1)
		require.NoError(t, err)

		counter2 := prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "test_counter",
				Help: "test help",
			},
			[]string{"label"},
		)

		result2, err := registerCounterVec(reg, counter2)
		require.NoError(t, err)
		assert.Same(t, result1, result2)
	})

	t.Run("returns error for wrong type", func(t *testing.T) {
		reg := prometheus.NewRegistry()
		gauge := prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "test_gauge",
				Help: "test help",
			},
			[]string{"label"},
		)

		_, err := registerGaugeVec(reg, gauge)
		require.NoError(t, err)

		counter := prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "test_gauge",
				Help: "test help",
			},
			[]string{"label"},
		)

		_, err = registerCounterVec(reg, counter)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected type")
	})
}

func TestRegisterGauge(t *testing.T) {
	t.Run("registers new gauge", func(t *testing.T) {
		reg := prometheus.NewRegistry()
		gauge := prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "test_gauge",
				Help: "test help",
			},
		)

		result, err := registerGauge(reg, gauge)
		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("returns existing gauge", func(t *testing.T) {
		reg := prometheus.NewRegistry()
		gauge1 := prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "test_gauge",
				Help: "test help",
			},
		)

		result1, err := registerGauge(reg, gauge1)
		require.NoError(t, err)

		gauge2 := prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "test_gauge",
				Help: "test help",
			},
		)

		result2, err := registerGauge(reg, gauge2)
		require.NoError(t, err)
		assert.Same(t, result1, result2)
	})

	t.Run("returns error for wrong type", func(t *testing.T) {
		// Counter cannot be registered as gauge - this will fail at compile time
		// Skip this test since there's no registerCounter function
		t.Skip("registerCounter function does not exist")
	})
}

func TestAsAlreadyRegistered(t *testing.T) {
	t.Run("returns true for AlreadyRegisteredError", func(t *testing.T) {
		reg := prometheus.NewRegistry()
		gauge := prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "test_gauge",
				Help: "test help",
			},
		)

		// Register once
		_, err := registerGauge(reg, gauge)
		require.NoError(t, err)

		// Try to register again
		err = reg.Register(gauge)
		require.Error(t, err)

		var already prometheus.AlreadyRegisteredError
		result := asAlreadyRegistered(err, &already)
		assert.True(t, result)
		assert.NotNil(t, already.ExistingCollector)
	})

	t.Run("returns false for other errors", func(t *testing.T) {
		var already prometheus.AlreadyRegisteredError
		result := asAlreadyRegistered(assert.AnError, &already)
		assert.False(t, result)
		assert.Nil(t, already.ExistingCollector)
	})

	t.Run("returns false for nil error", func(t *testing.T) {
		var already prometheus.AlreadyRegisteredError
		result := asAlreadyRegistered(nil, &already)
		assert.False(t, result)
	})
}

func TestInboundStat(t *testing.T) {
	t.Run("JSON marshaling", func(t *testing.T) {
		stat := InboundStat{
			Tag:         "vless-in",
			Type:        "vless",
			Users:       5,
			Connections: 10,
			UpBytes:     1000,
			DownBytes:   2000,
		}

		data, err := json.Marshal(stat)
		require.NoError(t, err)

		var parsed InboundStat
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)
		assert.Equal(t, stat, parsed)
	})

	t.Run("JSON with zero values", func(t *testing.T) {
		stat := InboundStat{}

		data, err := json.Marshal(stat)
		require.NoError(t, err)

		var parsed InboundStat
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)
		assert.Equal(t, stat, parsed)
	})
}

func TestCollector_Concurrency(t *testing.T) {
	reg := prometheus.NewRegistry()
	c, err := NewCollector(reg)
	require.NoError(t, err)

	t.Run("concurrent ObserveInbounds calls", func(t *testing.T) {
		done := make(chan bool)
		for i := 0; i < 10; i++ {
			go func(id int) {
				stats := []InboundStat{
					{
						Tag:         "in-" + string(rune('a'+id)),
						Type:        "vless",
						Users:       id,
						Connections: id * 2,
						UpBytes:     uint64(id * 100),
						DownBytes:   uint64(id * 200),
					},
				}
				c.ObserveInbounds(stats)
				done <- true
			}(i)
		}

		// Wait for all goroutines
		for i := 0; i < 10; i++ {
			<-done
		}

		// Should not panic and metrics should be consistent
		metrics, err := reg.Gather()
		require.NoError(t, err)
		assert.NotEmpty(t, metrics)
	})
}

func TestCollector_ExportsRequiredMetrics(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector, err := NewCollector(registry)
	require.NoError(t, err)

	collector.SetAgentInfo("1.2.3", "1.12.21")
	collector.SetSyncStatus("synced", time.Unix(1735689600, 0))
	collector.ObserveInbounds([]InboundStat{{
		Tag:         "vless-reality",
		Type:        "vless",
		Users:       7,
		Connections: 3,
		UpBytes:     100,
		DownBytes:   200,
	}})

	collector.ObserveInbounds([]InboundStat{{
		Tag:         "vless-reality",
		Type:        "vless",
		Users:       9,
		Connections: 4,
		UpBytes:     250,
		DownBytes:   400,
	}})

	metricFamilies, err := registry.Gather()
	require.NoError(t, err)

	names := map[string]bool{}
	for _, family := range metricFamilies {
		names[family.GetName()] = true
	}

	required := []string{
		"singbox_agent_info",
		"singbox_agent_sync_status",
		"singbox_agent_sync_timestamp_seconds",
		"singbox_inbound_users",
		"singbox_inbound_connections",
		"singbox_inbound_traffic_up_bytes_total",
		"singbox_inbound_traffic_down_bytes_total",
	}

	for _, metricName := range required {
		assert.True(t, names[metricName], metricName)
	}
}

func TestCollector_AvoidsSensitiveLabelKeys(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector, err := NewCollector(registry)
	require.NoError(t, err)

	collector.SetAgentInfo("1.0.0", "1.12.0")
	collector.SetSyncStatus("synced", time.Now())
	collector.ObserveInbounds([]InboundStat{{Tag: "tag-a", Type: "vless", Users: 1, Connections: 1, UpBytes: 1, DownBytes: 1}})

	metricFamilies, err := registry.Gather()
	require.NoError(t, err)

	for _, family := range metricFamilies {
		for _, metric := range family.GetMetric() {
			for _, label := range metric.GetLabel() {
				assert.NotEqual(t, "token", label.GetName())
				assert.NotEqual(t, "secret", label.GetName())
				assert.NotEqual(t, "password", label.GetName())
			}
		}
	}
}
