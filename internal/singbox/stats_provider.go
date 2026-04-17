package singbox

import (
	"context"
	"time"

	"github.com/lenya/sing-box-agent/internal/models"
)

// StatsProviderAdapter bridges sing-box data to the handlers.StatsProvider interface.
// Since sing-box doesn't expose per-user traffic stats via its Go API,
// this adapter reads from the config to enumerate inbounds and returns
// placeholder stats. Real traffic stats require sing-box experimental API integration.
type StatsProviderAdapter struct {
	configClient *ConfigClient
}

// NewStatsProviderAdapter creates a new StatsProviderAdapter.
func NewStatsProviderAdapter(configClient *ConfigClient) *StatsProviderAdapter {
	return &StatsProviderAdapter{configClient: configClient}
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
		stats = append(stats, models.TrafficInboundStat{
			Tag:       ib.Tag,
			Type:      ib.Type,
			UpBytes:   0,
			DownBytes: 0,
		})
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
