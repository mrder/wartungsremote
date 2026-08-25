//go:build linux

// Package linux implements platform.Provider for Linux using gopsutil,
// per docs/AGENT.md §9-10.
package linux

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	psnet "github.com/shirou/gopsutil/v3/net"

	"wartungsremote/internal/protocol"
)

var partitionSuffix = regexp.MustCompile(`(p?\d+)$`)

// isRemovableDevice reports whether the block device backing a partition
// (e.g. "/dev/sda1", "/dev/nvme0n1p1") is removable media (USB/SD/optical)
// rather than a fixed internal disk, via /sys/block/<device>/removable.
// Health thresholds only apply to fixed disks — a full USB stick isn't a
// device-health emergency. Unreadable/unrecognized devices fail safe as
// "fixed" (still alerts) rather than silently excluded.
func isRemovableDevice(devicePath string) bool {
	name := strings.TrimPrefix(devicePath, "/dev/")
	if name == "" || name == devicePath {
		return false
	}
	base := partitionSuffix.ReplaceAllString(name, "")
	data, err := os.ReadFile("/sys/block/" + base + "/removable")
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(data)) == "1"
}

type Provider struct {
	AgentVersion string
}

func New(agentVersion string) *Provider {
	return &Provider{AgentVersion: agentVersion}
}

func (p *Provider) Capabilities() []string {
	return []string{"metrics", "inventory", "terminal", "files_read", "files_write", "systemd", "processes", "ssh_tunnel"}
}

func (p *Provider) Inventory(ctx context.Context) (protocol.InventoryResponsePayload, error) {
	hostname, _ := os.Hostname()

	info, err := host.InfoWithContext(ctx)
	if err != nil {
		return protocol.InventoryResponsePayload{}, fmt.Errorf("linux: host info: %w", err)
	}

	cpuInfos, err := cpu.InfoWithContext(ctx)
	cpuModel := "unknown"
	if err == nil && len(cpuInfos) > 0 {
		cpuModel = cpuInfos[0].ModelName
	}
	logicalCores, _ := cpu.CountsWithContext(ctx, true)
	physicalCores, _ := cpu.CountsWithContext(ctx, false)

	vm, err := mem.VirtualMemoryWithContext(ctx)
	var totalMem uint64
	if err == nil {
		totalMem = vm.Total
	}

	disks, err := diskInventory(ctx)
	if err != nil {
		return protocol.InventoryResponsePayload{}, err
	}

	interfaces, err := interfaceInventory(ctx)
	if err != nil {
		return protocol.InventoryResponsePayload{}, err
	}

	bootTime := time.Unix(int64(info.BootTime), 0).UTC()

	return protocol.InventoryResponsePayload{
		Hostname: hostname,
		OS: protocol.OSInfo{
			Family:       "linux",
			Distribution: info.Platform,
			Version:      info.PlatformVersion,
			Kernel:       info.KernelVersion,
		},
		CPU: protocol.CPUInfo{
			Model:   cpuModel,
			Cores:   physicalCores,
			Threads: logicalCores,
		},
		MemoryBytes:   totalMem,
		Disks:         disks,
		Interfaces:    interfaces,
		AgentVersion:  p.AgentVersion,
		BootTime:      &bootTime,
		UptimeSeconds: int64(info.Uptime),
	}, nil
}

func (p *Provider) Metrics(ctx context.Context) (protocol.MetricsReportPayload, error) {
	percents, err := cpu.PercentWithContext(ctx, 0, false)
	var cpuPercent float64
	if err == nil && len(percents) > 0 {
		cpuPercent = percents[0]
	}

	vm, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return protocol.MetricsReportPayload{}, fmt.Errorf("linux: memory: %w", err)
	}

	disks, err := diskUsageOnly(ctx)
	if err != nil {
		return protocol.MetricsReportPayload{}, err
	}

	uptime, _ := host.UptimeWithContext(ctx)

	return protocol.MetricsReportPayload{
		CPUPercent: cpuPercent,
		Memory: protocol.MemoryUsage{
			UsedBytes:  vm.Used,
			TotalBytes: vm.Total,
		},
		Filesystems:   disks,
		UptimeSeconds: int64(uptime),
	}, nil
}

func diskInventory(ctx context.Context) ([]protocol.DiskInfo, error) {
	parts, err := disk.PartitionsWithContext(ctx, false)
	if err != nil {
		return nil, fmt.Errorf("linux: disk partitions: %w", err)
	}
	var out []protocol.DiskInfo
	for _, part := range parts {
		usage, err := disk.UsageWithContext(ctx, part.Mountpoint)
		if err != nil {
			continue
		}
		out = append(out, protocol.DiskInfo{
			Path:       part.Mountpoint,
			Filesystem: part.Fstype,
			UsedBytes:  usage.Used,
			TotalBytes: usage.Total,
			Removable:  isRemovableDevice(part.Device),
		})
	}
	return out, nil
}

func diskUsageOnly(ctx context.Context) ([]protocol.FilesystemUsage, error) {
	parts, err := disk.PartitionsWithContext(ctx, false)
	if err != nil {
		return nil, fmt.Errorf("linux: disk partitions: %w", err)
	}
	var out []protocol.FilesystemUsage
	for _, part := range parts {
		usage, err := disk.UsageWithContext(ctx, part.Mountpoint)
		if err != nil {
			continue
		}
		out = append(out, protocol.FilesystemUsage{
			Path:       part.Mountpoint,
			UsedBytes:  usage.Used,
			TotalBytes: usage.Total,
			Removable:  isRemovableDevice(part.Device),
		})
	}
	return out, nil
}

func interfaceInventory(ctx context.Context) ([]protocol.InterfaceInfo, error) {
	ifaces, err := psnet.InterfacesWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("linux: interfaces: %w", err)
	}
	var out []protocol.InterfaceInfo
	for _, iface := range ifaces {
		info := protocol.InterfaceInfo{Name: iface.Name, MACAddress: iface.HardwareAddr}
		for _, addr := range iface.Addrs {
			ip := addr.Addr
			if isIPv6(ip) {
				info.IPv6 = append(info.IPv6, ip)
			} else {
				info.IPv4 = append(info.IPv4, ip)
			}
		}
		out = append(out, info)
	}
	return out, nil
}

func isIPv6(cidr string) bool {
	for _, c := range cidr {
		if c == ':' {
			return true
		}
		if c == '/' {
			break
		}
	}
	return false
}
