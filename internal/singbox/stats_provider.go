package singbox

import (
	"context"
	"time"

	"github.com/oglenyaboss/sing-box-agent/internal/models"
)

// StatsProviderAdapter bridges sing-box data to the handlers.StatsProvider interface.
// When a V2RayStatsClient is attached, traffic stats are real counters from the
// sing-box experimental v2ray_api; otherwise placeholder zero stats are returned.
type StatsProviderAdapter struct {
	configClient *ConfigClient
	statsClient  *V2RayStatsClient
}

// NewStatsProviderAdapter creates a new StatsProviderAdapter.
func NewStatsProviderAdapter(configClient *ConfigClient) *StatsProviderAdapter {
	return &StatsProviderAdapter{configClient: configClient}
}

// WithStatsClient attaches a v2ray_api client so GetTrafficStats returns real counters.
func (a *StatsProviderAdapter) WithStatsClient(client *V2RayStatsClient) *StatsProviderAdapter {
	a.statsClient = client
	return a
}

// GetTrafficStats returns traffic statistics per inbound.
func (a *StatsProviderAdapter) GetTrafficStats(ctx context.Context, inbound string, start, end *time.Time) ([]models.TrafficInboundStat, error) {
	inbounds, err := a.configClient.GetInbounds(ctx)
	if err != nil {
		return nil, err
	}

	var stats []models.TrafficInboundStat
	for _, ib := range inbounds {
		if inbound != "" && ib.Tag != inbound {
			continue
		}

		stat := models.TrafficInboundStat{
			Tag:       ib.Tag,
			Type:      ib.Type,
			UpBytes:   0,
			DownBytes: 0,
		}

		if a.statsClient != nil {
			up, down, err := a.statsClient.InboundTraffic(ctx, ib.Tag)
			if err != nil {
				return nil, err
			}
			stat.UpBytes = up
			stat.DownBytes = down
		}

		stats = append(stats, stat)
	}

	return stats, nil
}

// GetOnlineUsers returns currently connected users.
// Since sing-box doesn't expose connection tracking via Go API,
// this returns an empty list. Real implementation requires
// sing-box experimental clash API or similar.
func (a *StatsProviderAdapter) GetOnlineUsers(ctx context.Context) ([]models.OnlineUser, error) {
	return []models.OnlineUser{}, nil
}
