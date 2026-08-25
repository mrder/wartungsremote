//go:build linux

package linux

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"wartungsremote/internal/platform"
)

// serviceNamePattern allowlists systemd unit names. exec.Command already
// never invokes a shell (no injection surface), but this is defense in
// depth against pathological unit names reaching systemctl at all, per
// docs/AGENT.md §14.
var serviceNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_@:.\-]{1,255}$`)

func normalizeServiceName(name string) (string, error) {
	if !serviceNamePattern.MatchString(name) {
		return "", fmt.Errorf("linux: invalid service name %q", name)
	}
	if !strings.HasSuffix(name, ".service") {
		name += ".service"
	}
	return name, nil
}

type systemdUnit struct {
	Unit        string `json:"unit"`
	Load        string `json:"load"`
	Active      string `json:"active"`
	Sub         string `json:"sub"`
	Description string `json:"description"`
}

func (p *Provider) ListServices(ctx context.Context) ([]platform.ServiceInfo, error) {
	cmd := exec.CommandContext(ctx, "systemctl", "list-units", "--type=service", "--all", "--no-pager", "--output=json")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("linux: list services: %w", err)
	}
	var units []systemdUnit
	if err := json.Unmarshal(out, &units); err != nil {
		return nil, fmt.Errorf("linux: parse systemctl output: %w", err)
	}
	result := make([]platform.ServiceInfo, 0, len(units))
	for _, u := range units {
		status := "unknown"
		switch u.Active {
		case "active":
			status = "running"
		case "inactive", "failed":
			status = "stopped"
		}
		result = append(result, platform.ServiceInfo{
			Name:        u.Unit,
			DisplayName: u.Description,
			Status:      status,
		})
	}
	return result, nil
}

func (p *Provider) StartService(ctx context.Context, name string) error {
	return systemctl(ctx, "start", name)
}

func (p *Provider) StopService(ctx context.Context, name string) error {
	return systemctl(ctx, "stop", name)
}

func (p *Provider) RestartService(ctx context.Context, name string) error {
	return systemctl(ctx, "restart", name)
}

func systemctl(ctx context.Context, action, name string) error {
	unit, err := normalizeServiceName(name)
	if err != nil {
		return err
	}
	// exec.Command uses an argument array, never a shell — no command
	// injection surface regardless of unit name content.
	cmd := exec.CommandContext(ctx, "systemctl", action, unit)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("linux: systemctl %s %s: %w: %s", action, unit, err, strings.TrimSpace(string(out)))
	}
	return nil
}
