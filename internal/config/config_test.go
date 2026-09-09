package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaults(t *testing.T) {
	cfg := Defaults()

	assert.Equal(t, 8080, cfg.APIPort, "Default APIPort should be 8080")
	assert.Equal(t, 9090, cfg.MetricsPort, "Default MetricsPort should be 9090")
	assert.Equal(t, "/etc/sing-box/config.json", cfg.SingBoxConfigPath, "Default SingBoxConfigPath should be /etc/sing-box/config.json")
	assert.Equal(t, "info", cfg.LogLevel, "Default LogLevel should be info")
}

func TestConfigString_Redaction(t *testing.T) {
	cfg := &Config{
		APIPort:           8080,
		MetricsPort:       9090,
		Token:             "super-secret-token-value",
		Secret:            "super-secret-value",
		SingBoxConfigPath: "/etc/sing-box/config.json",
		LogLevel:          "debug",
	}

	result := cfg.String()

	assert.Contains(t, result, "Token=[REDACTED]", "Token should be redacted")
	assert.Contains(t, result, "Secret=[REDACTED]", "Secret should be redacted")
	assert.NotContains(t, result, "super-secret-token-value", "Token value should not appear")
	assert.NotContains(t, result, "super-secret-value", "Secret value should not appear")
	assert.Contains(t, result, "APIPort=8080", "APIPort should be visible")
	assert.Contains(t, result, "TLS=disabled", "TLS status should be visible when disabled")
}

func TestConfigString_TLSStatus(t *testing.T) {
	cfg := &Config{
		APIPort:     8080,
		MetricsPort: 9090,
		Token:       "12345678901234567890123456789012",
		Secret:      "secret-value",
		TLSCertPath: "/path/to/cert.pem",
		TLSKeyPath:  "/path/to/key.pem",
		LogLevel:    "info",
	}

	result := cfg.String()
	assert.Contains(t, result, "TLS=enabled", "TLS status should be enabled when both cert and key are set")
}

func TestValidate_Success(t *testing.T) {
	tmpDir := t.TempDir()
	singBoxConfig := filepath.Join(tmpDir, "config.json")
	require.NoError(t, os.WriteFile(singBoxConfig, []byte("{}"), 0o600))

	certPath := filepath.Join(tmpDir, "cert.pem")
	keyPath := filepath.Join(tmpDir, "key.pem")
	require.NoError(t, os.WriteFile(certPath, []byte("cert"), 0o600))
	require.NoError(t, os.WriteFile(keyPath, []byte("key"), 0o600))

	cfg := &Config{
		APIPort:           8080,
		MetricsPort:       9090,
		Token:             "12345678901234567890123456789012",
		Secret:            "super-secret-value",
		SingBoxConfigPath: singBoxConfig,
		LogLevel:          "info",
		TLSCertPath:       certPath,
		TLSKeyPath:        keyPath,
	}

	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestValidate_TokenTooShort(t *testing.T) {
	cfg := &Config{
		APIPort:           8080,
		MetricsPort:       9090,
		Token:             "short",
		Secret:            "secret",
		SingBoxConfigPath: "/etc/sing-box/config.json",
		LogLevel:          "info",
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Token must be at least 32 characters")
}

func TestValidate_TokenMissing(t *testing.T) {
	cfg := &Config{
		APIPort:           8080,
		MetricsPort:       9090,
		Token:             "",
		Secret:            "secret",
		SingBoxConfigPath: "/etc/sing-box/config.json",
		LogLevel:          "info",
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Token is required")
}

func TestValidate_SecretMissing(t *testing.T) {
	cfg := &Config{
		APIPort:           8080,
		MetricsPort:       9090,
		Token:             "12345678901234567890123456789012",
		Secret:            "",
		SingBoxConfigPath: "/etc/sing-box/config.json",
		LogLevel:          "info",
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Secret is required")
}

func TestValidate_APIPortOutOfRange(t *testing.T) {
	tests := []struct {
		name  string
		port  int
		valid bool
	}{
		{"port zero", 0, false},
		{"port negative", -1, false},
		{"port too high", 65536, false},
		{"port minimum valid", 1, true},
		{"port maximum valid", 65535, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			singBoxConfig := filepath.Join(tmpDir, "config.json")
			require.NoError(t, os.WriteFile(singBoxConfig, []byte("{}"), 0o600))

			cfg := &Config{
				APIPort:           tt.port,
				MetricsPort:       9090,
				Token:             "12345678901234567890123456789012",
				Secret:            "secret",
				SingBoxConfigPath: singBoxConfig,
				LogLevel:          "info",
			}

			err := cfg.Validate()
			if tt.valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "APIPort must be between 1 and 65535")
			}
		})
	}
}

func TestValidate_MetricsPortOutOfRange(t *testing.T) {
	cfg := &Config{
		APIPort:           8080,
		MetricsPort:       0,
		Token:             "12345678901234567890123456789012",
		Secret:            "secret",
		SingBoxConfigPath: "/etc/sing-box/config.json",
		LogLevel:          "info",
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "MetricsPort must be between 1 and 65535")
}

func TestValidate_PortsConflict(t *testing.T) {
	tmpDir := t.TempDir()
	singBoxConfig := filepath.Join(tmpDir, "config.json")
	require.NoError(t, os.WriteFile(singBoxConfig, []byte("{}"), 0o600))

	cfg := &Config{
		APIPort:           8080,
		MetricsPort:       8080,
		Token:             "12345678901234567890123456789012",
		Secret:            "secret",
		SingBoxConfigPath: singBoxConfig,
		LogLevel:          "info",
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "APIPort and MetricsPort must be different")
}

func TestValidate_LogLevelInvalid(t *testing.T) {
	cfg := &Config{
		APIPort:           8080,
		MetricsPort:       9090,
		Token:             "12345678901234567890123456789012",
		Secret:            "secret",
		SingBoxConfigPath: "/etc/sing-box/config.json",
		LogLevel:          "invalid",
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "LogLevel must be one of: debug, info, warn, error")
}

func TestValidate_LogLevelValid(t *testing.T) {
	tmpDir := t.TempDir()
	singBoxConfig := filepath.Join(tmpDir, "config.json")
	require.NoError(t, os.WriteFile(singBoxConfig, []byte("{}"), 0o600))

	validLevels := []string{"debug", "info", "warn", "error"}

	for _, level := range validLevels {
		t.Run(level, func(t *testing.T) {
			cfg := &Config{
				APIPort:           8080,
				MetricsPort:       9090,
				Token:             "12345678901234567890123456789012",
				Secret:            "secret",
				SingBoxConfigPath: singBoxConfig,
				LogLevel:          level,
			}

			err := cfg.Validate()
			assert.NoError(t, err)
		})
	}
}

func TestValidate_LogLevelCaseInsensitive(t *testing.T) {
	tmpDir := t.TempDir()
	singBoxConfig := filepath.Join(tmpDir, "config.json")
	require.NoError(t, os.WriteFile(singBoxConfig, []byte("{}"), 0o600))

	cfg := &Config{
		APIPort:           8080,
		MetricsPort:       9090,
		Token:             "12345678901234567890123456789012",
		Secret:            "secret",
		SingBoxConfigPath: singBoxConfig,
		LogLevel:          "DEBUG",
	}

	err := cfg.Validate()
	assert.NoError(t, err, "LogLevel should be case-insensitive")
}

func TestValidate_TLSConfiguration(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("both cert and key provided", func(t *testing.T) {
		singBoxConfig := filepath.Join(tmpDir, "config.json")
		certPath := filepath.Join(tmpDir, "cert.pem")
		keyPath := filepath.Join(tmpDir, "key.pem")
		require.NoError(t, os.WriteFile(singBoxConfig, []byte("{}"), 0o600))
		require.NoError(t, os.WriteFile(certPath, []byte("cert"), 0o600))
		require.NoError(t, os.WriteFile(keyPath, []byte("key"), 0o600))

		cfg := &Config{
			APIPort:           8080,
			MetricsPort:       9090,
			Token:             "12345678901234567890123456789012",
			Secret:            "secret",
			SingBoxConfigPath: singBoxConfig,
			LogLevel:          "info",
			TLSCertPath:       certPath,
			TLSKeyPath:        keyPath,
		}

		err := cfg.Validate()
		assert.NoError(t, err)
	})

	t.Run("only cert provided", func(t *testing.T) {
		singBoxConfig := filepath.Join(tmpDir, "config.json")
		require.NoError(t, os.WriteFile(singBoxConfig, []byte("{}"), 0o600))

		cfg := &Config{
			APIPort:           8080,
			MetricsPort:       9090,
			Token:             "12345678901234567890123456789012",
			Secret:            "secret",
			SingBoxConfigPath: singBoxConfig,
			LogLevel:          "info",
			TLSCertPath:       "/path/to/cert.pem",
		}

		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "TLSCertPath and TLSKeyPath must both be specified or both empty")
	})

	t.Run("only key provided", func(t *testing.T) {
		singBoxConfig := filepath.Join(tmpDir, "config.json")
		require.NoError(t, os.WriteFile(singBoxConfig, []byte("{}"), 0o600))

		cfg := &Config{
			APIPort:           8080,
			MetricsPort:       9090,
			Token:             "12345678901234567890123456789012",
			Secret:            "secret",
			SingBoxConfigPath: singBoxConfig,
			LogLevel:          "info",
			TLSKeyPath:        "/path/to/key.pem",
		}

		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "TLSCertPath and TLSKeyPath must both be specified or both empty")
	})

	t.Run("neither cert nor key provided", func(t *testing.T) {
		singBoxConfig := filepath.Join(tmpDir, "config.json")
		require.NoError(t, os.WriteFile(singBoxConfig, []byte("{}"), 0o600))

		cfg := &Config{
			APIPort:           8080,
			MetricsPort:       9090,
			Token:             "12345678901234567890123456789012",
			Secret:            "secret",
			SingBoxConfigPath: singBoxConfig,
			LogLevel:          "info",
		}

		err := cfg.Validate()
		assert.NoError(t, err)
	})
}

func TestValidate_TLSFileNotExist(t *testing.T) {
	tmpDir := t.TempDir()
	singBoxConfig := filepath.Join(tmpDir, "config.json")
	require.NoError(t, os.WriteFile(singBoxConfig, []byte("{}"), 0o600))

	cfg := &Config{
		APIPort:           8080,
		MetricsPort:       9090,
		Token:             "12345678901234567890123456789012",
		Secret:            "secret",
		SingBoxConfigPath: singBoxConfig,
		LogLevel:          "info",
		TLSCertPath:       "/nonexistent/cert.pem",
		TLSKeyPath:        "/nonexistent/key.pem",
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "TLSCertPath file does not exist")
}

func TestValidate_SingBoxConfigPathMissing(t *testing.T) {
	cfg := &Config{
		APIPort:           8080,
		MetricsPort:       9090,
		Token:             "12345678901234567890123456789012",
		Secret:            "secret",
		SingBoxConfigPath: "",
		LogLevel:          "info",
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "SingBoxConfigPath is required")
}

func TestValidate_SingBoxConfigPathNotExist(t *testing.T) {
	cfg := &Config{
		APIPort:           8080,
		MetricsPort:       9090,
		Token:             "12345678901234567890123456789012",
		Secret:            "secret",
		SingBoxConfigPath: "/nonexistent/config.json",
		LogLevel:          "info",
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "SingBoxConfigPath file does not exist")
}

func TestIsProductionReady_Success(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "cert.pem")
	keyPath := filepath.Join(tmpDir, "key.pem")
	require.NoError(t, os.WriteFile(certPath, []byte("cert"), 0o600))
	require.NoError(t, os.WriteFile(keyPath, []byte("key"), 0o600))

	cfg := &Config{
		APIPort:     8080,
		MetricsPort: 9090,
		Token:       "12345678901234567890123456789012",
		Secret:      "secret",
		TLSCertPath: certPath,
		TLSKeyPath:  keyPath,
	}

	err := cfg.IsProductionReady()
	assert.NoError(t, err)
}

func TestIsProductionReady_NoTLS(t *testing.T) {
	cfg := &Config{
		APIPort:     8080,
		MetricsPort: 9090,
		Token:       "12345678901234567890123456789012",
		Secret:      "secret",
	}

	err := cfg.IsProductionReady()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "TLS is required for production")
}

func TestIsProductionReady_TLSFilesNotExist(t *testing.T) {
	cfg := &Config{
		APIPort:     8080,
		MetricsPort: 9090,
		Token:       "12345678901234567890123456789012",
		Secret:      "secret",
		TLSCertPath: "/nonexistent/cert.pem",
		TLSKeyPath:  "/nonexistent/key.pem",
	}

	err := cfg.IsProductionReady()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "TLSCertPath file does not exist")
}

func TestLoad_YAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	yamlContent := `
api_port: 8081
metrics_port: 9091
token: yaml-token-12345678901234567890
secret: yaml-secret
singbox_config_path: /custom/path/config.json
log_level: debug
tls_cert_path: /custom/cert.pem
tls_key_path: /custom/key.pem
`

	require.NoError(t, os.WriteFile(configPath, []byte(yamlContent), 0o600))

	cfg, err := Load(configPath)
	require.NoError(t, err)

	assert.Equal(t, 8081, cfg.APIPort)
	assert.Equal(t, 9091, cfg.MetricsPort)
	assert.Equal(t, "yaml-token-12345678901234567890", cfg.Token)
	assert.Equal(t, "yaml-secret", cfg.Secret)
	assert.Equal(t, "/custom/path/config.json", cfg.SingBoxConfigPath)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.Equal(t, "/custom/cert.pem", cfg.TLSCertPath)
	assert.Equal(t, "/custom/key.pem", cfg.TLSKeyPath)
}

func TestLoad_YAMLWithEnvOverride(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	yamlContent := `
api_port: 8081
token: yaml-token-12345678901234567890
secret: yaml-secret
`

	require.NoError(t, os.WriteFile(configPath, []byte(yamlContent), 0o600))

	t.Setenv("SINGBOX_AGENT_API_PORT", "9999")
	t.Setenv("SINGBOX_AGENT_TOKEN", "env-token-12345678901234567890")
	t.Setenv("SINGBOX_AGENT_SECRET", "env-secret")

	cfg, err := Load(configPath)
	require.NoError(t, err)

	assert.Equal(t, 9999, cfg.APIPort, "Environment variable should override YAML")
	assert.Equal(t, "env-token-12345678901234567890", cfg.Token, "Environment variable should override YAML")
	assert.Equal(t, "env-secret", cfg.Secret, "Environment variable should override YAML")
}

func TestLoad_NoFile(t *testing.T) {
	cfg, err := Load("/nonexistent/config.yaml")
	require.NoError(t, err)

	assert.Equal(t, 8080, cfg.APIPort)
	assert.Equal(t, 9090, cfg.MetricsPort)
	assert.Equal(t, "/etc/sing-box/config.json", cfg.SingBoxConfigPath)
	assert.Equal(t, "info", cfg.LogLevel)
}

func TestLoad_EmptyConfigPath(t *testing.T) {
	cfg, err := Load("")
	require.NoError(t, err)

	assert.Equal(t, 8080, cfg.APIPort)
	assert.Equal(t, 9090, cfg.MetricsPort)
}

func TestLoad_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	invalidYAML := `api_port: [invalid`

	require.NoError(t, os.WriteFile(configPath, []byte(invalidYAML), 0o600))

	_, err := Load(configPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse YAML")
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("SINGBOX_AGENT_API_PORT", "8888")
	t.Setenv("SINGBOX_AGENT_METRICS_PORT", "9999")
	t.Setenv("SINGBOX_AGENT_TOKEN", "env-token-12345678901234567890")
	t.Setenv("SINGBOX_AGENT_SECRET", "env-secret")
	t.Setenv("SINGBOX_AGENT_LOG_LEVEL", "debug")

	cfg, err := LoadFromEnv()
	require.NoError(t, err)

	assert.Equal(t, 8888, cfg.APIPort)
	assert.Equal(t, 9999, cfg.MetricsPort)
	assert.Equal(t, "env-token-12345678901234567890", cfg.Token)
	assert.Equal(t, "env-secret", cfg.Secret)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.Equal(t, "/etc/sing-box/config.json", cfg.SingBoxConfigPath, "Should have default value")
}

func TestLoadFromEnv_NoEnvVars(t *testing.T) {
	cfg, err := LoadFromEnv()
	require.NoError(t, err)

	assert.Equal(t, 8080, cfg.APIPort)
	assert.Equal(t, 9090, cfg.MetricsPort)
	assert.Equal(t, "/etc/sing-box/config.json", cfg.SingBoxConfigPath)
	assert.Equal(t, "info", cfg.LogLevel)
}

func TestLoad_YAMLPrecedenceOverDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	yamlContent := `
api_port: 8081
`

	require.NoError(t, os.WriteFile(configPath, []byte(yamlContent), 0o600))

	cfg, err := Load(configPath)
	require.NoError(t, err)

	assert.Equal(t, 8081, cfg.APIPort, "YAML should override default")
	assert.Equal(t, 9090, cfg.MetricsPort, "Should keep default for fields not in YAML")
}

func TestValidate_MultipleErrors(t *testing.T) {
	cfg := &Config{
		APIPort:           0,
		MetricsPort:       9090,
		Token:             "short",
		Secret:            "",
		SingBoxConfigPath: "",
		LogLevel:          "invalid",
	}

	err := cfg.Validate()
	assert.Error(t, err)

	errMsg := err.Error()
	assert.Contains(t, errMsg, "APIPort must be between 1 and 65535")
	assert.Contains(t, errMsg, "Token must be at least 32 characters")
	assert.Contains(t, errMsg, "Secret is required")
	assert.Contains(t, errMsg, "SingBoxConfigPath is required")
	assert.Contains(t, errMsg, "LogLevel must be one of: debug, info, warn, error")
}
