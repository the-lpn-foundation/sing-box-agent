package singbox

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config represents a sing-box configuration.
// This is a minimal representation for validation and hashing.
// The actual sing-box config structure is more complex and will be
// fully validated by the sing-box library when it is integrated.
type Config struct {
	// Log configuration
	Log *LogConfig `json:"log,omitempty"`

	// DNS configuration
	DNS *DNSConfig `json:"dns,omitempty"`

	// Inbound configurations
	Inbounds []Inbound `json:"inbounds,omitempty"`

	// Outbound configurations
	Outbounds []Outbound `json:"outbounds,omitempty"`

	// Route configuration
	Route *RouteConfig `json:"route,omitempty"`

	// Experimental features
	Experimental *ExperimentalConfig `json:"experimental,omitempty"`
}

// LogConfig contains logging configuration.
type LogConfig struct {
	Disabled bool   `json:"disabled"`
	Level    string `json:"level,omitempty"`
	Output   string `json:"output,omitempty"`
}

// DNSConfig contains DNS configuration.
type DNSConfig struct {
	Servers []DNSServer `json:"servers,omitempty"`
	Rules   []DNSRule   `json:"rules,omitempty"`
}

// DNSServer represents a DNS server.
type DNSServer struct {
	Address string `json:"address"`
	Tag     string `json:"tag,omitempty"`
}

// DNSRule represents a DNS rule.
type DNSRule struct {
	QueryType []string `json:"query_type,omitempty"`
	Server    string   `json:"server"`
}

// Inbound represents an inbound configuration.
type Inbound struct {
	Type    string                 `json:"type"`
	Tag     string                 `json:"tag"`
	Listen  string                 `json:"listen,omitempty"`
	Port    int                    `json:"port,omitempty"`
	Options map[string]interface{} `json:"-"` // Protocol-specific options
}

// MarshalJSON implements json.Marshaler for Inbound to handle Options.
func (i Inbound) MarshalJSON() ([]byte, error) {
	type inboundWithExtras Inbound
	type rawInbound struct {
		inboundWithExtras
		Options map[string]interface{} `json:"options,omitempty"`
	}
	if len(i.Options) > 0 {
		r := rawInbound{inboundWithExtras: inboundWithExtras(i), Options: i.Options}
		return json.Marshal(r)
	}
	return json.Marshal(inboundWithExtras(i))
}

// Outbound represents an outbound configuration.
type Outbound struct {
	Type    string                 `json:"type"`
	Tag     string                 `json:"tag,omitempty"`
	Options map[string]interface{} `json:"-"` // Protocol-specific options
}

// MarshalJSON implements json.Marshaler for Outbound to handle Options.
func (o Outbound) MarshalJSON() ([]byte, error) {
	type outboundWithExtras Outbound
	type rawOutbound struct {
		outboundWithExtras
		Options map[string]interface{} `json:"options,omitempty"`
	}
	if len(o.Options) > 0 {
		r := rawOutbound{outboundWithExtras: outboundWithExtras(o), Options: o.Options}
		return json.Marshal(r)
	}
	return json.Marshal(outboundWithExtras(o))
}

// RouteConfig contains routing configuration.
type RouteConfig struct {
	Rules      []RouteRule `json:"rules,omitempty"`
	AutoDetect interface{} `json:"auto_detect,omitempty"`
	Final      string      `json:"final,omitempty"`
	AutoRoute  interface{} `json:"auto_route,omitempty"`
}

// RouteRule represents a routing rule.
type RouteRule struct {
	Inbound  []string `json:"inbound,omitempty"`
	Outbound string   `json:"outbound"`
}

// ExperimentalConfig contains experimental features.
type ExperimentalConfig struct {
	CacheFile *CacheFileConfig `json:"cache_file,omitempty"`
}

// CacheFileConfig contains cache file configuration.
type CacheFileConfig struct {
	Path  string `json:"path,omitempty"`
	Store bool   `json:"store,omitempty"`
}

// LoadConfig loads and parses a sing-box configuration file from the given path.
func LoadConfig(path string) (*Config, error) {
	if path == "" {
		return nil, WrapError("load config", ErrConfigInvalid)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, WrapError("load config", fmt.Errorf("invalid path: %w", err))
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, WrapError("load config", fmt.Errorf("read failed: %w", err))
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, WrapError("load config", fmt.Errorf("parse failed: %w", err))
	}

	if err := cfg.Validate(); err != nil {
		return nil, WrapError("load config", err)
	}

	return &cfg, nil
}

// Validate performs basic validation on the configuration.
// Note: This is minimal validation. Full validation will be done
// by the sing-box library when it is integrated.
func (c *Config) Validate() error {
	if c == nil {
		return fmt.Errorf("config is nil")
	}

	return nil
}

// Hash returns a SHA256 hash of the configuration's JSON representation.
// This can be used to detect configuration changes.
func (c *Config) Hash() (string, error) {
	if c == nil {
		return "", fmt.Errorf("cannot hash nil config")
	}

	data, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("marshal failed: %w", err)
	}

	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h), nil
}

// ToJSON converts the configuration to a JSON byte slice.
func (c *Config) ToJSON() ([]byte, error) {
	if c == nil {
		return nil, fmt.Errorf("cannot marshal nil config")
	}
	return json.MarshalIndent(c, "", "  ")
}
