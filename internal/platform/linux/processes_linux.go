//go:build linux

package linux

import (
	"context"
	"fmt"
	"syscall"

	gopsproc "github.com/shirou/gopsutil/v3/process"

	"wartungsremote/internal/platform"
)

func (p *Provider) ListProcesses(ctx context.Context) ([]platform.ProcessInfo, error) {
	procs, err := gopsproc.ProcessesWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("linux: list processes: %w", err)
	}
	out := make([]platform.ProcessInfo, 0, len(procs))
	for _, proc := range procs {
		name, _ := proc.NameWithContext(ctx)
		cpuPct, _ := proc.CPUPercentWithContext(ctx)
		memInfo, _ := proc.MemoryInfoWithContext(ctx)
		username, _ := proc.UsernameWithContext(ctx)
		createTime, _ := proc.CreateTimeWithContext(ctx)

		var rss uint64
		if memInfo != nil {
			rss = memInfo.RSS
		}
		out = append(out, platform.ProcessInfo{
			PID:        proc.Pid,
			Name:       name,
			CPUPercent: cpuPct,
			MemoryRSS:  rss,
			Username:   username,
			StartTime:  createTime,
		})
	}
	return out, nil
}

// TerminateProcess kills a process only if its observed start time still
// matches what the caller last saw, reducing PID-reuse risk (docs/API.md
// §12: "PID + Prozess-Startzeit/Identity sollen gemeinsam geprüft werden").
func (p *Provider) TerminateProcess(ctx context.Context, pid int32, startTimeUnixMS int64) error {
	proc, err := gopsproc.NewProcessWithContext(ctx, pid)
	if err != nil {
		return fmt.Errorf("linux: process %d not found: %w", pid, err)
	}
	actualStart, err := proc.CreateTimeWithContext(ctx)
	if err != nil {
		return fmt.Errorf("linux: read process %d start time: %w", pid, err)
	}
	if startTimeUnixMS != 0 && actualStart != startTimeUnixMS {
		return fmt.Errorf("linux: process %d identity mismatch (likely PID reuse), refusing to terminate", pid)
	}
	if err := syscall.Kill(int(pid), syscall.SIGTERM); err != nil {
		return fmt.Errorf("linux: terminate process %d: %w", pid, err)
	}
	return nil
}
