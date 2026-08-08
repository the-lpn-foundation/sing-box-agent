package singbox

import (
	"context"
	"net"
	"testing"

	"github.com/sagernet/sing-box/experimental/v2rayapi"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type mockStatsServer struct {
	v2rayapi.UnimplementedStatsServiceServer
	values map[string]int64
}

func (m *mockStatsServer) QueryStats(_ context.Context, req *v2rayapi.QueryStatsRequest) (*v2rayapi.QueryStatsResponse, error) {
	pattern := ""
	if len(req.GetPatterns()) > 0 {
		pattern = req.GetPatterns()[0]
	} else {
		pattern = req.GetPattern()
	}
	val, ok := m.values[pattern]
	if !ok {
		return &v2rayapi.QueryStatsResponse{}, nil
	}
	return &v2rayapi.QueryStatsResponse{
		Stat: []*v2rayapi.Stat{{Name: req.GetPattern(), Value: val}},
	}, nil
}

func startMockStatsServer(t *testing.T, values map[string]int64) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	srv := grpc.NewServer()
	v2rayapi.RegisterStatsServiceServer(srv, &mockStatsServer{values: values})
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
