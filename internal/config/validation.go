package config

import (
	"fmt"
	"os"
	"strings"
)

// Validate checks the configuration for validity and returns an error if invalid.
func (c *Config) Validate() error {
	var errors []string

	// Validate Token
	if c.Token == "" {
		errors = append(errors, "Token is required")
	} else if len(c.Token) < 32 {
		errors = append(errors, fmt.Sprintf("Token must be at least 32 characters (got %d)", len(c.Token)))
	}

	// Validate Secret
	if c.Secret == "" {
		errors = append(errors, "Secret is required")
	}

	// Validate APIPort
	if c.APIPort < 1 || c.APIPort > 65535 {
		errors = append(errors, fmt.Sprintf("APIPort must be between 1 and 65535 (got %d)", c.APIPort))
	}

	// Validate MetricsPort
	if c.MetricsPort < 1 || c.MetricsPort > 65535 {
		errors = append(errors, fmt.Sprintf("MetricsPort must be between 1 and 65535 (got %d)", c.MetricsPort))
	}

	// Validate that APIPort and MetricsPort are different
	if c.APIPort == c.MetricsPort {
		errors = append(errors, fmt.Sprintf("APIPort and MetricsPort must be different (both are %d)", c.APIPort))
	}

	// Validate LogLevel
	validLogLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	if !validLogLevels[strings.ToLower(c.LogLevel)] {
		errors = append(errors, fmt.Sprintf("LogLevel must be one of: debug, info, warn, error (got %q)", c.LogLevel))
	}

	// Validate TLS configuration (both or neither)
	hasTLSCert := c.TLSCertPath != ""
	hasTLSKey := c.TLSKeyPath != ""
	if hasTLSCert != hasTLSKey {
		errors = append(errors, "TLSCertPath and TLSKeyPath must both be specified or both empty")
	}

	// Validate TLS file paths if provided
	if hasTLSCert {
		if _, err := os.Stat(c.TLSCertPath); os.IsNotExist(err) {
			errors = append(errors, fmt.Sprintf("TLSCertPath file does not exist: %s", c.TLSCertPath))
		}
	}
	if hasTLSKey {
		if _, err := os.Stat(c.TLSKeyPath); os.IsNotExist(err) {
			errors = append(errors, fmt.Sprintf("TLSKeyPath file does not exist: %s", c.TLSKeyPath))
		}
	}

	// Validate SingBoxConfigPath
	if c.SingBoxConfigPath == "" {
		errors = append(errors, "SingBoxConfigPath is required")
	} else if _, err := os.Stat(c.SingBoxConfigPath); os.IsNotExist(err) {
		// Only error if file doesn't exist - directories are not checked
		errors = append(errors, fmt.Sprintf("SingBoxConfigPath file does not exist: %s", c.SingBoxConfigPath))
	}

	// Return errors
	if len(errors) > 0 {
		return fmt.Errorf("configuration validation failed:\n  - %s", strings.Join(errors, "\n  - "))
	}

	return nil
}

// IsProductionReady checks if the configuration is suitable for production use.
// This verifies that TLS is properly configured.
func (c *Config) IsProductionReady() error {
	if c.TLSCertPath == "" || c.TLSKeyPath == "" {
		return fmt.Errorf("TLS is required for production (TLSCertPath and TLSKeyPath must be specified)")
	}

	if _, err := os.Stat(c.TLSCertPath); os.IsNotExist(err) {
		return fmt.Errorf("TLSCertPath file does not exist: %s", c.TLSCertPath)
	}

	if _, err := os.Stat(c.TLSKeyPath); os.IsNotExist(err) {
		return fmt.Errorf("TLSKeyPath file does not exist: %s", c.TLSKeyPath)
	}

	return nil
}
