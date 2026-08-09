package singbox

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/oglenyaboss/sing-box-agent/internal/models"
)

// Reloader triggers sing-box to pick up a new configuration from disk.
// This is structurally compatible with sync.Reloader (systemctl/signal/command)
// without creating a circular import.
type Reloader interface {
	Reload(ctx context.Context) error
}

// reloadDebounceInterval is the window during which multiple config writes
// are coalesced into a single sing-box reload. This minimises TCP connection
// disruptions when several user CRUD operations arrive in quick succession
// (e.g. ghost-pool refill creating N users at once).
const reloadDebounceInterval = 3 * time.Second

type ConfigClient struct {
	configPath string
	wrapper    *Wrapper
	mu         sync.RWMutex
	lastReload time.Time

	// debounceInterval is the reload debounce window. Defaults to
	// reloadDebounceInterval; overridable for tests via SetDebounceInterval.
	debounceInterval time.Duration

	// reloader, if set, replaces the hardcoded systemctl path so that
	// reload_strategy (systemctl|signal|command) is respected on all code
	// paths, not just /sync/desired-state.
	reloader Reloader

	// Debounce state: coalesces multiple saveAndReload calls within a window
	// into a single sing-box reload, minimising connection disruptions.
	debounceMu       sync.Mutex
	debounceTimer    *time.Timer
	debounceOriginal []byte // config snapshot from before the current debounce window
}

func NewConfigClient(configPath string, wrapper *Wrapper) *ConfigClient {
	return &ConfigClient{
		configPath:       configPath,
		wrapper:          wrapper,
		debounceInterval: reloadDebounceInterval,
	}
}

// SetDebounceInterval overrides the reload debounce window. Tests use this to
// shrink the default 3s window so debounce behaviour can be exercised quickly.
// Returns the receiver for chaining.
func (c *ConfigClient) SetDebounceInterval(d time.Duration) *ConfigClient {
	c.debounceInterval = d
	return c
}

// WithReloader injects a reloader (systemctl/signal/command) to use instead of
// the legacy hardcoded systemctl path. Returns the receiver for chaining.
func (c *ConfigClient) WithReloader(r Reloader) *ConfigClient {
	c.reloader = r
	return c
}

func (c *ConfigClient) GetInbounds(ctx context.Context) ([]models.Inbound, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	root, err := c.loadRootConfig()
	if err != nil {
		return nil, err
	}

	rawInbounds := getInbounds(root)
	result := make([]models.Inbound, 0, len(rawInbounds))
	for _, inbound := range rawInbounds {
		if inbound == nil {
			continue
		}

		m := models.Inbound{
			Tag:    getString(inbound, "tag"),
			Type:   getString(inbound, "type"),
			Listen: getString(inbound, "listen"),
			Port:   getInt(inbound, "listen_port", "port"),
		}

		extra := map[string]interface{}{}
		for k, v := range inbound {
			switch k {
			case "tag", "type", "listen", "listen_port", "port":
			default:
				extra[k] = v
			}
		}
		if len(extra) > 0 {
			m.Options = extra
		}

		result = append(result, m)
	}

	return result, nil
}

func (c *ConfigClient) GetUsers(ctx context.Context) ([]models.User, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	root, err := c.loadRootConfig()
	if err != nil {
		return nil, err
	}

	rawInbounds := getInbounds(root)
	users := make([]models.User, 0)
	for _, inbound := range rawInbounds {
		tag := getString(inbound, "tag")
		for _, rawUser := range getInboundUsers(inbound) {
			userMap, ok := rawUser.(map[string]interface{})
			if !ok {
				continue
			}
			if user := c.userFromMap(userMap, tag); user != nil {
				users = append(users, *user)
			}
		}
	}

	return users, nil
}

func (c *ConfigClient) CreateInbound(ctx context.Context, inbound models.Inbound) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	root, err := c.loadRootConfig()
	if err != nil {
		return err
	}

	rawInbounds := getInbounds(root)
	for _, existing := range rawInbounds {
		if getString(existing, "tag") == inbound.Tag {
			return fmt.Errorf("inbound with tag %s already exists", inbound.Tag)
		}
	}

	newInbound := map[string]interface{}{
		"type": inbound.Type,
		"tag":  inbound.Tag,
	}
	if inbound.Listen != "" {
		newInbound["listen"] = inbound.Listen
	}
	if inbound.Port != 0 {
		newInbound["listen_port"] = inbound.Port
	}
	for k, v := range inbound.Options {
		newInbound[k] = v
	}

	rawInbounds = append(rawInbounds, newInbound)
	setInbounds(root, rawInbounds)

	return c.saveAndReload(ctx, root)
}

func (c *ConfigClient) UpdateInbound(ctx context.Context, inbound models.Inbound) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	root, err := c.loadRootConfig()
	if err != nil {
		return err
	}

	rawInbounds := getInbounds(root)
	idx := findInboundIndex(rawInbounds, inbound.Tag)
	if idx < 0 {
		return fmt.Errorf("inbound with tag %s not found", inbound.Tag)
	}

	existing := rawInbounds[idx]
	existing["type"] = inbound.Type
	if inbound.Listen != "" {
		existing["listen"] = inbound.Listen
	}
	if inbound.Port != 0 {
		existing["listen_port"] = inbound.Port
	}
	for k, v := range inbound.Options {
		existing[k] = v
	}

	rawInbounds[idx] = existing
	setInbounds(root, rawInbounds)

	return c.saveAndReload(ctx, root)
}

func (c *ConfigClient) DeleteInbound(ctx context.Context, tag string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	root, err := c.loadRootConfig()
	if err != nil {
		return err
	}

	rawInbounds := getInbounds(root)
	idx := findInboundIndex(rawInbounds, tag)
	if idx < 0 {
		return fmt.Errorf("inbound with tag %s not found", tag)
	}

	rawInbounds = append(rawInbounds[:idx], rawInbounds[idx+1:]...)
	setInbounds(root, rawInbounds)

	return c.saveAndReload(ctx, root)
}

func (c *ConfigClient) CreateUser(ctx context.Context, user models.User) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	root, err := c.loadRootConfig()
	if err != nil {
		return err
	}

	rawInbounds := getInbounds(root)
	idx := findInboundIndex(rawInbounds, user.InboundTag)
	if idx < 0 {
		return fmt.Errorf("inbound with tag %s not found", user.InboundTag)
	}

	inbound := rawInbounds[idx]
	users := getInboundUsers(inbound)
	for _, existing := range users {
		u, ok := existing.(map[string]interface{})
		if !ok {
			continue
		}
		if userMatchesSubID(u, user.SubID) {
			return fmt.Errorf("user with subID %s already exists", user.SubID)
		}
	}

	newUser := buildProtocolUser(getString(inbound, "type"), user, nil)
	users = append(users, newUser)
	setInboundUsers(inbound, users)

	rawInbounds[idx] = inbound
	setInbounds(root, rawInbounds)

	return c.saveAndReload(ctx, root)
}

func (c *ConfigClient) UpdateUser(ctx context.Context, user models.User) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	root, err := c.loadRootConfig()
	if err != nil {
		return err
	}

	rawInbounds := getInbounds(root)
	idx := findInboundIndex(rawInbounds, user.InboundTag)
	if idx < 0 {
		return fmt.Errorf("inbound with tag %s not found", user.InboundTag)
	}

	inbound := rawInbounds[idx]
	users := getInboundUsers(inbound)
	found := -1
	var existing map[string]interface{}
	for i, existingAny := range users {
		u, ok := existingAny.(map[string]interface{})
		if !ok {
			continue
		}
		if userMatchesSubID(u, user.SubID) {
			found = i
			existing = u
			break
		}
	}
	if found < 0 {
		return fmt.Errorf("user with subID %s not found", user.SubID)
	}

	users[found] = buildProtocolUser(getString(inbound, "type"), user, existing)
	setInboundUsers(inbound, users)

	rawInbounds[idx] = inbound
	setInbounds(root, rawInbounds)

	return c.saveAndReload(ctx, root)
}

func (c *ConfigClient) DeleteUser(ctx context.Context, subID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	root, err := c.loadRootConfig()
	if err != nil {
		return err
	}

	rawInbounds := getInbounds(root)
	removed := false
	for i, inbound := range rawInbounds {
		users := getInboundUsers(inbound)
		if len(users) == 0 {
			continue
		}

		filtered := make([]interface{}, 0, len(users))
		removedHere := false
		for _, existingAny := range users {
			u, ok := existingAny.(map[string]interface{})
			if ok && userMatchesSubID(u, subID) {
				removedHere = true
				removed = true
				continue
			}
			filtered = append(filtered, existingAny)
		}

		if removedHere {
			setInboundUsers(inbound, filtered)
			rawInbounds[i] = inbound
		}
	}

	if !removed {
		return fmt.Errorf("user with subID %s not found", subID)
	}

	setInbounds(root, rawInbounds)
	return c.saveAndReload(ctx, root)
}

func (c *ConfigClient) loadRootConfig() (map[string]interface{}, error) {
	data, err := os.ReadFile(c.configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	root := map[string]interface{}{}
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	if _, ok := root["inbounds"]; !ok {
		root["inbounds"] = []interface{}{}
	}

	return root, nil
}

func getInbounds(root map[string]interface{}) []map[string]interface{} {
	raw, ok := root["inbounds"].([]interface{})
	if !ok {
		return []map[string]interface{}{}
	}

	result := make([]map[string]interface{}, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		result = append(result, m)
	}

	return result
}

func setInbounds(root map[string]interface{}, inbounds []map[string]interface{}) {
	raw := make([]interface{}, 0, len(inbounds))
	for _, inbound := range inbounds {
		raw = append(raw, inbound)
	}
	root["inbounds"] = raw
}

func findInboundIndex(inbounds []map[string]interface{}, tag string) int {
	for i, inbound := range inbounds {
		if getString(inbound, "tag") == tag {
			return i
		}
	}
	return -1
}

func getInboundUsers(inbound map[string]interface{}) []interface{} {
	if raw, ok := inbound["users"].([]interface{}); ok {
		return raw
	}
	if options, ok := inbound["options"].(map[string]interface{}); ok {
		if raw, ok := options["users"].([]interface{}); ok {
			return raw
		}
	}
	return []interface{}{}
}

func setInboundUsers(inbound map[string]interface{}, users []interface{}) {
	if _, ok := inbound["users"]; ok {
		inbound["users"] = users
		return
	}
	if options, ok := inbound["options"].(map[string]interface{}); ok {
		if _, ok := options["users"]; ok {
			options["users"] = users
			inbound["options"] = options
			return
		}
	}
	inbound["users"] = users
}

func userMatchesSubID(userMap map[string]interface{}, subID string) bool {
	for _, k := range []string{"subId", "name", "email", "uuid"} {
		if getString(userMap, k) == subID {
			return true
		}
	}
	return false
}

func buildProtocolUser(inboundType string, user models.User, existing map[string]interface{}) map[string]interface{} {
	inboundType = strings.ToLower(inboundType)

	switch inboundType {
	case "vless", "vmess":
		uuidValue := user.UUID
		if uuidValue == "" {
			uuidValue = getString(existing, "uuid")
		}
		if uuidValue == "" {
			uuidValue = uuid.NewString()
		}
		nameValue := user.SubID
		if nameValue == "" {
			nameValue = getString(existing, "name")
		}

		result := map[string]interface{}{
			"uuid": uuidValue,
		}
		if nameValue != "" {
			result["name"] = nameValue
		}
		if user.Flow != "" {
			result["flow"] = user.Flow
		} else if flow := getString(existing, "flow"); flow != "" {
			result["flow"] = flow
		}
		return result

	case "hysteria2", "shadowtls", "shadowsocks", "trojan", "tuic":
		name := user.SubID
		if name == "" {
			name = getString(existing, "name")
		}
		password := user.UUID
		if password == "" {
			password = getString(existing, "password")
		}
		if password == "" {
			password = uuid.NewString()
		}

		result := map[string]interface{}{
			"name":     name,
			"password": password,
		}
		return result
	}

	result := map[string]interface{}{}
	for k, v := range existing {
		result[k] = v
	}
	if user.SubID != "" {
		result["subId"] = user.SubID
	}
	if user.UUID != "" {
		result["uuid"] = user.UUID
	}
	if user.Email != "" {
		result["email"] = user.Email
	}
	if user.Flow != "" {
		result["flow"] = user.Flow
	}
	return result
}

func (c *ConfigClient) userFromMap(userMap map[string]interface{}, inboundTag string) *models.User {
	user := &models.User{
		InboundTag: inboundTag,
		Enabled:    true,
	}

	user.SubID = firstNonEmpty(
		getString(userMap, "subId"),
		getString(userMap, "name"),
		getString(userMap, "email"),
		getString(userMap, "uuid"),
	)
	user.UUID = firstNonEmpty(getString(userMap, "uuid"), getString(userMap, "password"))
	user.Email = getString(userMap, "email")
	user.Flow = getString(userMap, "flow")

	if enabledRaw, ok := userMap["enabled"]; ok {
		if enabled, ok := enabledRaw.(bool); ok {
			user.Enabled = enabled
		}
	}

	if user.SubID == "" {
		return nil
	}

	return user
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func getString(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

func getInt(m map[string]interface{}, keys ...string) int {
	for _, key := range keys {
		v, ok := m[key]
		if !ok {
			continue
		}
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		case int64:
			return int(n)
		}
	}
	return 0
}

func (c *ConfigClient) saveAndReload(_ context.Context, root map[string]interface{}) error {
	previousConfig, err := os.ReadFile(c.configPath)
	if err != nil {
		return fmt.Errorf("failed to read current config for backup: %w", err)
	}

	_ = os.WriteFile(c.configPath+".agent.bak", previousConfig, 0o600)

	configJSON, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(c.configPath, configJSON, 0o644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	if c.wrapper == nil {
		return nil
	}

	// Debounce the reload: coalesce multiple config writes within a window
	// into a single sing-box reload to minimise TCP connection disruptions.
	// Each call writes the config file immediately (so disk is always current),
	// but the reload is deferred until the debounce window closes.
	c.debounceMu.Lock()
	if c.debounceTimer == nil {
		// First write in this window — capture the pre-window config for rollback.
		c.debounceOriginal = previousConfig
		c.debounceTimer = time.AfterFunc(c.debounceInterval, c.performDebouncedReload)
	} else {
		// Subsequent write in the same window — reset the timer.
		c.debounceTimer.Reset(c.debounceInterval)
	}
	c.debounceMu.Unlock()

	return nil
}

// performDebouncedReload executes the deferred reload after the debounce
// window closes. On failure it rolls back to the pre-window config and retries.
func (c *ConfigClient) performDebouncedReload() {
	c.debounceMu.Lock()
	original := c.debounceOriginal
	c.debounceOriginal = nil
	c.debounceTimer = nil
	c.debounceMu.Unlock()

	if original == nil {
		return
	}

	reloadCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := c.doReload(reloadCtx); err != nil {
		// Rollback to the pre-window config and retry reload.
		if restoreErr := os.WriteFile(c.configPath, original, 0o644); restoreErr != nil {
			fmt.Fprintf(os.Stderr, "reload failed: %v; rollback write failed: %v\n", err, restoreErr)
			return
		}
		_ = c.doReload(reloadCtx)
		fmt.Fprintf(os.Stderr, "reload failed and config rolled back: %v\n", err)
	}
}

// doReload executes the reload using the injected reloader (if configured),
// or falls back to the legacy systemctl path. This ensures reload_strategy
// (systemctl|signal|command) is respected on all code paths.
func (c *ConfigClient) doReload(ctx context.Context) error {
	if c.reloader != nil {
		if err := c.reloader.Reload(ctx); err != nil {
			c.lastReload = time.Now()
			return err
		}
		c.lastReload = time.Now()
		return nil
	}
	return c.reloadSystemService(ctx)
}

func (c *ConfigClient) reloadSystemService(ctx context.Context) error {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return nil
	}

	const minReloadInterval = 2 * time.Second
	if !c.lastReload.IsZero() {
		elapsed := time.Since(c.lastReload)
		if elapsed < minReloadInterval {
			wait := minReloadInterval - elapsed
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
		}
	}

	tryReloadOrRestart := exec.CommandContext(ctx, "systemctl", "try-reload-or-restart", "sing-box")
	if out, err := tryReloadOrRestart.CombinedOutput(); err == nil {
		c.lastReload = time.Now()
		return nil
	} else {
		lastErr := fmt.Errorf("try-reload-or-restart failed (%s): %w", strings.TrimSpace(string(out)), err)
		backoff := []time.Duration{200 * time.Millisecond, 500 * time.Millisecond, time.Second}
		for _, wait := range backoff {
			_ = exec.CommandContext(ctx, "systemctl", "reset-failed", "sing-box").Run()
			restartCmd := exec.CommandContext(ctx, "systemctl", "restart", "sing-box")
			restartOut, restartErr := restartCmd.CombinedOutput()
			if restartErr == nil {
				c.lastReload = time.Now()
				return nil
			}

			lastErr = fmt.Errorf("%v; restart failed (%s): %w", lastErr, strings.TrimSpace(string(restartOut)), restartErr)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
		}

		return lastErr
	}
}
