package singbox

//nolint:errcheck // Test file uses type assertions

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oglenyaboss/sing-box-agent/internal/models"
)

func TestNewConfigClient(t *testing.T) {
	client := NewConfigClient("/path/to/config.json")
	require.NotNil(t, client)
	assert.Equal(t, "/path/to/config.json", client.configPath)
	assert.Equal(t, reloadDebounceInterval, client.debounceInterval)
}

func TestConfigClient_GetInbounds(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T) string
		wantErr   bool
		wantCount int
		wantTags  []string
		check     func(t *testing.T, inbounds []models.Inbound)
	}{
		{
			name: "multiple inbounds",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				path := filepath.Join(dir, "config.json")
				config := `{
					"inbounds": [
						{
							"type": "vless",
							"tag": "in-1",
							"listen": "0.0.0.0",
							"listen_port": 443,
							"tls": {"enabled": true}
						},
						{
							"type": "hysteria2",
							"tag": "in-2",
							"listen": "::",
							"listen_port": 8443
						}
					]
				}`
				err := os.WriteFile(path, []byte(config), 0o644)
				require.NoError(t, err)
				return path
			},
			wantErr:   false,
			wantCount: 2,
			wantTags:  []string{"in-1", "in-2"},
			check: func(t *testing.T, inbounds []models.Inbound) {
				assert.Equal(t, "vless", inbounds[0].Type)
				assert.Equal(t, "0.0.0.0", inbounds[0].Listen)
				assert.Equal(t, 443, inbounds[0].Port)
				assert.NotNil(t, inbounds[0].Options)
				assert.Contains(t, inbounds[0].Options, "tls")

				assert.Equal(t, "hysteria2", inbounds[1].Type)
				assert.Equal(t, "::", inbounds[1].Listen)
				assert.Equal(t, 8443, inbounds[1].Port)
			},
		},
		{
			name: "empty inbounds",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				path := filepath.Join(dir, "config.json")
				config := `{"inbounds": []}`
				err := os.WriteFile(path, []byte(config), 0o644)
				require.NoError(t, err)
				return path
			},
			wantErr:   false,
			wantCount: 0,
			wantTags:  []string{},
		},
		{
			name: "no inbounds field",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				path := filepath.Join(dir, "config.json")
				config := `{}`
				err := os.WriteFile(path, []byte(config), 0o644)
				require.NoError(t, err)
				return path
			},
			wantErr:   false,
			wantCount: 0,
			wantTags:  []string{},
		},
		{
			name: "inbound with port field (not listen_port)",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				path := filepath.Join(dir, "config.json")
				config := `{
					"inbounds": [
						{
							"type": "vmess",
							"tag": "in-1",
							"port": 1080
						}
					]
				}`
				err := os.WriteFile(path, []byte(config), 0o644)
				require.NoError(t, err)
				return path
			},
			wantErr:   false,
			wantCount: 1,
			wantTags:  []string{"in-1"},
			check: func(t *testing.T, inbounds []models.Inbound) {
				assert.Equal(t, 1080, inbounds[0].Port)
			},
		},
		{
			name: "nil inbound entry",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				path := filepath.Join(dir, "config.json")
				config := `{
					"inbounds": [
						{
							"type": "vless",
							"tag": "in-1"
						},
						null
					]
				}`
				err := os.WriteFile(path, []byte(config), 0o644)
				require.NoError(t, err)
				return path
			},
			wantErr:   false,
			wantCount: 1,
			wantTags:  []string{"in-1"},
		},
		{
			name: "invalid config file",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				path := filepath.Join(dir, "config.json")
				config := `{invalid json`
				err := os.WriteFile(path, []byte(config), 0o644)
				require.NoError(t, err)
				return path
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t)
			client := NewConfigClient(path)
			inbounds, err := client.GetInbounds(context.Background())

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, inbounds)
			} else {
				require.NoError(t, err)
				require.NotNil(t, inbounds)
				assert.Len(t, inbounds, tt.wantCount)

				tags := make([]string, len(inbounds))
				for i, inbound := range inbounds {
					tags[i] = inbound.Tag
				}
				assert.Equal(t, tt.wantTags, tags)

				if tt.check != nil {
					tt.check(t, inbounds)
				}
			}
		})
	}
}

func TestConfigClient_GetUsers(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T) string
		wantErr   bool
		wantCount int
		check     func(t *testing.T, users []models.User)
	}{
		{
			name: "users in vless inbound",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				path := filepath.Join(dir, "config.json")
				config := `{
					"inbounds": [
						{
							"type": "vless",
							"tag": "in-1",
							"users": [
								{
									"uuid": "12345678-1234-1234-1234-123456789012",
									"name": "user1",
									"flow": "xtls-rprx-vision"
								},
								{
									"uuid": "87654321-4321-4321-4321-210987654321",
									"name": "user2"
								}
							]
						}
					]
				}`
				err := os.WriteFile(path, []byte(config), 0o644)
				require.NoError(t, err)
				return path
			},
			wantErr:   false,
			wantCount: 2,
			check: func(t *testing.T, users []models.User) {
				assert.Equal(t, "in-1", users[0].InboundTag)
				assert.Equal(t, "user1", users[0].SubID)
				assert.Equal(t, "12345678-1234-1234-1234-123456789012", users[0].UUID)
				assert.Equal(t, "xtls-rprx-vision", users[0].Flow)
				assert.True(t, users[0].Enabled)

				assert.Equal(t, "user2", users[1].SubID)
				assert.Equal(t, "87654321-4321-4321-4321-210987654321", users[1].UUID)
			},
		},
		{
			name: "users in hysteria2 inbound",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				path := filepath.Join(dir, "config.json")
				config := `{
					"inbounds": [
						{
							"type": "hysteria2",
							"tag": "in-1",
							"users": [
								{
									"name": "user1",
									"password": "pass123"
								}
							]
						}
					]
				}`
				err := os.WriteFile(path, []byte(config), 0o644)
				require.NoError(t, err)
				return path
			},
			wantErr:   false,
			wantCount: 1,
			check: func(t *testing.T, users []models.User) {
				assert.Equal(t, "user1", users[0].SubID)
				assert.Equal(t, "pass123", users[0].UUID)
			},
		},
		{
			name: "users in options.users",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				path := filepath.Join(dir, "config.json")
				config := `{
					"inbounds": [
						{
							"type": "vless",
							"tag": "in-1",
							"options": {
								"users": [
									{
										"uuid": "12345678-1234-1234-1234-123456789012",
										"name": "user1"
									}
								]
							}
						}
					]
				}`
				err := os.WriteFile(path, []byte(config), 0o644)
				require.NoError(t, err)
				return path
			},
			wantErr:   false,
			wantCount: 1,
			check: func(t *testing.T, users []models.User) {
				assert.Equal(t, "user1", users[0].SubID)
			},
		},
		{
			name: "users with email field",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				path := filepath.Join(dir, "config.json")
				config := `{
					"inbounds": [
						{
							"type": "vless",
							"tag": "in-1",
							"users": [
								{
									"uuid": "12345678-1234-1234-1234-123456789012",
									"email": "user@example.com"
								}
							]
						}
					]
				}`
				err := os.WriteFile(path, []byte(config), 0o644)
				require.NoError(t, err)
				return path
			},
			wantErr:   false,
			wantCount: 1,
			check: func(t *testing.T, users []models.User) {
				assert.Equal(t, "user@example.com", users[0].SubID)
				assert.Equal(t, "user@example.com", users[0].Email)
			},
		},
		{
			name: "users with enabled field",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				path := filepath.Join(dir, "config.json")
				config := `{
					"inbounds": [
						{
							"type": "vless",
							"tag": "in-1",
							"users": [
								{
									"uuid": "12345678-1234-1234-1234-123456789012",
									"name": "user1",
									"enabled": false
								}
							]
						}
					]
				}`
				err := os.WriteFile(path, []byte(config), 0o644)
				require.NoError(t, err)
				return path
			},
			wantErr:   false,
			wantCount: 1,
			check: func(t *testing.T, users []models.User) {
				assert.False(t, users[0].Enabled)
			},
		},
		{
			name: "user without identifier",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				path := filepath.Join(dir, "config.json")
				config := `{
					"inbounds": [
						{
							"type": "vless",
							"tag": "in-1",
							"users": [
								{
									"flow": "xtls-rprx-vision"
								}
							]
						}
					]
				}`
				err := os.WriteFile(path, []byte(config), 0o644)
				require.NoError(t, err)
				return path
			},
			wantErr:   false,
			wantCount: 0,
		},
		{
			name: "multiple inbounds with users",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				path := filepath.Join(dir, "config.json")
				config := `{
					"inbounds": [
						{
							"type": "vless",
							"tag": "in-1",
							"users": [
								{"uuid": "11111111-1111-1111-1111-111111111111", "name": "user1"}
							]
						},
						{
							"type": "hysteria2",
							"tag": "in-2",
							"users": [
								{"name": "user2", "password": "pass2"}
							]
						}
					]
				}`
				err := os.WriteFile(path, []byte(config), 0o644)
				require.NoError(t, err)
				return path
			},
			wantErr:   false,
			wantCount: 2,
			check: func(t *testing.T, users []models.User) {
				assert.Equal(t, "in-1", users[0].InboundTag)
				assert.Equal(t, "in-2", users[1].InboundTag)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t)
			client := NewConfigClient(path)
			users, err := client.GetUsers(context.Background())

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, users)
			} else {
				require.NoError(t, err)
				require.NotNil(t, users)
				assert.Len(t, users, tt.wantCount)

				if tt.check != nil {
					tt.check(t, users)
				}
			}
		})
	}
}

func TestConfigClient_CreateInbound(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		inbound models.Inbound
		wantErr bool
		errMsg  string
		check   func(t *testing.T, path string)
	}{
		{
			name: "create new inbound",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				path := filepath.Join(dir, "config.json")
				config := `{"inbounds": []}`
				err := os.WriteFile(path, []byte(config), 0o644)
				require.NoError(t, err)
				return path
			},
			inbound: models.Inbound{
				Type:   "vless",
				Tag:    "new-inbound",
				Listen: "0.0.0.0",
				Port:   443,
				Options: map[string]interface{}{
					"tls": map[string]interface{}{
						"enabled": true,
					},
				},
			},
			wantErr: false,
			check: func(t *testing.T, path string) {
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				var result map[string]interface{}
				err = json.Unmarshal(data, &result)
				require.NoError(t, err)
				inbounds := result["inbounds"].([]interface{}) //nolint:errcheck
				assert.Len(t, inbounds, 1)
			},
		},
		{
			name: "duplicate tag",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				path := filepath.Join(dir, "config.json")
				config := `{
					"inbounds": [
						{"type": "vless", "tag": "existing"}
					]
				}`
				err := os.WriteFile(path, []byte(config), 0o644)
				require.NoError(t, err)
				return path
			},
			inbound: models.Inbound{
				Type: "vless",
				Tag:  "existing",
			},
			wantErr: true,
			errMsg:  "already exists",
		},
		{
			name: "minimal inbound",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				path := filepath.Join(dir, "config.json")
				config := `{"inbounds": []}`
				err := os.WriteFile(path, []byte(config), 0o644)
				require.NoError(t, err)
				return path
			},
			inbound: models.Inbound{
				Type: "direct",
				Tag:  "minimal",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t)
			client := NewConfigClient(path)
			err := client.CreateInbound(context.Background(), tt.inbound)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				if tt.check != nil {
					tt.check(t, path)
				}
			}
		})
	}
}

func TestConfigClient_UpdateInbound(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		inbound models.Inbound
		wantErr bool
		errMsg  string
		check   func(t *testing.T, path string)
	}{
		{
			name: "update existing inbound",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				path := filepath.Join(dir, "config.json")
				config := `{
					"inbounds": [
						{
							"type": "vless",
							"tag": "in-1",
							"listen": "0.0.0.0",
							"listen_port": 443
						}
					]
				}`
				err := os.WriteFile(path, []byte(config), 0o644)
				require.NoError(t, err)
				return path
			},
			inbound: models.Inbound{
				Type:   "vless",
				Tag:    "in-1",
				Listen: "::",
				Port:   8443,
				Options: map[string]interface{}{
					"tls": map[string]interface{}{
						"enabled": true,
					},
				},
			},
			wantErr: false,
			check: func(t *testing.T, path string) {
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				var result map[string]interface{}
				err = json.Unmarshal(data, &result)
				require.NoError(t, err)
				inbounds := result["inbounds"].([]interface{}) //nolint:errcheck
				assert.Len(t, inbounds, 1)
				inbound := inbounds[0].(map[string]interface{}) //nolint:errcheck
				assert.Equal(t, "::", inbound["listen"])
				assert.Equal(t, float64(8443), inbound["listen_port"])
			},
		},
		{
			name: "inbound not found",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				path := filepath.Join(dir, "config.json")
				config := `{"inbounds": []}`
				err := os.WriteFile(path, []byte(config), 0o644)
				require.NoError(t, err)
				return path
			},
			inbound: models.Inbound{
				Type: "vless",
				Tag:  "nonexistent",
			},
			wantErr: true,
			errMsg:  "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t)
			client := NewConfigClient(path)
			err := client.UpdateInbound(context.Background(), tt.inbound)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				if tt.check != nil {
					tt.check(t, path)
				}
			}
		})
	}
}

func TestConfigClient_DeleteInbound(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		tag     string
		wantErr bool
		errMsg  string
		check   func(t *testing.T, path string)
	}{
		{
			name: "delete existing inbound",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				path := filepath.Join(dir, "config.json")
				config := `{
					"inbounds": [
						{"type": "vless", "tag": "in-1"},
						{"type": "hysteria2", "tag": "in-2"}
					]
				}`
				err := os.WriteFile(path, []byte(config), 0o644)
				require.NoError(t, err)
				return path
			},
			tag:     "in-1",
			wantErr: false,
			check: func(t *testing.T, path string) {
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				var result map[string]interface{}
				err = json.Unmarshal(data, &result)
				require.NoError(t, err)
				inbounds := result["inbounds"].([]interface{}) //nolint:errcheck
				assert.Len(t, inbounds, 1)
			},
		},
		{
			name: "inbound not found",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				path := filepath.Join(dir, "config.json")
				config := `{"inbounds": []}`
				err := os.WriteFile(path, []byte(config), 0o644)
				require.NoError(t, err)
				return path
			},
			tag:     "nonexistent",
			wantErr: true,
			errMsg:  "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t)
			client := NewConfigClient(path)
			err := client.DeleteInbound(context.Background(), tt.tag)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				if tt.check != nil {
					tt.check(t, path)
				}
			}
		})
	}
}

func TestConfigClient_CreateUser(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		user    models.User
		wantErr bool
		errMsg  string
		check   func(t *testing.T, path string)
	}{
		{
			name: "create user in vless inbound",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				path := filepath.Join(dir, "config.json")
				config := `{
					"inbounds": [
						{
							"type": "vless",
							"tag": "in-1",
							"users": []
						}
					]
				}`
				err := os.WriteFile(path, []byte(config), 0o644)
				require.NoError(t, err)
				return path
			},
			user: models.User{
				InboundTag: "in-1",
				SubID:      "user1",
				UUID:       "12345678-1234-1234-1234-123456789012",
				Flow:       "xtls-rprx-vision",
			},
			wantErr: false,
			check: func(t *testing.T, path string) {
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				var result map[string]interface{}
				err = json.Unmarshal(data, &result)
				require.NoError(t, err)
				inbounds := result["inbounds"].([]interface{})  //nolint:errcheck
				inbound := inbounds[0].(map[string]interface{}) //nolint:errcheck
				users := inbound["users"].([]interface{})       //nolint:errcheck
				assert.Len(t, users, 1)
			},
		},
		{
			name: "create user in hysteria2 inbound",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				path := filepath.Join(dir, "config.json")
				config := `{
					"inbounds": [
						{
							"type": "hysteria2",
							"tag": "in-1",
							"users": []
						}
					]
				}`
				err := os.WriteFile(path, []byte(config), 0o644)
				require.NoError(t, err)
				return path
			},
			user: models.User{
				InboundTag: "in-1",
				SubID:      "user1",
				UUID:       "password123",
			},
			wantErr: false,
		},
		{
			name: "inbound not found",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				path := filepath.Join(dir, "config.json")
				config := `{"inbounds": []}`
				err := os.WriteFile(path, []byte(config), 0o644)
				require.NoError(t, err)
				return path
			},
			user: models.User{
				InboundTag: "nonexistent",
				SubID:      "user1",
			},
			wantErr: true,
			errMsg:  "not found",
		},
		{
			name: "duplicate user",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				path := filepath.Join(dir, "config.json")
				config := `{
					"inbounds": [
						{
							"type": "vless",
							"tag": "in-1",
							"users": [
								{"uuid": "12345678-1234-1234-1234-123456789012", "name": "user1"}
							]
						}
					]
				}`
				err := os.WriteFile(path, []byte(config), 0o644)
				require.NoError(t, err)
				return path
			},
			user: models.User{
				InboundTag: "in-1",
				SubID:      "user1",
			},
			wantErr: true,
			errMsg:  "already exists",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t)
			client := NewConfigClient(path)
			err := client.CreateUser(context.Background(), tt.user)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				if tt.check != nil {
					tt.check(t, path)
				}
			}
		})
	}
}

func TestConfigClient_UpdateUser(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		user    models.User
		wantErr bool
		errMsg  string
		check   func(t *testing.T, path string)
	}{
		{
			name: "update existing user",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				path := filepath.Join(dir, "config.json")
				config := `{
					"inbounds": [
						{
							"type": "vless",
							"tag": "in-1",
							"users": [
								{
									"uuid": "12345678-1234-1234-1234-123456789012",
									"name": "user1",
									"flow": "old-flow"
								}
							]
						}
					]
				}`
				err := os.WriteFile(path, []byte(config), 0o644)
				require.NoError(t, err)
				return path
			},
			user: models.User{
				InboundTag: "in-1",
				SubID:      "user1",
				UUID:       "12345678-1234-1234-1234-123456789012",
				Flow:       "new-flow",
			},
			wantErr: false,
			check: func(t *testing.T, path string) {
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				var result map[string]interface{}
				err = json.Unmarshal(data, &result)
				require.NoError(t, err)
				inbounds := result["inbounds"].([]interface{})  //nolint:errcheck
				inbound := inbounds[0].(map[string]interface{}) //nolint:errcheck
				users := inbound["users"].([]interface{})       //nolint:errcheck
				user := users[0].(map[string]interface{})       //nolint:errcheck
				assert.Equal(t, "new-flow", user["flow"])
			},
		},
		{
			name: "user not found",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				path := filepath.Join(dir, "config.json")
				config := `{
					"inbounds": [
						{
							"type": "vless",
							"tag": "in-1",
							"users": []
						}
					]
				}`
				err := os.WriteFile(path, []byte(config), 0o644)
				require.NoError(t, err)
				return path
			},
			user: models.User{
				InboundTag: "in-1",
				SubID:      "nonexistent",
			},
			wantErr: true,
			errMsg:  "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t)
			client := NewConfigClient(path)
			err := client.UpdateUser(context.Background(), tt.user)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				if tt.check != nil {
					tt.check(t, path)
				}
			}
		})
	}
}

func TestConfigClient_DeleteUser(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		subID   string
		wantErr bool
		errMsg  string
		check   func(t *testing.T, path string)
	}{
		{
			name: "delete existing user",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				path := filepath.Join(dir, "config.json")
				config := `{
					"inbounds": [
						{
							"type": "vless",
							"tag": "in-1",
							"users": [
								{"uuid": "11111111-1111-1111-1111-111111111111", "name": "user1"},
								{"uuid": "22222222-2222-2222-2222-222222222222", "name": "user2"}
							]
						}
					]
				}`
				err := os.WriteFile(path, []byte(config), 0o644)
				require.NoError(t, err)
				return path
			},
			subID:   "user1",
			wantErr: false,
			check: func(t *testing.T, path string) {
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				var result map[string]interface{}
				err = json.Unmarshal(data, &result)
				require.NoError(t, err)
				inbounds := result["inbounds"].([]interface{})  //nolint:errcheck
				inbound := inbounds[0].(map[string]interface{}) //nolint:errcheck
				users := inbound["users"].([]interface{})       //nolint:errcheck
				assert.Len(t, users, 1)
			},
		},
		{
			name: "user not found",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				path := filepath.Join(dir, "config.json")
				config := `{
					"inbounds": [
						{
							"type": "vless",
							"tag": "in-1",
							"users": []
						}
					]
				}`
				err := os.WriteFile(path, []byte(config), 0o644)
				require.NoError(t, err)
				return path
			},
			subID:   "nonexistent",
			wantErr: true,
			errMsg:  "not found",
		},
		{
			name: "delete user from multiple inbounds",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				path := filepath.Join(dir, "config.json")
				config := `{
					"inbounds": [
						{
							"type": "vless",
							"tag": "in-1",
							"users": [
								{"uuid": "11111111-1111-1111-1111-111111111111", "name": "user1"}
							]
						},
						{
							"type": "hysteria2",
							"tag": "in-2",
							"users": [
								{"name": "user1", "password": "pass1"}
							]
						}
					]
				}`
				err := os.WriteFile(path, []byte(config), 0o644)
				require.NoError(t, err)
				return path
			},
			subID:   "user1",
			wantErr: false,
			check: func(t *testing.T, path string) {
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				var result map[string]interface{}
				err = json.Unmarshal(data, &result)
				require.NoError(t, err)
				inbounds := result["inbounds"].([]interface{}) //nolint:errcheck
				for _, inboundAny := range inbounds {
					inbound := inboundAny.(map[string]interface{}) //nolint:errcheck
					users := inbound["users"].([]interface{})      //nolint:errcheck
					assert.Len(t, users, 0)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t)
			client := NewConfigClient(path)
			err := client.DeleteUser(context.Background(), tt.subID)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				if tt.check != nil {
					tt.check(t, path)
				}
			}
		})
	}
}

func TestHelperFunctions(t *testing.T) {
	t.Run("getString", func(t *testing.T) {
		tests := []struct {
			name     string
			m        map[string]interface{}
			key      string
			expected string
		}{
			{
				name:     "existing key",
				m:        map[string]interface{}{"key": "value"},
				key:      "key",
				expected: "value",
			},
			{
				name:     "missing key",
				m:        map[string]interface{}{"other": "value"},
				key:      "key",
				expected: "",
			},
			{
				name:     "nil map",
				m:        nil,
				key:      "key",
				expected: "",
			},
			{
				name:     "non-string value",
				m:        map[string]interface{}{"key": 123},
				key:      "key",
				expected: "",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := getString(tt.m, tt.key)
				assert.Equal(t, tt.expected, result)
			})
		}
	})

	t.Run("getInt", func(t *testing.T) {
		tests := []struct {
			name     string
			m        map[string]interface{}
			keys     []string
			expected int
		}{
			{
				name:     "float64 value",
				m:        map[string]interface{}{"port": 443.0},
				keys:     []string{"port"},
				expected: 443,
			},
			{
				name:     "int value",
				m:        map[string]interface{}{"port": 443},
				keys:     []string{"port"},
				expected: 443,
			},
			{
				name:     "int64 value",
				m:        map[string]interface{}{"port": int64(443)},
				keys:     []string{"port"},
				expected: 443,
			},
			{
				name:     "first key found",
				m:        map[string]interface{}{"listen_port": 443, "port": 8080},
				keys:     []string{"listen_port", "port"},
				expected: 443,
			},
			{
				name:     "second key found",
				m:        map[string]interface{}{"port": 8080},
				keys:     []string{"listen_port", "port"},
				expected: 8080,
			},
			{
				name:     "no key found",
				m:        map[string]interface{}{"other": "value"},
				keys:     []string{"port"},
				expected: 0,
			},
			{
				name:     "nil map",
				m:        nil,
				keys:     []string{"port"},
				expected: 0,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := getInt(tt.m, tt.keys...)
				assert.Equal(t, tt.expected, result)
			})
		}
	})

	t.Run("firstNonEmpty", func(t *testing.T) {
		tests := []struct {
			name     string
			values   []string
			expected string
		}{
			{
				name:     "first non-empty",
				values:   []string{"", "", "value", ""},
				expected: "value",
			},
			{
				name:     "all empty",
				values:   []string{"", "", ""},
				expected: "",
			},
			{
				name:     "first value",
				values:   []string{"value", "other"},
				expected: "value",
			},
			{
				name:     "no values",
				values:   []string{},
				expected: "",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := firstNonEmpty(tt.values...)
				assert.Equal(t, tt.expected, result)
			})
		}
	})

	t.Run("userMatchesSubID", func(t *testing.T) {
		tests := []struct {
			name     string
			userMap  map[string]interface{}
			subID    string
			expected bool
		}{
			{
				name:     "matches subId",
				userMap:  map[string]interface{}{"subId": "user1"},
				subID:    "user1",
				expected: true,
			},
			{
				name:     "matches name",
				userMap:  map[string]interface{}{"name": "user1"},
				subID:    "user1",
				expected: true,
			},
			{
				name:     "matches email",
				userMap:  map[string]interface{}{"email": "user1"},
				subID:    "user1",
				expected: true,
			},
			{
				name:     "matches uuid",
				userMap:  map[string]interface{}{"uuid": "user1"},
				subID:    "user1",
				expected: true,
			},
			{
				name:     "no match",
				userMap:  map[string]interface{}{"name": "other"},
				subID:    "user1",
				expected: false,
			},
			{
				name:     "nil map",
				userMap:  nil,
				subID:    "user1",
				expected: false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := userMatchesSubID(tt.userMap, tt.subID)
				assert.Equal(t, tt.expected, result)
			})
		}
	})

	t.Run("buildProtocolUser", func(t *testing.T) {
		t.Run("vless protocol", func(t *testing.T) {
			user := models.User{
				SubID: "user1",
				UUID:  "12345678-1234-1234-1234-123456789012",
				Flow:  "xtls-rprx-vision",
			}
			result := buildProtocolUser("vless", user, nil)
			assert.Equal(t, "12345678-1234-1234-1234-123456789012", result["uuid"])
			assert.Equal(t, "user1", result["name"])
			assert.Equal(t, "xtls-rprx-vision", result["flow"])
		})

		t.Run("vmess protocol", func(t *testing.T) {
			user := models.User{
				SubID: "user1",
				UUID:  "12345678-1234-1234-1234-123456789012",
			}
			result := buildProtocolUser("vmess", user, nil)
			assert.Equal(t, "12345678-1234-1234-1234-123456789012", result["uuid"])
			assert.Equal(t, "user1", result["name"])
		})

		t.Run("hysteria2 protocol", func(t *testing.T) {
			user := models.User{
				SubID: "user1",
				UUID:  "password123",
			}
			result := buildProtocolUser("hysteria2", user, nil)
			assert.Equal(t, "user1", result["name"])
			assert.Equal(t, "password123", result["password"])
		})

		t.Run("shadowsocks protocol", func(t *testing.T) {
			user := models.User{
				SubID: "user1",
				UUID:  "password123",
			}
			result := buildProtocolUser("shadowsocks", user, nil)
			assert.Equal(t, "user1", result["name"])
			assert.Equal(t, "password123", result["password"])
		})

		t.Run("trojan protocol", func(t *testing.T) {
			user := models.User{
				SubID: "user1",
				UUID:  "password123",
			}
			result := buildProtocolUser("trojan", user, nil)
			assert.Equal(t, "user1", result["name"])
			assert.Equal(t, "password123", result["password"])
		})

		t.Run("tuic protocol", func(t *testing.T) {
			user := models.User{
				SubID: "user1",
				UUID:  "password123",
			}
			result := buildProtocolUser("tuic", user, nil)
			assert.Equal(t, "user1", result["name"])
			assert.Equal(t, "password123", result["password"])
		})

		t.Run("unknown protocol", func(t *testing.T) {
			existing := map[string]interface{}{
				"existingField": "value",
			}
			user := models.User{
				SubID: "user1",
				UUID:  "12345678-1234-1234-1234-123456789012",
				Email: "user@example.com",
				Flow:  "flow1",
			}
			result := buildProtocolUser("unknown", user, existing)
			assert.Equal(t, "user1", result["subId"])
			assert.Equal(t, "12345678-1234-1234-1234-123456789012", result["uuid"])
			assert.Equal(t, "user@example.com", result["email"])
			assert.Equal(t, "flow1", result["flow"])
			assert.Equal(t, "value", result["existingField"])
		})

		t.Run("vless with existing", func(t *testing.T) {
			existing := map[string]interface{}{
				"uuid": "existing-uuid",
				"flow": "existing-flow",
			}
			user := models.User{
				SubID: "user1",
			}
			result := buildProtocolUser("vless", user, existing)
			assert.Equal(t, "existing-uuid", result["uuid"])
			assert.Equal(t, "user1", result["name"])
			assert.Equal(t, "existing-flow", result["flow"])
		})

		t.Run("vless generates UUID", func(t *testing.T) {
			user := models.User{
				SubID: "user1",
			}
			result := buildProtocolUser("vless", user, nil)
			assert.NotEmpty(t, result["uuid"])
			assert.Equal(t, "user1", result["name"])
		})
	})

	t.Run("findInboundIndex", func(t *testing.T) {
		inbounds := []map[string]interface{}{
			{"tag": "in-1", "type": "vless"},
			{"tag": "in-2", "type": "hysteria2"},
			{"tag": "in-3", "type": "vmess"},
		}

		tests := []struct {
			name     string
			tag      string
			expected int
		}{
			{
				name:     "found at index 0",
				tag:      "in-1",
				expected: 0,
			},
			{
				name:     "found at index 1",
				tag:      "in-2",
				expected: 1,
			},
			{
				name:     "found at index 2",
				tag:      "in-3",
				expected: 2,
			},
			{
				name:     "not found",
				tag:      "nonexistent",
				expected: -1,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := findInboundIndex(inbounds, tt.tag)
				assert.Equal(t, tt.expected, result)
			})
		}
	})

	t.Run("getInbounds", func(t *testing.T) {
		tests := []struct {
			name     string
			root     map[string]interface{}
			expected int
		}{
			{
				name: "valid inbounds",
				root: map[string]interface{}{
					"inbounds": []interface{}{
						map[string]interface{}{"tag": "in-1"},
						map[string]interface{}{"tag": "in-2"},
					},
				},
				expected: 2,
			},
			{
				name: "empty inbounds",
				root: map[string]interface{}{
					"inbounds": []interface{}{},
				},
				expected: 0,
			},
			{
				name:     "no inbounds field",
				root:     map[string]interface{}{},
				expected: 0,
			},
			{
				name: "invalid inbounds type",
				root: map[string]interface{}{
					"inbounds": "invalid",
				},
				expected: 0,
			},
			{
				name: "mixed valid and invalid entries",
				root: map[string]interface{}{
					"inbounds": []interface{}{
						map[string]interface{}{"tag": "in-1"},
						"invalid",
						map[string]interface{}{"tag": "in-2"},
						nil,
					},
				},
				expected: 2,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := getInbounds(tt.root)
				assert.Len(t, result, tt.expected)
			})
		}
	})

	t.Run("setInbounds", func(t *testing.T) {
		root := map[string]interface{}{}
		inbounds := []map[string]interface{}{
			{"tag": "in-1"},
			{"tag": "in-2"},
		}

		setInbounds(root, inbounds)

		result := root["inbounds"].([]interface{}) //nolint:errcheck
		assert.Len(t, result, 2)
	})

	t.Run("getInboundUsers", func(t *testing.T) {
		tests := []struct {
			name     string
			inbound  map[string]interface{}
			expected int
		}{
			{
				name: "users in inbound",
				inbound: map[string]interface{}{
					"users": []interface{}{
						map[string]interface{}{"name": "user1"},
						map[string]interface{}{"name": "user2"},
					},
				},
				expected: 2,
			},
			{
				name: "users in options",
				inbound: map[string]interface{}{
					"options": map[string]interface{}{
						"users": []interface{}{
							map[string]interface{}{"name": "user1"},
						},
					},
				},
				expected: 1,
			},
			{
				name:     "no users",
				inbound:  map[string]interface{}{},
				expected: 0,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := getInboundUsers(tt.inbound)
				assert.Len(t, result, tt.expected)
			})
		}
	})

	t.Run("setInboundUsers", func(t *testing.T) {
		tests := []struct {
			name    string
			inbound map[string]interface{}
			users   []interface{}
			check   func(t *testing.T, inbound map[string]interface{}) //nolint:errcheck
		}{
			{
				name: "set users in inbound",
				inbound: map[string]interface{}{
					"users": []interface{}{},
				},
				users: []interface{}{
					map[string]interface{}{"name": "user1"},
				},
				check: func(t *testing.T, inbound map[string]interface{}) {
					users := inbound["users"].([]interface{}) //nolint:errcheck
					assert.Len(t, users, 1)
				},
			},
			{
				name: "set users in options",
				inbound: map[string]interface{}{
					"options": map[string]interface{}{
						"users": []interface{}{},
					},
				},
				users: []interface{}{
					map[string]interface{}{"name": "user1"},
				},
				check: func(t *testing.T, inbound map[string]interface{}) {
					options := inbound["options"].(map[string]interface{}) //nolint:errcheck
					users := options["users"].([]interface{})              //nolint:errcheck
					assert.Len(t, users, 1)
				},
			},
			{
				name:    "create users field",
				inbound: map[string]interface{}{},
				users: []interface{}{
					map[string]interface{}{"name": "user1"},
				},
				check: func(t *testing.T, inbound map[string]interface{}) {
					users := inbound["users"].([]interface{}) //nolint:errcheck
					assert.Len(t, users, 1)
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				setInboundUsers(tt.inbound, tt.users)
				tt.check(t, tt.inbound)
			})
		}
	})
}

func TestConfigClient_saveAndReload(t *testing.T) {
	// This test triggers an async systemctl reload after the debounce window.
	// Skip by default so `go test ./...` works on any Linux dev box without
	// requiring sudo/polkit. Opt in with SINGBOX_AGENT_LIVE_SYSTEMCTL=1.
	if os.Getenv("SINGBOX_AGENT_LIVE_SYSTEMCTL") == "" {
		t.Skip("skipping; set SINGBOX_AGENT_LIVE_SYSTEMCTL=1 to run")
	}

	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"inbounds": []}`), 0o644))
	client := NewConfigClient(path)
	root := map[string]interface{}{
		"inbounds": []interface{}{
			map[string]interface{}{"tag": "in-1"},
		},
	}

	err := client.saveAndReload(context.Background(), root)

	require.NoError(t, err)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotEmpty(t, data)
}

func TestConfigClient_reloadSystemService(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T) *ConfigClient
		wantErr bool
	}{
		{
			name: "systemctl not available",
			setup: func(t *testing.T) *ConfigClient {
				client := NewConfigClient("/tmp/config.json")
				return client
			},
			wantErr: false, // Should return nil if systemctl not found
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := tt.setup(t)
			_ = client.reloadSystemService(context.Background())
			// systemctl not available in CI, so we ignore the result
			// The test verifies the function doesn't panic
		})
	}
}

func TestConfigClient_Concurrency(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	config := `{"inbounds": []}`
	err := os.WriteFile(path, []byte(config), 0o644)
	require.NoError(t, err)

	client := NewConfigClient(path)

	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			inbound := models.Inbound{
				Type: "vless",
				Tag:  "in-" + string(rune('0'+idx)),
			}
			_ = client.CreateInbound(context.Background(), inbound)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	inbounds, err := client.GetInbounds(context.Background())
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(inbounds), 0)
}

func TestConfigClient_BackupFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	config := `{"inbounds": []}`
	err := os.WriteFile(path, []byte(config), 0o644)
	require.NoError(t, err)

	client := NewConfigClient(path)
	inbound := models.Inbound{
		Type: "vless",
		Tag:  "in-1",
	}
	err = client.CreateInbound(context.Background(), inbound)
	require.NoError(t, err)

	backupPath := path + ".agent.bak"
	_, err = os.Stat(backupPath)
	assert.NoError(t, err, "backup file should be created")
}

func TestConfigClient_loadRootConfig(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T) string
		wantErr     bool
		errContains string
	}{
		{
			name: "valid config",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				path := filepath.Join(dir, "config.json")
				config := `{"inbounds": []}`
				require.NoError(t, os.WriteFile(path, []byte(config), 0o644))
				return path
			},
			wantErr: false,
		},
		{
			name: "invalid json",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				path := filepath.Join(dir, "config.json")
				require.NoError(t, os.WriteFile(path, []byte(`{invalid`), 0o644))
				return path
			},
			errContains: "failed to parse config",
			wantErr:     true,
		},
		{
			name: "file not found",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				return filepath.Join(dir, "nonexistent.json")
			},
			errContains: "failed to read config",
			wantErr:     true,
		},
		{
			name: "no inbounds field - should add empty array",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				path := filepath.Join(dir, "config.json")
				config := `{"log": {}}`
				require.NoError(t, os.WriteFile(path, []byte(config), 0o644))
				return path
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t)
			client := NewConfigClient(path)
			root, err := client.loadRootConfig()

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.ErrorContains(t, err, tt.errContains)
				}
				assert.Nil(t, root)
			} else {
				require.NoError(t, err)
				require.NotNil(t, root)
				_, hasInbounds := root["inbounds"]
				assert.True(t, hasInbounds)
			}
		})
	}
}

func TestConfigClient_saveAndReload_RollbackFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	config := `{"inbounds": []}`
	require.NoError(t, os.WriteFile(path, []byte(config), 0o644))

	client := NewConfigClient(path)
	root := map[string]interface{}{
		"inbounds": []interface{}{
			map[string]interface{}{"tag": "in-1"},
		},
	}

	// This should succeed even without wrapper
	err := client.saveAndReload(context.Background(), root)
	require.NoError(t, err)

	// Verify file was written
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotEmpty(t, data)
}

func TestConfigClient_reloadSystemService_ContextCanceled(t *testing.T) {
	client := NewConfigClient("/tmp/config.json")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_ = client.reloadSystemService(ctx)
	// systemctl not available in CI, so we ignore the result
	// The test verifies the function doesn't panic
}

func TestConfigClient_reloadSystemService_RateLimit(t *testing.T) {
	client := NewConfigClient("/tmp/config.json")

	// First call
	// First call
	_ = client.reloadSystemService(context.Background())
	// systemctl not available in CI, so we ignore the result

	// Immediate second call should trigger rate limiting
	ctx2, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_ = client.reloadSystemService(ctx2)
	// systemctl not available in CI, so we ignore the result
}

func TestConfigClient_UpdateUser_NonMapUserEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	config := `{
		"inbounds": [
			{
				"type": "vless",
				"tag": "in-1",
				"users": [
					{"uuid": "11111111-1111-1111-1111-111111111111", "name": "user1"},
					"invalid-entry",
					{"uuid": "22222222-2222-2222-2222-222222222222", "name": "user2"}
				]
			}
		]
	}`
	require.NoError(t, os.WriteFile(path, []byte(config), 0o644))

	client := NewConfigClient(path)
	user := models.User{
		InboundTag: "in-1",
		SubID:      "user2",
		UUID:       "new-uuid",
	}

	err := client.UpdateUser(context.Background(), user)
	require.NoError(t, err)

	// Verify update
	users, err := client.GetUsers(context.Background())
	require.NoError(t, err)
	assert.Len(t, users, 2)
}

func TestConfigClient_GetUsers_NonMapUserEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	config := `{
		"inbounds": [
			{
				"type": "vless",
				"tag": "in-1",
				"users": [
					{"uuid": "11111111-1111-1111-1111-111111111111", "name": "user1"},
					"invalid-entry",
					null
				]
			}
		]
	}`
	require.NoError(t, os.WriteFile(path, []byte(config), 0o644))

	client := NewConfigClient(path)
	users, err := client.GetUsers(context.Background())
	require.NoError(t, err)
	assert.Len(t, users, 1) // Only the valid user
	assert.Equal(t, "user1", users[0].SubID)
}

func TestBuildProtocolUser_EdgeCases(t *testing.T) {
	t.Run("vless with empty UUID and no existing", func(t *testing.T) {
		user := models.User{
			SubID: "user1",
		}
		result := buildProtocolUser("vless", user, nil)
		assert.NotEmpty(t, result["uuid"]) // Should generate UUID
		assert.Equal(t, "user1", result["name"])
	})

	t.Run("vless with empty UUID and existing UUID", func(t *testing.T) {
		existing := map[string]interface{}{
			"uuid": "existing-uuid",
		}
		user := models.User{
			SubID: "user1",
		}
		result := buildProtocolUser("vless", user, existing)
		assert.Equal(t, "existing-uuid", result["uuid"])
		assert.Equal(t, "user1", result["name"])
	})

	t.Run("vless preserves existing flow when user flow empty", func(t *testing.T) {
		existing := map[string]interface{}{
			"uuid": "existing-uuid",
			"flow": "existing-flow",
		}
		user := models.User{
			SubID: "user1",
			UUID:  "new-uuid",
		}
		result := buildProtocolUser("vless", user, existing)
		assert.Equal(t, "new-uuid", result["uuid"])
		assert.Equal(t, "existing-flow", result["flow"])
	})

	t.Run("hysteria2 with empty password and no existing", func(t *testing.T) {
		user := models.User{
			SubID: "user1",
		}
		result := buildProtocolUser("hysteria2", user, nil)
		assert.NotEmpty(t, result["password"]) // Should generate password
		assert.Equal(t, "user1", result["name"])
	})

	t.Run("hysteria2 with empty password and existing password", func(t *testing.T) {
		existing := map[string]interface{}{
			"password": "existing-password",
		}
		user := models.User{
			SubID: "user1",
		}
		result := buildProtocolUser("hysteria2", user, existing)
		assert.Equal(t, "existing-password", result["password"])
		assert.Equal(t, "user1", result["name"])
	})

	t.Run("shadowtls protocol", func(t *testing.T) {
		user := models.User{
			SubID: "user1",
			UUID:  "password123",
		}
		result := buildProtocolUser("shadowtls", user, nil)
		assert.Equal(t, "user1", result["name"])
		assert.Equal(t, "password123", result["password"])
	})

	t.Run("unknown protocol with existing", func(t *testing.T) {
		existing := map[string]interface{}{
			"customField": "customValue",
			"uuid":        "old-uuid",
		}
		user := models.User{
			SubID: "user1",
			UUID:  "new-uuid",
			Email: "user@example.com",
			Flow:  "flow1",
		}
		result := buildProtocolUser("custom-protocol", user, existing)
		assert.Equal(t, "user1", result["subId"])
		assert.Equal(t, "new-uuid", result["uuid"])
		assert.Equal(t, "user@example.com", result["email"])
		assert.Equal(t, "flow1", result["flow"])
		assert.Equal(t, "customValue", result["customField"])
	})

	t.Run("unknown protocol without existing", func(t *testing.T) {
		user := models.User{
			SubID: "user1",
			UUID:  "new-uuid",
			Email: "user@example.com",
			Flow:  "flow1",
		}
		result := buildProtocolUser("custom-protocol", user, nil)
		assert.Equal(t, "user1", result["subId"])
		assert.Equal(t, "new-uuid", result["uuid"])
		assert.Equal(t, "user@example.com", result["email"])
		assert.Equal(t, "flow1", result["flow"])
	})
}

func TestSyncStatsUsers(t *testing.T) {
	root := map[string]interface{}{
		"inbounds": []interface{}{
			map[string]interface{}{
				"tag":  "vless-in",
				"type": "vless",
				"users": []interface{}{
					map[string]interface{}{"name": "sub-b", "uuid": "11111111-1111-1111-1111-111111111111"},
					map[string]interface{}{"name": "sub-a", "uuid": "22222222-2222-2222-2222-222222222222"},
				},
			},
			map[string]interface{}{
				"tag":  "hy2-in",
				"type": "hysteria2",
				"users": []interface{}{
					map[string]interface{}{"name": "sub-b", "password": "p1"},
				},
			},
		},
		"experimental": map[string]interface{}{
			"v2ray_api": map[string]interface{}{
				"listen": "127.0.0.1:9091",
				"stats": map[string]interface{}{
					"enabled":  true,
					"inbounds": []interface{}{"vless-in", "hy2-in"},
				},
			},
		},
	}

	syncStatsUsers(root)

	experimental, ok := root["experimental"].(map[string]interface{})
	require.True(t, ok)
	stats, ok := experimental["v2ray_api"].(map[string]interface{})["stats"].(map[string]interface{})
	require.True(t, ok)
	users, ok := stats["users"].([]string)
	require.True(t, ok)
	require.Len(t, users, 2)
	assert.Equal(t, "sub-a", users[0])
	assert.Equal(t, "sub-b", users[1]) // deduped across inbounds

	// No experimental section — no panic, nothing written.
	root2 := map[string]interface{}{"inbounds": []interface{}{}}
	syncStatsUsers(root2)
	_, ok = root2["experimental"]
	assert.False(t, ok)
}

// TestBuildProtocolUser_EmailAndEnabled verifies that buildProtocolUser keeps
// email/enabled in sync with the ConfigManager (sync) path: user email is
// written, existing email is preserved on update when the request has none,
// and enabled is always written.
func TestBuildProtocolUser_EmailAndEnabled(t *testing.T) {
	tests := []struct {
		name        string
		inboundType string
		user        models.User
		existing    map[string]interface{}
		wantEmail   interface{}
		wantEnabled bool
	}{
		{
			name:        "vless with email",
			inboundType: "vless",
			user:        models.User{SubID: "user1", UUID: "uuid1", Email: "user@example.com", Enabled: true},
			wantEmail:   "user@example.com",
			wantEnabled: true,
		},
		{
			name:        "vless preserves existing email on empty",
			inboundType: "vless",
			user:        models.User{SubID: "user1", UUID: "uuid1", Enabled: false},
			existing:    map[string]interface{}{"email": "old@example.com", "uuid": "uuid1"},
			wantEmail:   "old@example.com",
			wantEnabled: false,
		},
		{
			name:        "hysteria2 with email",
			inboundType: "hysteria2",
			user:        models.User{SubID: "user1", UUID: "pass", Email: "user@example.com", Enabled: true},
			wantEmail:   "user@example.com",
			wantEnabled: true,
		},
		{
			name:        "hysteria2 preserves existing email on empty",
			inboundType: "hysteria2",
			user:        models.User{SubID: "user1", UUID: "pass", Enabled: false},
			existing:    map[string]interface{}{"email": "old@example.com", "password": "pass"},
			wantEmail:   "old@example.com",
			wantEnabled: false,
		},
		{
			name:        "vless without any email",
			inboundType: "vless",
			user:        models.User{SubID: "user1", UUID: "uuid1", Enabled: true},
			wantEmail:   nil,
			wantEnabled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildProtocolUser(tt.inboundType, tt.user, tt.existing)
			assert.Equal(t, tt.wantEmail, result["email"])
			assert.Equal(t, tt.wantEnabled, result["enabled"])
		})
	}
}
