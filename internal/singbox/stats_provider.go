package singbox

import (
	"context"
	"time"

	"github.com/the-lpn-foundation/sing-box-agent/internal/models"
)

// StatsProviderAdapter bridges sing-box data to the handlers.StatsProvider interface.
// When a V2RayStatsClient is attached, traffic stats are real counters from the
// sing-box experimental v2ray_api; otherwise placeholder zero stats are returned.
type StatsProviderAdapter struct {
	configClient *ConfigClient
	statsClient  trafficQuerier
}

// trafficQuerier abstracts the v2ray_api stats queries so tests can inject a
// fake client and so unavailable stats can degrade to zero counters instead
// of failing the whole endpoint.
type trafficQuerier interface {
	InboundTraffic(ctx context.Context, tag string) (up, down uint64, err error)
	UserTrafficBatch(ctx context.Context, names []string) (map[string][2]uint64, error)
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
				// v2ray_api unavailable for this inbound: degrade to zero counters
				// (project philosophy) instead of failing the whole endpoint.
				up, down = 0, 0
			}
			stat.UpBytes = up
			stat.DownBytes = down
		}

		stats = append(stats, stat)
	}

	return stats, nil
}

// GetUserTrafficStats returns cumulative traffic per user (keyed by subId).
// User names in the sing-box config equal the subId (see buildProtocolUser),
// and sing-box v2ray_api tracks per-user counters only for names listed in
// experimental.v2ray_api.stats.users (kept in sync by ConfigClient).
func (a *StatsProviderAdapter) GetUserTrafficStats(ctx context.Context) ([]models.TrafficUserStat, error) {
	users, err := a.configClient.GetUsers(ctx)
	if err != nil {
		return nil, err
	}
	if a.statsClient == nil {
		return []models.TrafficUserStat{}, nil
	}

	names := make([]string, 0, len(users))
	for _, u := range users {
		if u.SubID != "" {
			names = append(names, u.SubID)
		}
	}

	// If the stats client is unavailable, degrade to zero counters for all users
	// (project philosophy) instead of failing the endpoint.
	counters, batchErr := a.statsClient.UserTrafficBatch(ctx, names)
	if batchErr != nil {
		counters = nil
	}

	result := make([]models.TrafficUserStat, 0, len(users))
	for _, u := range users {
		if u.SubID == "" {
			continue
		}
		entry, ok := counters[u.SubID]
		if !ok {
			// No counter yet — sing-box creates per-user counters lazily on
			// first connection; report zeroes so consumers can compute deltas.
			entry = [2]uint64{0, 0}
		}
		result = append(result, models.TrafficUserStat{
			SubID:     u.SubID,
			Inbound:   u.InboundTag,
			UpBytes:   entry[0],
			DownBytes: entry[1],
		})
	}
	return result, nil
}

// GetOnlineUsers returns currently connected users.
// Since sing-box doesn't expose connection tracking via Go API,
// this returns an empty list. Real implementation requires
// sing-box experimental clash API or similar.
func (a *StatsProviderAdapter) GetOnlineUsers(ctx context.Context) ([]models.OnlineUser, error) {
	return []models.OnlineUser{}, nil
}
