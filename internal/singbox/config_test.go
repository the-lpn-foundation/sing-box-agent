package singbox

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
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
				content := `{
				  "log": {"disabled": false, "level": "info"},
				  "inbounds": [{"type": "vless", "tag": "in-1", "listen": "::", "port": 8443}],
				  "outbounds": [{"type": "direct", "tag": "direct"}]
				}`
				require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
				return path
			},
			wantErr: false,
		},
		{
			name: "empty path",
			setup: func(t *testing.T) string {
				t.Helper()
				return ""
			},
			wantErr:     true,
			errContains: "invalid sing-box configuration",
		},
		{
			name: "invalid json",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				path := filepath.Join(dir, "config.json")
				require.NoError(t, os.WriteFile(path, []byte(`{"inbounds": [`), 0o644))
				return path
			},
			wantErr:     true,
			errContains: "parse failed",
		},
		{
			name: "file not found",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				return filepath.Join(dir, "missing.json")
			},
			wantErr:     true,
			errContains: "read failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t)
			cfg, err := LoadConfig(path)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.ErrorContains(t, err, tt.errContains)
				}
				assert.Nil(t, cfg)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, cfg)
			require.NotNil(t, cfg.Log)
			assert.Equal(t, "info", cfg.Log.Level)
			require.Len(t, cfg.Inbounds, 1)
			assert.Equal(t, "in-1", cfg.Inbounds[0].Tag)
			require.Len(t, cfg.Outbounds, 1)
			assert.Equal(t, "direct", cfg.Outbounds[0].Tag)
		})
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *Config
		wantErr     bool
		errContains string
	}{
		{
			name:        "nil config",
			cfg:         nil,
			wantErr:     true,
			errContains: "config is nil",
		},
		{
			name:    "valid config",
			cfg:     &Config{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.ErrorContains(t, err, tt.errContains)
				}
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestConfigHashComprehensive(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *Config
		wantErr     bool
		errContains string
	}{
		{
			name:        "nil config",
			cfg:         nil,
			wantErr:     true,
			errContains: "cannot hash nil config",
		},
		{
			name: "valid config",
			cfg: &Config{
				Inbounds: []Inbound{{Type: "vless", Tag: "in-1"}},
				Outbounds: []Outbound{{
					Type: "direct",
					Tag:  "direct",
				}},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, err := tt.cfg.Hash()
			if tt.wantErr {
				require.Error(t, err)
				assert.Empty(t, h)
				if tt.errContains != "" {
					assert.ErrorContains(t, err, tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			assert.Len(t, h, 64)
		})
	}

	t.Run("deterministic hash", func(t *testing.T) {
		cfg := &Config{
			Inbounds: []Inbound{{Type: "vless", Tag: "stable"}},
			Outbounds: []Outbound{{
				Type: "direct",
				Tag:  "direct",
			}},
		}

		h1, err := cfg.Hash()
		require.NoError(t, err)
		h2, err := cfg.Hash()
		require.NoError(t, err)
		assert.Equal(t, h1, h2)
	})
}

func TestConfigToJSONComprehensive(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *Config
		wantErr     bool
		errContains string
	}{
		{
			name:        "nil config",
			cfg:         nil,
			wantErr:     true,
			errContains: "cannot marshal nil config",
		},
		{
			name: "valid config",
			cfg: &Config{
				Log:      &LogConfig{Disabled: false, Level: "info"},
				Inbounds: []Inbound{{Type: "vless", Tag: "in-1"}},
				Outbounds: []Outbound{{
					Type: "direct",
					Tag:  "direct",
				}},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := tt.cfg.ToJSON()
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, data)
				if tt.errContains != "" {
					assert.ErrorContains(t, err, tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotEmpty(t, data)
			assert.Contains(t, string(data), "\n  \"log\":")
			assert.True(t, strings.HasPrefix(string(data), "{"))

			var parsed map[string]interface{}
			require.NoError(t, json.Unmarshal(data, &parsed))
			assert.Contains(t, parsed, "log")
		})
	}
}

func TestInboundMarshalJSON(t *testing.T) {
	tests := []struct {
		name                string
		inbound             Inbound
		wantOptionsField    bool
		wantRawOptionsField bool
	}{
		{
			name: "without options",
			inbound: Inbound{
				Type: "vless",
				Tag:  "in-1",
			},
			wantOptionsField:    false,
			wantRawOptionsField: false,
		},
		{
			name: "with options",
			inbound: Inbound{
				Type: "vless",
				Tag:  "in-1",
				Options: map[string]interface{}{
					"users": []interface{}{map[string]interface{}{"name": "u1"}},
				},
			},
			wantOptionsField:    true,
			wantRawOptionsField: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := tt.inbound.MarshalJSON()
			require.NoError(t, err)

			var parsed map[string]interface{}
			require.NoError(t, json.Unmarshal(data, &parsed))
			assert.Equal(t, tt.inbound.Type, parsed["type"])
			assert.Equal(t, tt.inbound.Tag, parsed["tag"])

			_, hasOptions := parsed["options"]
			assert.Equal(t, tt.wantOptionsField, hasOptions)
			_, hasRawOptions := parsed["Options"]
			assert.Equal(t, tt.wantRawOptionsField, hasRawOptions)
		})
	}
}

func TestOutboundMarshalJSON(t *testing.T) {
	tests := []struct {
		name             string
		outbound         Outbound
		wantOptionsField bool
	}{
		{
			name: "without options",
			outbound: Outbound{
				Type: "direct",
				Tag:  "direct",
			},
			wantOptionsField: false,
		},
		{
			name: "with options",
			outbound: Outbound{
				Type: "selector",
				Tag:  "sel",
				Options: map[string]interface{}{
					"outbounds": []interface{}{"direct", "proxy"},
				},
			},
			wantOptionsField: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := tt.outbound.MarshalJSON()
			require.NoError(t, err)

			var parsed map[string]interface{}
			require.NoError(t, json.Unmarshal(data, &parsed))
			assert.Equal(t, tt.outbound.Type, parsed["type"])

			_, hasOptions := parsed["options"]
			assert.Equal(t, tt.wantOptionsField, hasOptions)
			_, hasRawOptions := parsed["Options"]
			assert.False(t, hasRawOptions)
		})
	}
}

func TestLoadConfig_InvalidPath(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		wantErr     bool
		errContains string
	}{
		{
			name:        "invalid path with special characters",
			path:        "/path/with/\x00/null/byte",
			wantErr:     true,
			errContains: "read failed",
		},
		{
			name:        "path too long",
			path:        string(make([]byte, 10000)),
			wantErr:     true,
			errContains: "read failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := LoadConfig(tt.path)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.ErrorContains(t, err, tt.errContains)
				}
				assert.Nil(t, cfg)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, cfg)
		})
	}
}

func TestConfigHash_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *Config
		wantErr     bool
		errContains string
	}{
		{
			name: "config with complex nested structures",
			cfg: &Config{
				Log: &LogConfig{
					Disabled: false,
					Level:    "debug",
					Output:   "/var/log/sing-box.log",
				},
				DNS: &DNSConfig{
					Servers: []DNSServer{
						{Address: "8.8.8.8", Tag: "google"},
						{Address: "1.1.1.1", Tag: "cloudflare"},
					},
					Rules: []DNSRule{
						{QueryType: []string{"A", "AAAA"}, Server: "google"},
					},
				},
				Inbounds: []Inbound{
					{Type: "vless", Tag: "in-1", Listen: "::", Port: 8443},
					{Type: "vmess", Tag: "in-2", Listen: "0.0.0.0", Port: 1080},
				},
				Outbounds: []Outbound{
					{Type: "direct", Tag: "direct"},
					{Type: "block", Tag: "block"},
				},
				Route: &RouteConfig{
					Rules: []RouteRule{
						{Inbound: []string{"in-1"}, Outbound: "direct"},
						{Inbound: []string{"in-2"}, Outbound: "block"},
					},
					Final: "direct",
				},
				Experimental: &ExperimentalConfig{
					CacheFile: &CacheFileConfig{
						Path:  "/var/cache/sing-box/cache.db",
						Store: true,
					},
				},
			},
			wantErr: false,
		},
		{
			name:    "empty config",
			cfg:     &Config{},
			wantErr: false,
		},
		{
			name: "config with only log",
			cfg: &Config{
				Log: &LogConfig{Disabled: true},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, err := tt.cfg.Hash()
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.ErrorContains(t, err, tt.errContains)
				}
				assert.Empty(t, h)
				return
			}
			require.NoError(t, err)
			assert.Len(t, h, 64)
		})
	}
}
