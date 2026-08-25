//go:build windows

package windows

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"wartungsremote/internal/platform"
)

func (p *Provider) ListServices(ctx context.Context) ([]platform.ServiceInfo, error) {
	m, err := mgr.Connect()
	if err != nil {
		return nil, fmt.Errorf("windows: connect to service manager: %w", err)
	}
	defer m.Disconnect()

	names, err := m.ListServices()
	if err != nil {
		return nil, fmt.Errorf("windows: list services: %w", err)
	}

	out := make([]platform.ServiceInfo, 0, len(names))
	for _, name := range names {
		s, err := m.OpenService(name)
		if err != nil {
			continue // service removed concurrently, or access denied for this one
		}
		status, err := s.Query()
		cfg, cfgErr := s.Config()
		s.Close()
		if err != nil {
			continue
		}
		info := platform.ServiceInfo{Name: name, Status: serviceStatusString(status.State)}
		if cfgErr == nil {
			info.DisplayName = cfg.DisplayName
		}
		out = append(out, info)
	}
	return out, nil
}

func serviceStatusString(state svc.State) string {
	switch state {
	case svc.Running:
		return "running"
	case svc.Stopped:
		return "stopped"
	default:
		return "unknown"
	}
}

func (p *Provider) StartService(ctx context.Context, name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("windows: connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(name)
	if err != nil {
		return fmt.Errorf("windows: open service %q: %w", name, err)
	}
	defer s.Close()

	if err := s.Start(); err != nil {
		return fmt.Errorf("windows: start service %q: %w", name, err)
	}
	return nil
}

func (p *Provider) StopService(ctx context.Context, name string) error {
	return controlService(name, svc.Stop, svc.Stopped)
}

func (p *Provider) RestartService(ctx context.Context, name string) error {
	if err := controlService(name, svc.Stop, svc.Stopped); err != nil {
		return err
	}
	return p.StartService(context.Background(), name)
}

func controlService(name string, cmd svc.Cmd, wantState svc.State) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("windows: connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(name)
	if err != nil {
		return fmt.Errorf("windows: open service %q: %w", name, err)
	}
	defer s.Close()

	status, err := s.Control(cmd)
	if err != nil {
		return fmt.Errorf("windows: control service %q: %w", name, err)
	}

	deadline := time.Now().Add(20 * time.Second)
	for status.State != wantState && time.Now().Before(deadline) {
		time.Sleep(300 * time.Millisecond)
		status, err = s.Query()
		if err != nil {
			return fmt.Errorf("windows: query service %q: %w", name, err)
		}
	}
	return nil
}
