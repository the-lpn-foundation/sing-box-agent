package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds the application configuration.
// It can be loaded from a YAML file and overridden by environment variables.
type Config struct {
	// APIPort is the port for the REST API server.
	// Default: 8080
	// Environment: SINGBOX_AGENT_API_PORT
	APIPort int `yaml:"api_port" env:"SINGBOX_AGENT_API_PORT"`

	// MetricsPort is the port for the Prometheus metrics endpoint.
	// Default: 9090
	// Environment: SINGBOX_AGENT_METRICS_PORT
	MetricsPort int `yaml:"metrics_port" env:"SINGBOX_AGENT_METRICS_PORT"`

	// Token is the authentication bearer token.
	// Minimum: 32 characters
	// Required: true
	// Environment: SINGBOX_AGENT_TOKEN
	Token string `yaml:"token" env:"SINGBOX_AGENT_TOKEN"`

	// Secret is the HMAC signing secret for request authentication.
	// Required: true
	// Environment: SINGBOX_AGENT_SECRET
	Secret string `yaml:"secret" env:"SINGBOX_AGENT_SECRET"`

	// SingBoxConfigPath is the path to the sing-box configuration file.
	// Default: /etc/sing-box/config.json
	// Environment: SINGBOX_AGENT_SINGBOX_CONFIG_PATH
	SingBoxConfigPath string `yaml:"singbox_config_path" env:"SINGBOX_AGENT_SINGBOX_CONFIG_PATH"`

	// LogLevel is the logging level.
	// Valid values: debug, info, warn, error
	// Default: info
	// Environment: SINGBOX_AGENT_LOG_LEVEL
	LogLevel string `yaml:"log_level" env:"SINGBOX_AGENT_LOG_LEVEL"`

	// TLSCertPath is the path to the TLS certificate file.
	// Optional, but required for production TLS.
	// Environment: SINGBOX_AGENT_TLS_CERT_PATH
	TLSCertPath string `yaml:"tls_cert_path" env:"SINGBOX_AGENT_TLS_CERT_PATH"`

	// TLSKeyPath is the path to the TLS private key file.
	// Optional, but required for production TLS.
	// Environment: SINGBOX_AGENT_TLS_KEY_PATH
	TLSKeyPath string `yaml:"tls_key_path" env:"SINGBOX_AGENT_TLS_KEY_PATH"`

	// FastifyBaseURL is the base URL of an optional central control plane.
	// ("Fastify" is the name of the reference implementation — any HTTP
	// service that implements the documented contract will work.)
	// Leave empty to run standalone without any control plane.
	// Environment: SINGBOX_AGENT_FASTIFY_URL
	FastifyBaseURL string `yaml:"fastify_base_url" env:"SINGBOX_AGENT_FASTIFY_URL"`

	// ServerID identifies this agent to the control plane.
	// Required when FastifyBaseURL is set.
	// Environment: SINGBOX_AGENT_SERVER_ID
	ServerID string `yaml:"server_id" env:"SINGBOX_AGENT_SERVER_ID"`

	// ReloadStrategy selects how sing-box is reloaded after a desired-state apply.
	// Valid values: "systemctl" (default), "signal", "command".
	// Environment: SINGBOX_AGENT_RELOAD_STRATEGY
	ReloadStrategy string `yaml:"reload_strategy" env:"SINGBOX_AGENT_RELOAD_STRATEGY"`

	// ReloadTarget identifies the reload target:
	//   - systemctl: systemd unit name (default: "sing-box")
	//   - signal:    path to the sing-box PID file
	//   - command:   unused (see ReloadCommand)
	// Environment: SINGBOX_AGENT_RELOAD_TARGET
	ReloadTarget string `yaml:"reload_target" env:"SINGBOX_AGENT_RELOAD_TARGET"`

	// ReloadCommand is the shell command executed when ReloadStrategy == "command".
	// Environment: SINGBOX_AGENT_RELOAD_COMMAND
	ReloadCommand string `yaml:"reload_command" env:"SINGBOX_AGENT_RELOAD_COMMAND"`
}

// String returns a redacted string representation of the Config.
// Sensitive fields (Token and Secret) are redacted.
func (c *Config) String() string {
	// Determine TLS status
	tlsStatus := "disabled"
	if c.TLSCertPath != "" && c.TLSKeyPath != "" {
		tlsStatus = "enabled"
	}

	return fmt.Sprintf(
		"Config{APIPort=%d, MetricsPort=%d, Token=[REDACTED], Secret=[REDACTED], SingBoxConfigPath=%s, LogLevel=%s, TLS=%s}",
		c.APIPort,
		c.MetricsPort,
		c.SingBoxConfigPath,
		c.LogLevel,
		tlsStatus,
	)
}

// GetEnv retrieves the value of an environment variable.
// It returns the provided default if the variable is not set.
func GetEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// GetEnvInt retrieves the value of an environment variable as an integer.
// It returns the provided default if the variable is not set or invalid.
func GetEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

// GetEnvBool retrieves the value of an environment variable as a boolean.
// It returns false if the variable is not set.
func GetEnvBool(key string) bool {
	return strings.ToLower(os.Getenv(key)) == "true"
}
