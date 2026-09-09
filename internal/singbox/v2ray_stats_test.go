package singbox

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/encoding"

	v2rayapi "github.com/oglenyaboss/sing-box-agent/internal/v2rayapi"
)

// The gRPC server resolves codecs by content subtype ("proto") in a global
// registry, so the hand-rolled wire-compatible codec must be registered for
// the mock server below to decode requests and encode responses.
func init() {
	encoding.RegisterCodec(v2rayapi.Codec{})
}

type statsServiceServer interface {
	QueryStats(context.Context, *v2rayapi.QueryStatsRequest) (*v2rayapi.QueryStatsResponse, error)
}

type mockStatsServer struct {
	values map[string]int64
}

func (m *mockStatsServer) QueryStats(_ context.Context, req *v2rayapi.QueryStatsRequest) (*v2rayapi.QueryStatsResponse, error) {
	patterns := req.GetPatterns()
	if len(patterns) == 0 && req.GetPattern() != "" {
		patterns = []string{req.GetPattern()}
	}
	resp := &v2rayapi.QueryStatsResponse{}
	for _, pattern := range patterns {
		if val, ok := m.values[pattern]; ok {
			resp.Stat = append(resp.Stat, &v2rayapi.Stat{Name: pattern, Value: val})
		}
	}
	return resp, nil
}

var statsServiceDesc = grpc.ServiceDesc{
	ServiceName: "v2ray.core.app.stats.command.StatsService",
	HandlerType: (*statsServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "QueryStats",
			Handler:    queryStatsHandler,
		},
	},
}

func queryStatsHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(v2rayapi.QueryStatsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(statsServiceServer).QueryStats(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/v2ray.core.app.stats.command.StatsService/QueryStats"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(statsServiceServer).QueryStats(ctx, req.(*v2rayapi.QueryStatsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func startMockStatsServer(t *testing.T, values map[string]int64) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	srv := grpc.NewServer()
	srv.RegisterService(&statsServiceDesc, &mockStatsServer{values: values})
	go func() {
		_ = srv.Serve(lis)
	}()
	t.Cleanup(srv.Stop)

	return lis.Addr().String()
}

func TestV2RayStatsClientInboundTraffic(t *testing.T) {
	addr := startMockStatsServer(t, map[string]int64{
		"inbound>>>vless-in>>>traffic>>>uplink":   12345,
		"inbound>>>vless-in>>>traffic>>>downlink": 67890,
	})

	client, err := NewV2RayStatsClient(addr)
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	up, down, err := client.InboundTraffic(context.Background(), "vless-in")
	require.NoError(t, err)
	require.Equal(t, uint64(12345), up)
	require.Equal(t, uint64(67890), down)
}

func TestV2RayStatsClientMissingInbound(t *testing.T) {
	addr := startMockStatsServer(t, map[string]int64{})

	client, err := NewV2RayStatsClient(addr)
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	// Unknown inbound: sing-box returns an empty stat list — must not error.
	up, down, err := client.InboundTraffic(context.Background(), "unknown-in")
	require.NoError(t, err)
	require.Equal(t, uint64(0), up)
	require.Equal(t, uint64(0), down)
}

func TestV2RayStatsClientUserTrafficBatch(t *testing.T) {
	addr := startMockStatsServer(t, map[string]int64{
		"user>>>user1>>>traffic>>>uplink":   111,
		"user>>>user1>>>traffic>>>downlink": 222,
		"user>>>user2>>>traffic>>>uplink":   333,
	})

	client, err := NewV2RayStatsClient(addr)
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	got, err := client.UserTrafficBatch(context.Background(), []string{"user1", "user2", "user3"})
	require.NoError(t, err)
	require.Equal(t, [2]uint64{111, 222}, got["user1"])
	require.Equal(t, [2]uint64{333, 0}, got["user2"])
	require.Equal(t, [2]uint64{0, 0}, got["user3"])
}
