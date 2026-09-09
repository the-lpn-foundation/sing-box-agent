package singbox

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	v2rayapi "github.com/the-lpn-foundation/sing-box-agent/internal/v2rayapi"
)

// statsServiceFullMethod is the QueryStats method path of the sing-box
// v2ray_api service. sing-box registers that server under the v2ray
// canonical service name, not under its own Go package name.
const statsServiceFullMethod = "/v2ray.core.app.stats.command.StatsService/QueryStats"

// V2RayStatsClient queries per-inbound traffic counters from the sing-box
// experimental v2ray_api stats service (grpc StatsService).
type V2RayStatsClient struct {
	conn *grpc.ClientConn
}

// NewV2RayStatsClient dials the sing-box v2ray_api listener.
func NewV2RayStatsClient(address string) (*V2RayStatsClient, error) {
	conn, err := grpc.NewClient(address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.ForceCodec(v2rayapi.Codec{})),
	)
	if err != nil {
		return nil, fmt.Errorf("dial v2ray stats api %s: %w", address, err)
	}
	return &V2RayStatsClient{conn: conn}, nil
}

// Close closes the underlying grpc connection.
func (c *V2RayStatsClient) Close() error {
	return c.conn.Close()
}

// queryStats performs a single QueryStats RPC and returns the first stat value.
func (c *V2RayStatsClient) queryStats(ctx context.Context, pattern string) (int64, error) {
	req := &v2rayapi.QueryStatsRequest{Patterns: []string{pattern}}
	resp := &v2rayapi.QueryStatsResponse{}
	if err := c.conn.Invoke(ctx, statsServiceFullMethod, req, resp, grpc.StaticMethod()); err != nil {
		return 0, err
	}
	if stats := resp.GetStat(); len(stats) > 0 {
		return stats[0].GetValue(), nil
	}
	return 0, nil
}

// InboundTraffic returns cumulative uplink and downlink bytes for an inbound tag.
func (c *V2RayStatsClient) InboundTraffic(ctx context.Context, tag string) (up, down uint64, err error) {
	upVal, err := c.queryStats(ctx, "inbound>>>"+tag+">>>traffic>>>uplink")
	if err != nil {
		return 0, 0, fmt.Errorf("query uplink for %s: %w", tag, err)
	}

	downVal, err := c.queryStats(ctx, "inbound>>>"+tag+">>>traffic>>>downlink")
	if err != nil {
		return 0, 0, fmt.Errorf("query downlink for %s: %w", tag, err)
	}

	if upVal > 0 {
		up = uint64(upVal)
	}
	if downVal > 0 {
		down = uint64(downVal)
	}
	return up, down, nil
}

// UserTrafficBatch returns cumulative uplink/downlink bytes for all given user
// names (sing-box keys per-user counters by the inbound user's name, which the
// agent sets to the subId). A single QueryStats RPC carries all patterns.
func (c *V2RayStatsClient) UserTrafficBatch(ctx context.Context, names []string) (map[string][2]uint64, error) {
	if len(names) == 0 {
		return map[string][2]uint64{}, nil
	}

	patterns := make([]string, 0, len(names)*2)
	for _, name := range names {
		patterns = append(patterns, "user>>>"+name+">>>traffic>>>uplink")
		patterns = append(patterns, "user>>>"+name+">>>traffic>>>downlink")
	}

	req := &v2rayapi.QueryStatsRequest{Patterns: patterns}
	resp := &v2rayapi.QueryStatsResponse{}
	if err := c.conn.Invoke(ctx, statsServiceFullMethod, req, resp, grpc.StaticMethod()); err != nil {
		return nil, err
	}

	result := make(map[string][2]uint64, len(names))
	for _, stat := range resp.GetStat() {
		name := stat.GetName()
		if !strings.HasPrefix(name, "user>>>") || !strings.Contains(name, ">>>traffic>>>") {
			continue
		}
		userName := strings.TrimPrefix(name, "user>>>")
		userName = strings.TrimSuffix(userName, ">>>traffic>>>uplink")
		userName = strings.TrimSuffix(userName, ">>>traffic>>>downlink")
		entry := result[userName]
		switch {
		case strings.HasSuffix(name, ">>>uplink"):
			entry[0] = uint64(stat.GetValue())
		case strings.HasSuffix(name, ">>>downlink"):
			entry[1] = uint64(stat.GetValue())
		}
		result[userName] = entry
	}
	return result, nil
}
