package metrics

import (
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type InboundStat struct {
	Tag         string
	Type        string
	Users       int
	Connections int
	UpBytes     uint64
	DownBytes   uint64
}

type Collector struct {
	agentInfo          *prometheus.GaugeVec
	syncStatus         *prometheus.GaugeVec
	syncTimestamp      prometheus.Gauge
	inboundUsers       *prometheus.GaugeVec
	inboundConnections *prometheus.GaugeVec
	trafficUp          *prometheus.CounterVec
	trafficDown        *prometheus.CounterVec

	mu       sync.Mutex
	lastUp   map[string]uint64
	lastDown map[string]uint64
}

func NewCollector(registerer prometheus.Registerer) (*Collector, error) {
	if registerer == nil {
		registerer = prometheus.DefaultRegisterer
	}

	agentInfo, err := registerGaugeVec(registerer, prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "singbox_agent_info",
			Help: "Static agent build information",
		},
		[]string{"version", "singbox_version"},
	))
	if err != nil {
		return nil, err
	}

	syncStatus, err := registerGaugeVec(registerer, prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "singbox_agent_sync_status",
			Help: "Current sync status (0=synced, 1=syncing, 2=error)",
		},
		[]string{"state"},
	))
	if err != nil {
		return nil, err
	}

	syncTimestamp, err := registerGauge(registerer, prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "singbox_agent_sync_timestamp_seconds",
			Help: "Unix timestamp of the last successful sync",
		},
	))
	if err != nil {
		return nil, err
	}

	inboundUsers, err := registerGaugeVec(registerer, prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "singbox_inbound_users",
			Help: "Online users per inbound",
		},
		[]string{"tag", "type"},
	))
	if err != nil {
		return nil, err
	}

	inboundConnections, err := registerGaugeVec(registerer, prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "singbox_inbound_connections",
			Help: "Open connections per inbound",
		},
		[]string{"tag"},
	))
	if err != nil {
		return nil, err
	}

	trafficUp, err := registerCounterVec(registerer, prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "singbox_inbound_traffic_up_bytes_total",
			Help: "Observed upload traffic by inbound",
		},
		[]string{"tag"},
	))
	if err != nil {
		return nil, err
	}

	trafficDown, err := registerCounterVec(registerer, prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "singbox_inbound_traffic_down_bytes_total",
			Help: "Observed download traffic by inbound",
		},
		[]string{"tag"},
	))
	if err != nil {
		return nil, err
	}

	return &Collector{
		agentInfo:          agentInfo,
		syncStatus:         syncStatus,
		syncTimestamp:      syncTimestamp,
		inboundUsers:       inboundUsers,
		inboundConnections: inboundConnections,
		trafficUp:          trafficUp,
		trafficDown:        trafficDown,
		lastUp:             make(map[string]uint64),
		lastDown:           make(map[string]uint64),
	}, nil
}

func (c *Collector) SetAgentInfo(version, singBoxVersion string) {
	c.agentInfo.Reset()
	c.agentInfo.WithLabelValues(version, singBoxVersion).Set(1)
}

func (c *Collector) SetSyncStatus(state string, lastSync time.Time) {
	c.syncStatus.Reset()
	c.syncStatus.WithLabelValues(state).Set(1)
	if !lastSync.IsZero() {
		c.syncTimestamp.Set(float64(lastSync.Unix()))
	}
}

func (c *Collector) ObserveInbounds(stats []InboundStat) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.inboundUsers.Reset()
	c.inboundConnections.Reset()

	for _, stat := range stats {
		if stat.Tag == "" {
			continue
		}

		c.inboundUsers.WithLabelValues(stat.Tag, stat.Type).Set(float64(stat.Users))
		c.inboundConnections.WithLabelValues(stat.Tag).Set(float64(stat.Connections))

		lastUp := c.lastUp[stat.Tag]
		upDelta := deltaCounter(lastUp, stat.UpBytes)
		if upDelta > 0 {
			c.trafficUp.WithLabelValues(stat.Tag).Add(float64(upDelta))
		}
		c.lastUp[stat.Tag] = stat.UpBytes

		lastDown := c.lastDown[stat.Tag]
		downDelta := deltaCounter(lastDown, stat.DownBytes)
		if downDelta > 0 {
			c.trafficDown.WithLabelValues(stat.Tag).Add(float64(downDelta))
		}
		c.lastDown[stat.Tag] = stat.DownBytes
	}
}

func deltaCounter(previous, current uint64) uint64 {
	if current >= previous {
		return current - previous
	}
	return current
}

func registerGaugeVec(registerer prometheus.Registerer, collector *prometheus.GaugeVec) (*prometheus.GaugeVec, error) {
	if err := registerer.Register(collector); err != nil {
		var already prometheus.AlreadyRegisteredError
		if ok := asAlreadyRegistered(err, &already); ok {
			existing, ok := already.ExistingCollector.(*prometheus.GaugeVec)
			if ok {
				return existing, nil
			}
			return nil, fmt.Errorf("existing collector has unexpected type")
		}
		return nil, err
	}
	return collector, nil
}

func registerCounterVec(registerer prometheus.Registerer, collector *prometheus.CounterVec) (*prometheus.CounterVec, error) {
	if err := registerer.Register(collector); err != nil {
		var already prometheus.AlreadyRegisteredError
		if ok := asAlreadyRegistered(err, &already); ok {
			existing, ok := already.ExistingCollector.(*prometheus.CounterVec)
			if ok {
				return existing, nil
			}
			return nil, fmt.Errorf("existing collector has unexpected type")
		}
		return nil, err
	}
	return collector, nil
}

func registerGauge(registerer prometheus.Registerer, collector prometheus.Gauge) (prometheus.Gauge, error) {
	if err := registerer.Register(collector); err != nil {
		var already prometheus.AlreadyRegisteredError
		if ok := asAlreadyRegistered(err, &already); ok {
			existing, ok := already.ExistingCollector.(prometheus.Gauge)
			if ok {
				return existing, nil
			}
			return nil, fmt.Errorf("existing collector has unexpected type")
		}
		return nil, err
	}
	return collector, nil
}

func asAlreadyRegistered(err error, target *prometheus.AlreadyRegisteredError) bool {
	value, ok := err.(prometheus.AlreadyRegisteredError)
	if !ok {
		return false
	}
	*target = value
	return true
}
