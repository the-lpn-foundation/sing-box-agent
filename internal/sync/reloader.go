package sync

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Reloader triggers sing-box to pick up a new configuration from disk.
//
// Implementations cover different deployment environments:
//   - SystemctlReloader: host with systemd (production default).
//   - SignalReloader:    container or non-systemd host with a PID file.
//   - CommandReloader:   arbitrary shell command (advanced/custom).
type Reloader interface {
	Reload(ctx context.Context) error
}

// SystemctlReloader restarts a systemd unit (`systemctl restart <service>`).
type SystemctlReloader struct {
	ServiceName string
}

// NewSystemctlReloader returns a reloader that restarts the given unit.
// An empty service name falls back to "sing-box".
func NewSystemctlReloader(service string) *SystemctlReloader {
	if service == "" {
		service = "sing-box"
	}
	return &SystemctlReloader{ServiceName: service}
}

func (r *SystemctlReloader) Reload(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "systemctl", "restart", r.ServiceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("reload sing-box: systemctl restart %s: %s: %w",
			r.ServiceName, strings.TrimSpace(string(output)), err)
	}
	return nil
}

// SignalReloader sends a signal (default SIGHUP) to a process whose PID is in a file.
// Works in Docker containers that run sing-box and the agent side-by-side.
type SignalReloader struct {
	PIDFile string
	Signal  syscall.Signal
}

// NewSignalReloader returns a reloader that signals the process listed in pidFile.
func NewSignalReloader(pidFile string, sig syscall.Signal) *SignalReloader {
	if sig == 0 {
		sig = syscall.SIGHUP
	}
	return &SignalReloader{PIDFile: pidFile, Signal: sig}
}

func (r *SignalReloader) Reload(_ context.Context) error {
	if r.PIDFile == "" {
		return fmt.Errorf("reload sing-box: PID file path is empty")
	}
	data, err := os.ReadFile(r.PIDFile)
	if err != nil {
		return fmt.Errorf("reload sing-box: read PID file %s: %w", r.PIDFile, err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return fmt.Errorf("reload sing-box: parse PID from %s: %w", r.PIDFile, err)
	}
	if err := syscall.Kill(pid, r.Signal); err != nil {
		return fmt.Errorf("reload sing-box: signal %d to PID %d: %w", r.Signal, pid, err)
	}
	return nil
}

// CommandReloader runs an arbitrary shell command (/bin/sh -c "...") to reload sing-box.
type CommandReloader struct {
	Command string
}

// NewCommandReloader returns a reloader that runs the given shell command.
func NewCommandReloader(command string) *CommandReloader {
	return &CommandReloader{Command: command}
}

func (r *CommandReloader) Reload(ctx context.Context) error {
	if strings.TrimSpace(r.Command) == "" {
		return fmt.Errorf("reload sing-box: command is empty")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", r.Command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("reload sing-box: %q: %s: %w",
			r.Command, strings.TrimSpace(string(output)), err)
	}
	return nil
}
