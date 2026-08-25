//go:build windows

package windows

import (
	"context"
	"fmt"

	gopsproc "github.com/shirou/gopsutil/v3/process"
	wsyscall "golang.org/x/sys/windows"

	"wartungsremote/internal/platform"
)

func (p *Provider) ListProcesses(ctx context.Context) ([]platform.ProcessInfo, error) {
	procs, err := gopsproc.ProcessesWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("windows: list processes: %w", err)
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
// §12).
func (p *Provider) TerminateProcess(ctx context.Context, pid int32, startTimeUnixMS int64) error {
	proc, err := gopsproc.NewProcessWithContext(ctx, pid)
	if err != nil {
		return fmt.Errorf("windows: process %d not found: %w", pid, err)
	}
	actualStart, err := proc.CreateTimeWithContext(ctx)
	if err != nil {
		return fmt.Errorf("windows: read process %d start time: %w", pid, err)
	}
	if startTimeUnixMS != 0 && actualStart != startTimeUnixMS {
		return fmt.Errorf("windows: process %d identity mismatch (likely PID reuse), refusing to terminate", pid)
	}

	handle, err := wsyscall.OpenProcess(wsyscall.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return fmt.Errorf("windows: open process %d: %w", pid, err)
	}
	defer wsyscall.CloseHandle(handle)
	if err := wsyscall.TerminateProcess(handle, 1); err != nil {
		return fmt.Errorf("windows: terminate process %d: %w", pid, err)
	}
	return nil
}
