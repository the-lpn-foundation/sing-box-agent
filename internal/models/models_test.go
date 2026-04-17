//nolint:errcheck
package models

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUser(t *testing.T) {
	t.Run("JSON marshaling with all fields", func(t *testing.T) {
		resetTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		user := User{
			SubID:         "test-sub-123",
			UUID:          "550e8400-e29b-41d4-a716-446655440000",
			InboundTag:    "vless-in",
			Email:         "user@example.com",
			Enabled:       true,
			Flow:          "xtls-rprx-vision",
			LimitIP:       5,
			UploadLimit:   10737418240, // 10GB
			DownloadLimit: 10737418240, // 10GB
			ResetAt:       &resetTime,
		}

		data, err := json.Marshal(user)
		require.NoError(t, err)

		var parsed User
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)
		assert.Equal(t, user, parsed)
	})

	t.Run("JSON marshaling with required fields only", func(t *testing.T) {
		user := User{
			SubID:      "test-sub-123",
			InboundTag: "vless-in",
			Enabled:    true,
		}

		data, err := json.Marshal(user)
		require.NoError(t, err)

		var parsed User
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)
		assert.Equal(t, user, parsed)
	})

	t.Run("JSON marshaling with nil ResetAt", func(t *testing.T) {
		user := User{
			SubID:      "test-sub-123",
			InboundTag: "vless-in",
			Enabled:    true,
			ResetAt:    nil,
		}

		data, err := json.Marshal(user)
		require.NoError(t, err)

		var parsed User
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)
		assert.Equal(t, user, parsed)
		assert.Nil(t, parsed.ResetAt)
	})

	t.Run("JSON unmarshaling with omitempty fields", func(t *testing.T) {
		jsonData := `{
			"subId": "test-sub-123",
			"inboundTag": "vless-in",
			"enabled": true
		}`

		var user User
		err := json.Unmarshal([]byte(jsonData), &user)
		require.NoError(t, err)
		assert.Equal(t, "test-sub-123", user.SubID)
		assert.Equal(t, "vless-in", user.InboundTag)
		assert.True(t, user.Enabled)
		assert.Empty(t, user.UUID)
		assert.Empty(t, user.Email)
		assert.Empty(t, user.Flow)
		assert.Equal(t, 0, user.LimitIP)
		assert.Equal(t, uint64(0), user.UploadLimit)
		assert.Equal(t, uint64(0), user.DownloadLimit)
		assert.Nil(t, user.ResetAt)
	})

	t.Run("JSON unmarshaling with all fields", func(t *testing.T) {
		jsonData := `{
			"subId": "test-sub-123",
			"uuid": "550e8400-e29b-41d4-a716-446655440000",
			"inboundTag": "vless-in",
			"email": "user@example.com",
			"enabled": true,
			"flow": "xtls-rprx-vision",
			"limitIp": 5,
			"uploadLimit": 10737418240,
			"downloadLimit": 10737418240,
			"resetAt": "2024-01-01T00:00:00Z"
		}`

		var user User
		err := json.Unmarshal([]byte(jsonData), &user)
		require.NoError(t, err)
		assert.Equal(t, "test-sub-123", user.SubID)
		assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", user.UUID)
		assert.Equal(t, "vless-in", user.InboundTag)
		assert.Equal(t, "user@example.com", user.Email)
		assert.True(t, user.Enabled)
		assert.Equal(t, "xtls-rprx-vision", user.Flow)
		assert.Equal(t, 5, user.LimitIP)
		assert.Equal(t, uint64(10737418240), user.UploadLimit)
		assert.Equal(t, uint64(10737418240), user.DownloadLimit)
		assert.NotNil(t, user.ResetAt)
	})

	t.Run("JSON unmarshaling with disabled user", func(t *testing.T) {
		jsonData := `{
			"subId": "test-sub-123",
			"inboundTag": "vless-in",
			"enabled": false
		}`

		var user User
		err := json.Unmarshal([]byte(jsonData), &user)
		require.NoError(t, err)
		assert.False(t, user.Enabled)
	})

	t.Run("JSON marshaling produces valid JSON", func(t *testing.T) {
		user := User{
			SubID:      "test-sub-123",
			InboundTag: "vless-in",
			Enabled:    true,
		}

		data, err := json.Marshal(user)
		require.NoError(t, err)

		var js map[string]interface{}
		err = json.Unmarshal(data, &js)
		require.NoError(t, err)
		assert.Equal(t, "test-sub-123", js["subId"])
		assert.Equal(t, "vless-in", js["inboundTag"])
		assert.Equal(t, true, js["enabled"])
	})

	t.Run("handles large limit values", func(t *testing.T) {
		user := User{
			SubID:         "test-sub-123",
			InboundTag:    "vless-in",
			Enabled:       true,
			UploadLimit:   18446744073709551615, // max uint64
			DownloadLimit: 18446744073709551615, // max uint64
		}

		data, err := json.Marshal(user)
		require.NoError(t, err)

		var parsed User
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)
		assert.Equal(t, user, parsed)
	})
}

func TestInbound(t *testing.T) {
	t.Run("JSON marshaling with all fields", func(t *testing.T) {
		inbound := Inbound{
			Tag:    "vless-in",
			Type:   "vless",
			Listen: "0.0.0.0",
			Port:   443,
			Options: map[string]interface{}{
				"tls":         true,
				"reality":     true,
				"serverNames": []string{"example.com"},
			},
		}

		data, err := json.Marshal(inbound)
		require.NoError(t, err)

		var parsed Inbound
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)
		assert.Equal(t, inbound.Tag, parsed.Tag)
		assert.Equal(t, inbound.Type, parsed.Type)
		assert.Equal(t, inbound.Listen, parsed.Listen)
		assert.Equal(t, inbound.Port, parsed.Port)
		assert.Equal(t, inbound.Options["tls"], parsed.Options["tls"])
	})

	t.Run("JSON marshaling with required fields only", func(t *testing.T) {
		inbound := Inbound{
			Tag:  "vless-in",
			Type: "vless",
		}

		data, err := json.Marshal(inbound)
		require.NoError(t, err)

		var parsed Inbound
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)
		assert.Equal(t, inbound, parsed)
	})

	t.Run("JSON marshaling with nil options", func(t *testing.T) {
		inbound := Inbound{
			Tag:     "vless-in",
			Type:    "vless",
			Options: nil,
		}

		data, err := json.Marshal(inbound)
		require.NoError(t, err)

		var parsed Inbound
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)
		assert.Equal(t, inbound, parsed)
	})

	t.Run("JSON unmarshaling with omitempty fields", func(t *testing.T) {
		jsonData := `{
			"tag": "vless-in",
			"type": "vless"
		}`

		var inbound Inbound
		err := json.Unmarshal([]byte(jsonData), &inbound)
		require.NoError(t, err)
		assert.Equal(t, "vless-in", inbound.Tag)
		assert.Equal(t, "vless", inbound.Type)
		assert.Empty(t, inbound.Listen)
		assert.Equal(t, 0, inbound.Port)
		assert.Nil(t, inbound.Options)
	})

	t.Run("JSON unmarshaling with all fields", func(t *testing.T) {
		jsonData := `{
			"tag": "vless-in",
			"type": "vless",
			"listen": "0.0.0.0",
			"port": 443,
			"options": {
				"tls": true,
				"reality": true
			}
		}`

		var inbound Inbound
		err := json.Unmarshal([]byte(jsonData), &inbound)
		require.NoError(t, err)
		assert.Equal(t, "vless-in", inbound.Tag)
		assert.Equal(t, "vless", inbound.Type)
		assert.Equal(t, "0.0.0.0", inbound.Listen)
		assert.Equal(t, 443, inbound.Port)
		assert.NotNil(t, inbound.Options)
		assert.True(t, inbound.Options["tls"].(bool))
	})

	t.Run("JSON marshaling produces valid JSON", func(t *testing.T) {
		inbound := Inbound{
			Tag:  "vless-in",
			Type: "vless",
		}

		data, err := json.Marshal(inbound)
		require.NoError(t, err)

		var js map[string]interface{}
		err = json.Unmarshal(data, &js)
		require.NoError(t, err)
		assert.Equal(t, "vless-in", js["tag"])
		assert.Equal(t, "vless", js["type"])
	})

	t.Run("handles different inbound types", func(t *testing.T) {
		types := []string{"vless", "vmess", "trojan", "hysteria2", "shadowsocks"}

		for _, inboundType := range types {
			inbound := Inbound{
				Tag:  inboundType + "-in",
				Type: inboundType,
			}

			data, err := json.Marshal(inbound)
			require.NoError(t, err)

			var parsed Inbound
			err = json.Unmarshal(data, &parsed)
			require.NoError(t, err)
			assert.Equal(t, inbound, parsed)
		}
	})

	t.Run("handles complex options", func(t *testing.T) {
		inbound := Inbound{
			Tag:  "vless-in",
			Type: "vless",
			Options: map[string]interface{}{
				"tls": map[string]interface{}{
					"enabled":         true,
					"serverName":      "example.com",
					"certificatePath": "/path/to/cert.pem",
				},
				"reality": map[string]interface{}{
					"enabled": true,
					"dest":    "www.google.com",
				},
			},
		}

		data, err := json.Marshal(inbound)
		require.NoError(t, err)

		var parsed Inbound
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)
		assert.NotNil(t, parsed.Options)
	})
}

func TestDesiredState(t *testing.T) {
	t.Run("JSON marshaling with all fields", func(t *testing.T) {
		resetTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		timestamp := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)

		state := DesiredState{
			Version: 1,
			Inbounds: []Inbound{
				{
					Tag:  "vless-in",
					Type: "vless",
				},
			},
			Users: []User{
				{
					SubID:      "user-1",
					InboundTag: "vless-in",
					Enabled:    true,
					ResetAt:    &resetTime,
				},
			},
			Timestamp: timestamp,
		}

		data, err := json.Marshal(state)
		require.NoError(t, err)

		var parsed DesiredState
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)
		assert.Equal(t, state.Version, parsed.Version)
		assert.Len(t, parsed.Inbounds, 1)
		assert.Len(t, parsed.Users, 1)
		assert.True(t, parsed.Timestamp.Equal(timestamp))
	})

	t.Run("JSON marshaling with required fields only", func(t *testing.T) {
		state := DesiredState{
			Version:   1,
			Timestamp: time.Now(),
		}

		data, err := json.Marshal(state)
		require.NoError(t, err)

		var parsed DesiredState
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)
		assert.Equal(t, state.Version, parsed.Version)
		assert.Nil(t, parsed.Inbounds)
		assert.Nil(t, parsed.Users)
	})

	t.Run("JSON unmarshaling with omitempty fields", func(t *testing.T) {
		jsonData := `{
			"version": 1,
			"timestamp": "2024-01-15T12:00:00Z"
		}`

		var state DesiredState
		err := json.Unmarshal([]byte(jsonData), &state)
		require.NoError(t, err)
		assert.Equal(t, 1, state.Version)
		assert.Nil(t, state.Inbounds)
		assert.Nil(t, state.Users)
	})

	t.Run("JSON unmarshaling with all fields", func(t *testing.T) {
		jsonData := `{
			"version": 1,
			"inbounds": [
				{
					"tag": "vless-in",
					"type": "vless"
				}
			],
			"users": [
				{
					"subId": "user-1",
					"inboundTag": "vless-in",
					"enabled": true
				}
			],
			"timestamp": "2024-01-15T12:00:00Z"
		}`

		var state DesiredState
		err := json.Unmarshal([]byte(jsonData), &state)
		require.NoError(t, err)
		assert.Equal(t, 1, state.Version)
		assert.Len(t, state.Inbounds, 1)
		assert.Len(t, state.Users, 1)
	})

	t.Run("JSON marshaling produces valid JSON", func(t *testing.T) {
		state := DesiredState{
			Version:   1,
			Timestamp: time.Now(),
		}

		data, err := json.Marshal(state)
		require.NoError(t, err)

		var js map[string]interface{}
		err = json.Unmarshal(data, &js)
		require.NoError(t, err)
		assert.Equal(t, float64(1), js["version"])
		assert.Contains(t, js, "timestamp")
	})

	t.Run("handles multiple inbounds and users", func(t *testing.T) {
		state := DesiredState{
			Version: 2,
			Inbounds: []Inbound{
				{Tag: "vless-in", Type: "vless"},
				{Tag: "hysteria2-in", Type: "hysteria2"},
			},
			Users: []User{
				{SubID: "user-1", InboundTag: "vless-in", Enabled: true},
				{SubID: "user-2", InboundTag: "vless-in", Enabled: true},
				{SubID: "user-3", InboundTag: "hysteria2-in", Enabled: true},
			},
			Timestamp: time.Now(),
		}

		data, err := json.Marshal(state)
		require.NoError(t, err)

		var parsed DesiredState
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)
		assert.Len(t, parsed.Inbounds, 2)
		assert.Len(t, parsed.Users, 3)
	})

	t.Run("handles version increment", func(t *testing.T) {
		state1 := DesiredState{
			Version:   1,
			Timestamp: time.Now(),
		}

		state2 := DesiredState{
			Version:   2,
			Timestamp: time.Now(),
		}

		assert.Greater(t, state2.Version, state1.Version)
	})
}

func TestSyncResult(t *testing.T) {
	t.Run("JSON marshaling with success", func(t *testing.T) {
		result := SyncResult{
			Success:        true,
			ChangesApplied: 5,
			Version:        1,
			Error:          "",
			Timestamp:      time.Now(),
		}

		data, err := json.Marshal(result)
		require.NoError(t, err)

		var parsed SyncResult
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)
		assert.Equal(t, result.Success, parsed.Success)
		assert.Equal(t, result.ChangesApplied, parsed.ChangesApplied)
		assert.Equal(t, result.Version, parsed.Version)
		assert.Empty(t, parsed.Error)
	})

	t.Run("JSON marshaling with error", func(t *testing.T) {
		result := SyncResult{
			Success:        false,
			ChangesApplied: 0,
			Version:        1,
			Error:          "failed to apply config",
			Timestamp:      time.Now(),
		}

		data, err := json.Marshal(result)
		require.NoError(t, err)

		var parsed SyncResult
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)
		assert.False(t, parsed.Success)
		assert.Equal(t, "failed to apply config", parsed.Error)
	})

	t.Run("JSON unmarshaling with all fields", func(t *testing.T) {
		jsonData := `{
			"success": true,
			"changesApplied": 5,
			"version": 1,
			"error": "",
			"timestamp": "2024-01-15T12:00:00Z"
		}`

		var result SyncResult
		err := json.Unmarshal([]byte(jsonData), &result)
		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Equal(t, 5, result.ChangesApplied)
		assert.Equal(t, 1, result.Version)
	})

	t.Run("JSON unmarshaling with error", func(t *testing.T) {
		jsonData := `{
			"success": false,
			"changesApplied": 0,
			"version": 1,
			"error": "connection failed",
			"timestamp": "2024-01-15T12:00:00Z"
		}`

		var result SyncResult
		err := json.Unmarshal([]byte(jsonData), &result)
		require.NoError(t, err)
		assert.False(t, result.Success)
		assert.Equal(t, "connection failed", result.Error)
	})

	t.Run("JSON marshaling produces valid JSON", func(t *testing.T) {
		result := SyncResult{
			Success:   true,
			Timestamp: time.Now(),
		}

		data, err := json.Marshal(result)
		require.NoError(t, err)

		var js map[string]interface{}
		err = json.Unmarshal(data, &js)
		require.NoError(t, err)
		assert.Equal(t, true, js["success"])
	})

	t.Run("handles zero changes applied", func(t *testing.T) {
		result := SyncResult{
			Success:        true,
			ChangesApplied: 0,
			Version:        1,
			Timestamp:      time.Now(),
		}

		data, err := json.Marshal(result)
		require.NoError(t, err)

		var parsed SyncResult
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)
		assert.Equal(t, 0, parsed.ChangesApplied)
	})

	t.Run("handles large number of changes", func(t *testing.T) {
		result := SyncResult{
			Success:        true,
			ChangesApplied: 1000,
			Version:        1,
			Timestamp:      time.Now(),
		}

		data, err := json.Marshal(result)
		require.NoError(t, err)

		var parsed SyncResult
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)
		assert.Equal(t, 1000, parsed.ChangesApplied)
	})
}

func TestTrafficStats(t *testing.T) {
	t.Run("JSON marshaling with all fields", func(t *testing.T) {
		resetTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		stats := TrafficStats{
			SubID:    "user-123",
			Upload:   10737418240, // 10GB
			Download: 21474836480, // 20GB
			ResetAt:  &resetTime,
		}

		data, err := json.Marshal(stats)
		require.NoError(t, err)

		var parsed TrafficStats
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)
		assert.Equal(t, stats.SubID, parsed.SubID)
		assert.Equal(t, stats.Upload, parsed.Upload)
		assert.Equal(t, stats.Download, parsed.Download)
		assert.True(t, parsed.ResetAt.Equal(resetTime))
	})

	t.Run("JSON marshaling with required fields only", func(t *testing.T) {
		stats := TrafficStats{
			SubID:    "user-123",
			Upload:   1000,
			Download: 2000,
		}

		data, err := json.Marshal(stats)
		require.NoError(t, err)

		var parsed TrafficStats
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)
		assert.Equal(t, stats, parsed)
	})

	t.Run("JSON marshaling with nil ResetAt", func(t *testing.T) {
		stats := TrafficStats{
			SubID:    "user-123",
			Upload:   1000,
			Download: 2000,
			ResetAt:  nil,
		}

		data, err := json.Marshal(stats)
		require.NoError(t, err)

		var parsed TrafficStats
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)
		assert.Nil(t, parsed.ResetAt)
	})

	t.Run("JSON unmarshaling with omitempty fields", func(t *testing.T) {
		jsonData := `{
			"subId": "user-123",
			"upload": 1000,
			"download": 2000
		}`

		var stats TrafficStats
		err := json.Unmarshal([]byte(jsonData), &stats)
		require.NoError(t, err)
		assert.Equal(t, "user-123", stats.SubID)
		assert.Equal(t, uint64(1000), stats.Upload)
		assert.Equal(t, uint64(2000), stats.Download)
		assert.Nil(t, stats.ResetAt)
	})

	t.Run("JSON unmarshaling with all fields", func(t *testing.T) {
		jsonData := `{
			"subId": "user-123",
			"upload": 10737418240,
			"download": 21474836480,
			"resetAt": "2024-01-01T00:00:00Z"
		}`

		var stats TrafficStats
		err := json.Unmarshal([]byte(jsonData), &stats)
		require.NoError(t, err)
		assert.Equal(t, "user-123", stats.SubID)
		assert.Equal(t, uint64(10737418240), stats.Upload)
		assert.Equal(t, uint64(21474836480), stats.Download)
		assert.NotNil(t, stats.ResetAt)
	})

	t.Run("JSON marshaling produces valid JSON", func(t *testing.T) {
		stats := TrafficStats{
			SubID:    "user-123",
			Upload:   1000,
			Download: 2000,
		}

		data, err := json.Marshal(stats)
		require.NoError(t, err)

		var js map[string]interface{}
		err = json.Unmarshal(data, &js)
		require.NoError(t, err)
		assert.Equal(t, "user-123", js["subId"])
		assert.Equal(t, float64(1000), js["upload"])
		assert.Equal(t, float64(2000), js["download"])
	})

	t.Run("handles zero traffic", func(t *testing.T) {
		stats := TrafficStats{
			SubID:    "user-123",
			Upload:   0,
			Download: 0,
		}

		data, err := json.Marshal(stats)
		require.NoError(t, err)

		var parsed TrafficStats
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)
		assert.Equal(t, uint64(0), parsed.Upload)
		assert.Equal(t, uint64(0), parsed.Download)
	})

	t.Run("handles large traffic values", func(t *testing.T) {
		stats := TrafficStats{
			SubID:    "user-123",
			Upload:   18446744073709551615, // max uint64
			Download: 18446744073709551615, // max uint64
		}

		data, err := json.Marshal(stats)
		require.NoError(t, err)

		var parsed TrafficStats
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)
		assert.Equal(t, uint64(18446744073709551615), parsed.Upload)
		assert.Equal(t, uint64(18446744073709551615), parsed.Download)
	})
}

func TestModelsIntegration(t *testing.T) {
	t.Run("round-trip DesiredState with Users and Inbounds", func(t *testing.T) {
		resetTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		timestamp := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)

		original := DesiredState{
			Version: 1,
			Inbounds: []Inbound{
				{
					Tag:     "vless-in",
					Type:    "vless",
					Listen:  "0.0.0.0",
					Port:    443,
					Options: map[string]interface{}{"tls": true},
				},
			},
			Users: []User{
				{
					SubID:         "user-1",
					UUID:          "550e8400-e29b-41d4-a716-446655440000",
					InboundTag:    "vless-in",
					Email:         "user@example.com",
					Enabled:       true,
					Flow:          "xtls-rprx-vision",
					LimitIP:       5,
					UploadLimit:   10737418240,
					DownloadLimit: 10737418240,
					ResetAt:       &resetTime,
				},
			},
			Timestamp: timestamp,
		}

		data, err := json.Marshal(original)
		require.NoError(t, err)

		var parsed DesiredState
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)
		assert.Equal(t, original.Version, parsed.Version)
		assert.Len(t, parsed.Inbounds, 1)
		assert.Len(t, parsed.Users, 1)
		assert.Equal(t, original.Users[0].SubID, parsed.Users[0].SubID)
		assert.Equal(t, original.Inbounds[0].Tag, parsed.Inbounds[0].Tag)
	})

	t.Run("round-trip SyncResult with error", func(t *testing.T) {
		original := SyncResult{
			Success:        false,
			ChangesApplied: 0,
			Version:        1,
			Error:          "failed to connect to sing-box",
			Timestamp:      time.Now(),
		}

		data, err := json.Marshal(original)
		require.NoError(t, err)

		var parsed SyncResult
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)
		assert.Equal(t, original.Success, parsed.Success)
		assert.Equal(t, original.Error, parsed.Error)
	})
}
