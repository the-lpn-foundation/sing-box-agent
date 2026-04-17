package singbox

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// CoreServiceAdapter bridges sing-box systemd service and config file to the handlers.CoreService interface.
type CoreServiceAdapter struct {
	wrapper     *Wrapper
	configPath  string
	serviceName string
	pidFile     string
}

// NewCoreServiceAdapter creates a new CoreServiceAdapter.
func NewCoreServiceAdapter(wrapper *Wrapper, configPath string) *CoreServiceAdapter {
	return &CoreServiceAdapter{
		wrapper:     wrapper,
		configPath:  configPath,
		serviceName: "sing-box",
	}
}

// WithPIDFile configures a PID file used for health probing in non-systemd
// environments (e.g. Docker). When set, IsActive() falls back to signalling
// the recorded PID if `systemctl is-active` fails or is unavailable.
func (a *CoreServiceAdapter) WithPIDFile(path string) *CoreServiceAdapter {
	a.pidFile = path
	return a
}

// Reload reloads the sing-box configuration via systemctl.
func (a *CoreServiceAdapter) Reload(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "systemctl", "reload", a.serviceName)
	if err := cmd.Run(); err == nil {
		return nil
	}
	// Fall back to a full restart if the service does not implement reload.
	cmd = exec.CommandContext(ctx, "systemctl", "restart", a.serviceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("reload sing-box: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

// Restart restarts the sing-box service via systemctl.
func (a *CoreServiceAdapter) Restart(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "systemctl", "restart", a.serviceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("restart sing-box: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

// GetConfig returns the current sing-box configuration as a map.
func (a *CoreServiceAdapter) GetConfig(_ context.Context) (map[string]interface{}, error) {
	data, err := os.ReadFile(a.configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return result, nil
}

// IsActive reports whether sing-box is currently running.
//
// Strategy:
//  1. If `systemctl is-active --quiet` succeeds, return true (systemd hosts).
//  2. Otherwise, if a PID file is configured, read it and send signal 0 to
//     verify the process exists (Docker / non-systemd hosts).
//  3. Otherwise, return false.
func (a *CoreServiceAdapter) IsActive() bool {
	cmd := exec.Command("systemctl", "is-active", "--quiet", a.serviceName)
	if err := cmd.Run(); err == nil {
		return true
	}

	if a.pidFile == "" {
		return false
	}
	data, err := os.ReadFile(a.pidFile)
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return false
	}
	// Signal 0 = existence check only.
	return syscall.Kill(pid, 0) == nil
}
