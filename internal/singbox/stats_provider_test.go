package singbox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failingTrafficQuerier simulates an unreachable v2ray_api stats service.
type failingTrafficQuerier struct {
	inboundErr error
	batchErr   error
}

func (f *failingTrafficQuerier) InboundTraffic(ctx context.Context, tag string) (uint64, uint64, error) {
	return 0, 0, f.inboundErr
}

func (f *failingTrafficQuerier) UserTrafficBatch(ctx context.Context, names []string) (map[string][2]uint64, error) {
	return nil, f.batchErr
}

// writeStatsTestConfig creates a temporary sing-box config with two inbounds
// (one carrying a user) and returns the config path.
func writeStatsTestConfig(t *testing.T) string {
	t.Helper()
	content := `{
		"inbounds": [
			{
				"type": "vless",
				"tag": "vless-in",
				"port": 443,
				"users": [
					{"name": "user1", "uuid": "uuid-1", "subId": "user1"}
				]
			},
			{
				"type": "vless",
				"tag": "vless-in-2",
				"port": 8443,
				"users": []
			}
		]
	}`
	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

// TestGetTrafficStats_StatsClientUnavailable verifies that a failing v2ray_api
// stats client degrades to zero counters instead of an error.
func TestGetTrafficStats_StatsClientUnavailable(t *testing.T) {
	adapter := NewStatsProviderAdapter(NewConfigClient(writeStatsTestConfig(t)))
	adapter.statsClient = &failingTrafficQuerier{inboundErr: errors.New("connection refused")}

	stats, err := adapter.GetTrafficStats(context.Background(), "", nil, nil)

	require.NoError(t, err)
	require.Len(t, stats, 2)
	for _, stat := range stats {
		assert.Equal(t, uint64(0), stat.UpBytes, "inbound %s must degrade to zeros", stat.Tag)
		assert.Equal(t, uint64(0), stat.DownBytes, "inbound %s must degrade to zeros", stat.Tag)
	}
	// Tags and types must still be reported for the consumers.
	assert.Equal(t, "vless-in", stats[0].Tag)
	assert.Equal(t, "vless-in-2", stats[1].Tag)
}

// TestGetUserTrafficStats_StatsClientUnavailable verifies that a failing
// v2ray_api stats client returns zero counters for all users instead of an error.
func TestGetUserTrafficStats_StatsClientUnavailable(t *testing.T) {
	adapter := NewStatsProviderAdapter(NewConfigClient(writeStatsTestConfig(t)))
	adapter.statsClient = &failingTrafficQuerier{batchErr: errors.New("connection refused")}

	stats, err := adapter.GetUserTrafficStats(context.Background())

	require.NoError(t, err)
	require.Len(t, stats, 1)
	assert.Equal(t, "user1", stats[0].SubID)
	assert.Equal(t, "vless-in", stats[0].Inbound)
	assert.Equal(t, uint64(0), stats[0].UpBytes)
	assert.Equal(t, uint64(0), stats[0].DownBytes)
}
