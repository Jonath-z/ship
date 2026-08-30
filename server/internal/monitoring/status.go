// Package monitoring reports the live state of the Ship control plane and the
// machine view available to the API process.
package monitoring

import (
	"context"
	"os"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"

	"github.com/Jonath-z/ship/server/internal/platform/buildinfo"
	"github.com/Jonath-z/ship/server/internal/platform/config"
)

const workerStaleAfter = 15 * time.Second

type Check func(context.Context) error
type LastSeen func(context.Context) (time.Time, error)

type Dependencies struct {
	Postgres       Check
	Redis          Check
	WorkerLastSeen LastSeen
}

type Collector struct {
	configuration Configuration
	dependencies  Dependencies
	startedAt     time.Time
}

type Snapshot struct {
	Status        string                     `json:"status"`
	CollectedAt   time.Time                  `json:"collectedAt"`
	Components    map[string]ComponentStatus `json:"components"`
	Configuration Configuration              `json:"configuration"`
	Machine       Machine                    `json:"machine"`
	Runtime       Runtime                    `json:"runtime"`
	Warnings      []string                   `json:"warnings"`
}

type ComponentStatus struct {
	Status     string     `json:"status"`
	Detail     string     `json:"detail"`
	LastSeenAt *time.Time `json:"lastSeenAt,omitempty"`
}

type Configuration struct {
	Environment           string `json:"environment"`
	Hostname              string `json:"hostname"`
	PublicURL             string `json:"publicUrl"`
	SecureTransport       bool   `json:"secureTransport"`
	InsecureBootstrap     bool   `json:"insecureBootstrap"`
	APIAddress            string `json:"apiAddress"`
	WorkerAddress         string `json:"workerAddress"`
	DataDirectory         string `json:"dataDirectory"`
	LogLevel              string `json:"logLevel"`
	MigrationsOnStart     bool   `json:"migrationsOnStart"`
	SensitiveValuesHidden bool   `json:"sensitiveValuesHidden"`
}

type Machine struct {
	Scope                string `json:"scope"`
	Hostname             string `json:"hostname"`
	OperatingSystem      string `json:"operatingSystem"`
	Platform             string `json:"platform"`
	PlatformVersion      string `json:"platformVersion"`
	KernelVersion        string `json:"kernelVersion"`
	Architecture         string `json:"architecture"`
	VirtualizationSystem string `json:"virtualizationSystem"`
	VirtualizationRole   string `json:"virtualizationRole"`
	UptimeSeconds        uint64 `json:"uptimeSeconds"`
	CPU                  CPU    `json:"cpu"`
	Memory               Memory `json:"memory"`
	Disk                 Disk   `json:"disk"`
}

type CPU struct {
	LogicalCores  int     `json:"logicalCores"`
	PhysicalCores int     `json:"physicalCores"`
	UsedPercent   float64 `json:"usedPercent"`
	Load1         float64 `json:"load1"`
	Load5         float64 `json:"load5"`
	Load15        float64 `json:"load15"`
}

type Memory struct {
	TotalBytes     uint64  `json:"totalBytes"`
	AvailableBytes uint64  `json:"availableBytes"`
	UsedBytes      uint64  `json:"usedBytes"`
	UsedPercent    float64 `json:"usedPercent"`
}

type Disk struct {
	Path        string  `json:"path"`
	TotalBytes  uint64  `json:"totalBytes"`
	FreeBytes   uint64  `json:"freeBytes"`
	UsedBytes   uint64  `json:"usedBytes"`
	UsedPercent float64 `json:"usedPercent"`
}

type Runtime struct {
	GoVersion          string `json:"goVersion"`
	GoOS               string `json:"goOs"`
	GoArchitecture     string `json:"goArchitecture"`
	ProcessID          int    `json:"processId"`
	UptimeSeconds      uint64 `json:"uptimeSeconds"`
	Goroutines         int    `json:"goroutines"`
	HeapAllocatedBytes uint64 `json:"heapAllocatedBytes"`
	SystemMemoryBytes  uint64 `json:"systemMemoryBytes"`
	BuildSHA           string `json:"buildSha"`
	Version            string `json:"version"`
}

func New(cfg config.Config, dependencies Dependencies) *Collector {
	return &Collector{
		configuration: Configuration{
			Environment:           cfg.Environment,
			Hostname:              cfg.Hostname,
			PublicURL:             cfg.PublicURL,
			SecureTransport:       cfg.SecureCookies(),
			InsecureBootstrap:     !cfg.SecureCookies() && cfg.AllowInsecureHTTP,
			APIAddress:            cfg.APIAddr,
			WorkerAddress:         cfg.WorkerAddr,
			DataDirectory:         cfg.DataDir,
			LogLevel:              cfg.LogLevel,
			MigrationsOnStart:     cfg.RunMigrations,
			SensitiveValuesHidden: true,
		},
		dependencies: dependencies,
		startedAt:    time.Now().UTC(),
	}
}

func (collector *Collector) Collect(ctx context.Context) Snapshot {
	now := time.Now().UTC()
	components := map[string]ComponentStatus{
		"api": {
			Status: "ok",
			Detail: "Ship API is responding",
		},
		"postgres": check(ctx, collector.dependencies.Postgres, "PostgreSQL is reachable", "PostgreSQL is unavailable"),
		"redis":    check(ctx, collector.dependencies.Redis, "Redis is reachable", "Redis is unavailable"),
		"worker":   workerStatus(ctx, now, collector.dependencies.WorkerLastSeen),
	}

	machine, warnings := collectMachine(ctx)
	if collector.configuration.InsecureBootstrap {
		warnings = append(warnings, "Ship is using insecure HTTP bootstrap access; configure external HTTPS before production use")
	}
	status := "ok"
	for _, component := range components {
		if component.Status != "ok" {
			status = "degraded"
			break
		}
	}

	var memoryStats runtime.MemStats
	runtime.ReadMemStats(&memoryStats)

	return Snapshot{
		Status:        status,
		CollectedAt:   now,
		Components:    components,
		Configuration: collector.configuration,
		Machine:       machine,
		Runtime: Runtime{
			GoVersion:          runtime.Version(),
			GoOS:               runtime.GOOS,
			GoArchitecture:     runtime.GOARCH,
			ProcessID:          os.Getpid(),
			UptimeSeconds:      uint64(now.Sub(collector.startedAt).Seconds()),
			Goroutines:         runtime.NumGoroutine(),
			HeapAllocatedBytes: memoryStats.HeapAlloc,
			SystemMemoryBytes:  memoryStats.Sys,
			BuildSHA:           buildinfo.SHA,
			Version:            buildinfo.Version,
		},
		Warnings: warnings,
	}
}

func check(ctx context.Context, check Check, successDetail, failureDetail string) ComponentStatus {
	if check == nil || check(ctx) != nil {
		return ComponentStatus{Status: "error", Detail: failureDetail}
	}
	return ComponentStatus{Status: "ok", Detail: successDetail}
}

func workerStatus(ctx context.Context, now time.Time, lastSeen LastSeen) ComponentStatus {
	if lastSeen == nil {
		return ComponentStatus{Status: "unknown", Detail: "Worker heartbeat is unavailable"}
	}
	at, err := lastSeen(ctx)
	if err != nil {
		return ComponentStatus{Status: "unknown", Detail: "Worker heartbeat has not been received"}
	}
	at = at.UTC()
	if now.Sub(at) > workerStaleAfter {
		return ComponentStatus{Status: "error", Detail: "Worker heartbeat is stale", LastSeenAt: &at}
	}
	return ComponentStatus{Status: "ok", Detail: "Worker is sending heartbeats", LastSeenAt: &at}
}

func collectMachine(ctx context.Context) (Machine, []string) {
	machine := Machine{
		Scope:        "api-container",
		Architecture: runtime.GOARCH,
		Disk:         Disk{Path: "/"},
	}
	warnings := make([]string, 0)

	if info, err := host.InfoWithContext(ctx); err == nil {
		machine.Hostname = info.Hostname
		machine.OperatingSystem = info.OS
		machine.Platform = info.Platform
		machine.PlatformVersion = info.PlatformVersion
		machine.KernelVersion = info.KernelVersion
		machine.VirtualizationSystem = info.VirtualizationSystem
		machine.VirtualizationRole = info.VirtualizationRole
		machine.UptimeSeconds = info.Uptime
	} else {
		warnings = append(warnings, "Host information is unavailable")
	}

	if logical, err := cpu.CountsWithContext(ctx, true); err == nil {
		machine.CPU.LogicalCores = logical
	} else {
		warnings = append(warnings, "Logical CPU count is unavailable")
	}
	if physical, err := cpu.CountsWithContext(ctx, false); err == nil {
		machine.CPU.PhysicalCores = physical
	} else {
		warnings = append(warnings, "Physical CPU count is unavailable")
	}
	if percentages, err := cpu.PercentWithContext(ctx, 200*time.Millisecond, false); err == nil && len(percentages) == 1 {
		machine.CPU.UsedPercent = percentages[0]
	} else {
		warnings = append(warnings, "CPU utilization is unavailable")
	}
	if average, err := load.AvgWithContext(ctx); err == nil {
		machine.CPU.Load1 = average.Load1
		machine.CPU.Load5 = average.Load5
		machine.CPU.Load15 = average.Load15
	} else {
		warnings = append(warnings, "CPU load averages are unavailable")
	}

	if memory, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		machine.Memory = Memory{
			TotalBytes:     memory.Total,
			AvailableBytes: memory.Available,
			UsedBytes:      memory.Used,
			UsedPercent:    memory.UsedPercent,
		}
	} else {
		warnings = append(warnings, "Memory information is unavailable")
	}

	if usage, err := disk.UsageWithContext(ctx, "/"); err == nil {
		machine.Disk = Disk{
			Path:        usage.Path,
			TotalBytes:  usage.Total,
			FreeBytes:   usage.Free,
			UsedBytes:   usage.Used,
			UsedPercent: usage.UsedPercent,
		}
	} else {
		warnings = append(warnings, "Disk information is unavailable")
	}

	return machine, warnings
}
