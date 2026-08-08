package metrics

import (
	"context"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/oglenyaboss/sing-box-agent/internal/singbox"
)

// StatsCollectionInterval is how often inbound traffic counters are refreshed.
const StatsCollectionInterval = 30 * time.Second

// RunStatsLoop registers the inbound collector and periodically polls the
// sing-box v2ray_api stats service. It returns after ctx is cancelled.
// A missing or unreachable stats API disables traffic metrics without
// failing the agent (all counters stay at zero).
func RunStatsLoop(
	ctx context.Context,
	logger *slog.Logger,
	statsAPIAddress string,
	configClient *singbox.ConfigClient,
) error {
	collector, err := NewCollector(prometheus.DefaultRegisterer)
	if err != nil {
		return err
	}

	statsClient, err := singbox.NewV2RayStatsClient(statsAPIAddress)
	if err != nil {
		logger.Warn("v2ray stats api unavailable, traffic metrics disabled",
			slog.String("error", err.Error()))
		return nil
	}
	defer func() { _ = statsClient.Close() }()

	collect := func() {
		inbounds, err := configClient.GetInbounds(ctx)
		if err != nil {
			logger.Warn("failed to list inbounds for stats collection",
				slog.String("error", err.Error()))
			return
		}

		stats := make([]InboundStat, 0, len(inbounds))
		for _, ib := range inbounds {
			up, down, err := statsClient.InboundTraffic(ctx, ib.Tag)
			if err != nil {
				logger.Debug("failed to query inbound traffic",
					slog.String("tag", ib.Tag), slog.String("error", err.Error()))
				continue
			}
			stats = append(stats, InboundStat{
				Tag:       ib.Tag,
				Type:      ib.Type,
				UpBytes:   up,
				DownBytes: down,
			})
		}

		collector.ObserveInbounds(stats)
	}

	// Collect immediately on start, then on a fixed interval.
	collect()
	ticker := time.NewTicker(StatsCollectionInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			collect()
		}
	}
}
