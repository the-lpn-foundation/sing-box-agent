package config

import (
	"fmt"
	"os"
	"reflect"

	"gopkg.in/yaml.v3"
)

// DefaultConfigPath is the default path to the configuration file.
const DefaultConfigPath = "/etc/sing-box-agent/config.yaml"

// Defaults returns a Config with default values set.
func Defaults() *Config {
	return &Config{
		APIPort:           8080,
		MetricsPort:       9090,
		SingBoxConfigPath: "/etc/sing-box/config.json",
		LogLevel:          "info",
		ReloadStrategy:    "systemctl",
		ReloadTarget:      "sing-box",
	}
}

// Load loads configuration from a YAML file and applies environment variable overrides.
// If configPath is empty, it uses DefaultConfigPath.
// Precedence: environment variables > YAML file > defaults.
func Load(configPath string) (*Config, error) {
	cfg := Defaults()

	// Load from YAML if file exists
	if configPath == "" {
		configPath = DefaultConfigPath
	}

	if _, err := os.Stat(configPath); err == nil {
		if err := loadFromYAML(cfg, configPath); err != nil {
			return nil, fmt.Errorf("failed to load YAML config from %s: %w", configPath, err)
		}
	}

	// Apply environment variable overrides
	applyEnvOverrides(cfg)

	return cfg, nil
}

// loadFromYAML loads configuration from a YAML file into the provided Config.
func loadFromYAML(cfg *Config, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}

	return nil
}

// applyEnvOverrides applies environment variable overrides to the Config.
// It uses reflection to find struct fields with "env" tags and sets their values.
func applyEnvOverrides(cfg *Config) {
	v := reflect.ValueOf(cfg).Elem()
	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		fieldValue := v.Field(i)

		// Get the env tag
		envTag := field.Tag.Get("env")
		if envTag == "" {
			continue
		}

		// Get the environment variable value
		envValue := os.Getenv(envTag)
		if envValue == "" {
			continue
		}

		// Set the field value based on its type
		switch fieldValue.Kind() {
		case reflect.String:
			fieldValue.SetString(envValue)
		case reflect.Int:
			var intVal int
			if _, err := fmt.Sscanf(envValue, "%d", &intVal); err == nil {
				fieldValue.SetInt(int64(intVal))
			}
		}
	}
}

// LoadFromEnv loads configuration only from environment variables, ignoring any YAML files.
// This is useful for containerized environments where env vars are the primary configuration source.
func LoadFromEnv() (*Config, error) {
	cfg := Defaults()
	applyEnvOverrides(cfg)
	return cfg, nil
}
