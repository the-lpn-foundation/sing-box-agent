package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/the-lpn-foundation/sing-box-agent/internal/cfglock"
	"github.com/the-lpn-foundation/sing-box-agent/internal/models"
)

type ConfigManager struct {
	configPath string
	reloader   Reloader
	mu         sync.RWMutex
}

// NewConfigManager creates a config manager that reloads sing-box through the
// provided Reloader. If r is nil, Reload() returns an error until
// a reloader is injected via WithReloader.
func NewConfigManager(configPath string, r Reloader) *ConfigManager {
	return &ConfigManager{configPath: configPath, reloader: r}
}

// NewConfigManagerWithReloader creates a config manager that delegates
// Reload() to the supplied Reloader (systemctl/signal/command/etc.).
func NewConfigManagerWithReloader(configPath string, r Reloader) *ConfigManager {
	return &ConfigManager{configPath: configPath, reloader: r}
}

// WithReloader swaps the reloader at runtime (primarily for tests).
func (m *ConfigManager) WithReloader(r Reloader) *ConfigManager {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reloader = r
	return m
}

func (m *ConfigManager) AddUser(inboundTag string, user models.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Serialise the load-modify-save cycle against the other config writer
	// (ConfigClient) to prevent lost updates on the shared file.
	cfglock.For(m.configPath).Lock()
	defer cfglock.For(m.configPath).Unlock()

	cfg, err := m.loadConfigMap()
	if err != nil {
		return err
	}

	inbound, err := findInbound(cfg, inboundTag)
	if err != nil {
		return err
	}

	users, setUsers, err := usersAccessor(inbound, true)
	if err != nil {
		return err
	}

	for _, existing := range users {
		existingMap, ok := existing.(map[string]interface{})
		if !ok {
			continue
		}
		if userMapMatchesSubID(existingMap, user.SubID) {
			return fmt.Errorf("user already exists: %s", user.SubID)
		}
	}

	userMap, err := modelToMap(user)
	if err != nil {
		return fmt.Errorf("marshal user: %w", err)
	}

	users = append(users, userMap)
	setUsers(users)

	return m.saveConfigMap(cfg)
}

func (m *ConfigManager) UpdateUser(inboundTag string, user models.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfglock.For(m.configPath).Lock()
	defer cfglock.For(m.configPath).Unlock()

	cfg, err := m.loadConfigMap()
	if err != nil {
		return err
	}

	inbound, err := findInbound(cfg, inboundTag)
	if err != nil {
		return err
	}

	users, setUsers, err := usersAccessor(inbound, false)
	if err != nil {
		return err
	}

	userMap, err := modelToMap(user)
	if err != nil {
		return fmt.Errorf("marshal user: %w", err)
	}

	updated := false
	for i, existing := range users {
		existingMap, ok := existing.(map[string]interface{})
		if !ok {
			continue
		}
		if userMapMatchesSubID(existingMap, user.SubID) {
			users[i] = userMap
			updated = true
			break
		}
	}

	if !updated {
		return fmt.Errorf("user not found: %s", user.SubID)
	}

	setUsers(users)

	return m.saveConfigMap(cfg)
}

func (m *ConfigManager) DeleteUser(inboundTag, subID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfglock.For(m.configPath).Lock()
	defer cfglock.For(m.configPath).Unlock()

	cfg, err := m.loadConfigMap()
	if err != nil {
		return err
	}

	inbound, err := findInbound(cfg, inboundTag)
	if err != nil {
		return err
	}

	users, setUsers, err := usersAccessor(inbound, false)
	if err != nil {
		return err
	}

	filtered := make([]interface{}, 0, len(users))
	removed := false
	for _, entry := range users {
		entryMap, ok := entry.(map[string]interface{})
		if !ok {
			filtered = append(filtered, entry)
			continue
		}
		if userMapMatchesSubID(entryMap, subID) {
			removed = true
			continue
		}
		filtered = append(filtered, entry)
	}

	if !removed {
		return fmt.Errorf("user not found: %s", subID)
	}

	setUsers(filtered)

	return m.saveConfigMap(cfg)
}

func (m *ConfigManager) AddInbound(inbound models.Inbound) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfglock.For(m.configPath).Lock()
	defer cfglock.For(m.configPath).Unlock()

	cfg, err := m.loadConfigMap()
	if err != nil {
		return err
	}

	inbounds, err := inboundsSlice(cfg, true)
	if err != nil {
		return err
	}

	for _, existing := range inbounds {
		existingMap, ok := existing.(map[string]interface{})
		if !ok {
			continue
		}
		if tag, _ := existingMap["tag"].(string); tag == inbound.Tag {
			return fmt.Errorf("inbound already exists: %s", inbound.Tag)
		}
	}

	inboundMap, err := modelToMap(inbound)
	if err != nil {
		return fmt.Errorf("marshal inbound: %w", err)
	}

	cfg["inbounds"] = append(inbounds, inboundMap)

	return m.saveConfigMap(cfg)
}

func (m *ConfigManager) UpdateInbound(inbound models.Inbound) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfglock.For(m.configPath).Lock()
	defer cfglock.For(m.configPath).Unlock()

	cfg, err := m.loadConfigMap()
	if err != nil {
		return err
	}

	inbounds, err := inboundsSlice(cfg, false)
	if err != nil {
		return err
	}

	inboundMap, err := modelToMap(inbound)
	if err != nil {
		return fmt.Errorf("marshal inbound: %w", err)
	}

	updated := false
	for i, existing := range inbounds {
		existingMap, ok := existing.(map[string]interface{})
		if !ok {
			continue
		}
		if tag, _ := existingMap["tag"].(string); tag == inbound.Tag {
			preserveUsers(existingMap, inboundMap)
			inbounds[i] = inboundMap
			updated = true
			break
		}
	}

	if !updated {
		return fmt.Errorf("inbound not found: %s", inbound.Tag)
	}

	cfg["inbounds"] = inbounds

	return m.saveConfigMap(cfg)
}

func (m *ConfigManager) DeleteInbound(tag string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfglock.For(m.configPath).Lock()
	defer cfglock.For(m.configPath).Unlock()

	cfg, err := m.loadConfigMap()
	if err != nil {
		return err
	}

	inbounds, err := inboundsSlice(cfg, false)
	if err != nil {
		return err
	}

	filtered := make([]interface{}, 0, len(inbounds))
	deleted := false
	for _, existing := range inbounds {
		existingMap, ok := existing.(map[string]interface{})
		if !ok {
			filtered = append(filtered, existing)
			continue
		}
		if existingTag, _ := existingMap["tag"].(string); existingTag == tag {
			deleted = true
			continue
		}
		filtered = append(filtered, existing)
	}

	if !deleted {
		return fmt.Errorf("inbound not found: %s", tag)
	}

	cfg["inbounds"] = filtered

	return m.saveConfigMap(cfg)
}

func (m *ConfigManager) Reload(ctx context.Context) error {
	m.mu.RLock()
	reloader := m.reloader
	m.mu.RUnlock()

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if reloader == nil {
		return fmt.Errorf("sing-box client is not configured")
	}
	return reloader.Reload(ctx)
}

func (m *ConfigManager) loadConfigMap() (map[string]interface{}, error) {
	data, err := os.ReadFile(m.configPath)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if cfg == nil {
		cfg = make(map[string]interface{})
	}

	return cfg, nil
}

func (m *ConfigManager) saveConfigMap(cfg map[string]interface{}) error {
	encoded, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(m.configPath, encoded, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}

func inboundsSlice(cfg map[string]interface{}, create bool) ([]interface{}, error) {
	raw, exists := cfg["inbounds"]
	if !exists {
		if !create {
			return nil, fmt.Errorf("inbounds not found")
		}
		newInbounds := make([]interface{}, 0)
		cfg["inbounds"] = newInbounds
		return newInbounds, nil
	}

	inbounds, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid inbounds format")
	}

	return inbounds, nil
}

func findInbound(cfg map[string]interface{}, tag string) (map[string]interface{}, error) {
	inbounds, err := inboundsSlice(cfg, false)
	if err != nil {
		return nil, err
	}

	for _, candidate := range inbounds {
		inbound, ok := candidate.(map[string]interface{})
		if !ok {
			continue
		}
		if inboundTag, _ := inbound["tag"].(string); inboundTag == tag {
			return inbound, nil
		}
	}

	return nil, fmt.Errorf("inbound not found: %s", tag)
}

func usersAccessor(inbound map[string]interface{}, create bool) ([]interface{}, func([]interface{}), error) {
	if rawUsers, exists := inbound["users"]; exists {
		users, ok := rawUsers.([]interface{})
		if !ok {
			return nil, nil, fmt.Errorf("invalid users format")
		}
		return users, func(updated []interface{}) { inbound["users"] = updated }, nil
	}

	rawOptions, hasOptions := inbound["options"]
	if hasOptions {
		options, ok := rawOptions.(map[string]interface{})
		if !ok {
			return nil, nil, fmt.Errorf("invalid inbound options format")
		}
		if rawUsers, exists := options["users"]; exists {
			users, ok := rawUsers.([]interface{})
			if !ok {
				return nil, nil, fmt.Errorf("invalid options.users format")
			}
			return users, func(updated []interface{}) { options["users"] = updated }, nil
		}
	}

	if !create {
		return nil, nil, fmt.Errorf("users list not found")
	}

	inbound["users"] = make([]interface{}, 0)
	newUsers, ok := inbound["users"].([]interface{})
	if !ok {
		return nil, nil, fmt.Errorf("invalid users format after creation")
	}
	return newUsers, func(updated []interface{}) { inbound["users"] = updated }, nil
}

func userMapMatchesSubID(userMap map[string]interface{}, subID string) bool {
	if value, ok := userMap["subId"].(string); ok && value == subID {
		return true
	}
	if value, ok := userMap["sub_id"].(string); ok && value == subID {
		return true
	}
	if value, ok := userMap["uuid"].(string); ok && value == subID {
		return true
	}
	if value, ok := userMap["name"].(string); ok && value == subID {
		return true
	}
	return false
}

func preserveUsers(source, target map[string]interface{}) {
	if _, exists := target["users"]; !exists {
		if users, ok := source["users"]; ok {
			target["users"] = users
		}
	}

	targetOptions, targetHasOptions := target["options"].(map[string]interface{})
	sourceOptions, sourceHasOptions := source["options"].(map[string]interface{})
	if !sourceHasOptions {
		return
	}

	if !targetHasOptions {
		targetOptions = make(map[string]interface{})
		target["options"] = targetOptions
	}

	if _, exists := targetOptions["users"]; !exists {
		if users, ok := sourceOptions["users"]; ok {
			targetOptions["users"] = users
		}
	}
}

func modelToMap(v interface{}) (map[string]interface{}, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}

	if result == nil {
		result = make(map[string]interface{})
	}

	return result, nil
}
