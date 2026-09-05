package servers

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	cryptossh "golang.org/x/crypto/ssh"

	shipssh "github.com/Jonath-z/ship/server/internal/ssh"
	"github.com/Jonath-z/ship/server/migrations"
)

// Check is one step of the SH-043 sequence, reported individually so the UI
// can show per-check pass/fail with actionable text.
type Check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

type CheckReport struct {
	ServerID string    `json:"serverId"`
	Status   string    `json:"status"`
	Checks   []Check   `json:"checks"`
	RanAt    time.Time `json:"ranAt"`
}

type PrepareReport struct {
	Log    []string    `json:"log"`
	Report CheckReport `json:"report"`
}

// RunChecks executes the §38 sequence: SSH reachable → Docker present → OS →
// architecture → resources. Every step reports individually; later steps are
// skipped once SSH fails. Results update the server row, and the host key is
// recorded on the first successful connection (trust-on-first-use).
func (service *Service) RunChecks(ctx context.Context, requestContext RequestContext, serverID string) (CheckReport, error) {
	row, err := service.find(ctx, serverID)
	if err != nil {
		return CheckReport{}, err
	}
	signer, target, err := service.connection(ctx, row)
	if err != nil {
		return CheckReport{}, err
	}

	report := CheckReport{ServerID: row.ID, RanAt: time.Now().UTC()}
	values := map[string]any{}

	run := func(operation shipssh.Operation) (shipssh.Result, error) {
		command, err := shipssh.Render(shipssh.Request{Operation: operation})
		if err != nil {
			return shipssh.Result{}, err
		}
		return service.runner.Run(ctx, target, signer, command, nil)
	}

	// Step 1: SSH reachability (piggybacked on the cheapest allowlisted call).
	probe, probeErr := run(shipssh.OperationDiskUsage)
	if probeErr != nil {
		report.Checks = append(report.Checks, Check{Name: "ssh", OK: false, Detail: reachabilityDetail(probeErr)})
		report.Status = "disconnected"
		service.saveCheckOutcome(ctx, requestContext, &row, map[string]any{"status": report.Status}, report)
		return report, nil
	}
	report.Checks = append(report.Checks, Check{Name: "ssh", OK: true, Detail: "connected as " + target.User})
	if row.HostKey == "" && probe.HostKey != "" {
		values["host_key"] = probe.HostKey
	}

	// Step 2: Docker present.
	docker, dockerErr := run(shipssh.OperationDockerVersion)
	dockerOK := dockerErr == nil && docker.ExitCode == 0
	dockerDetail := "docker is not installed; run server preparation"
	if dockerOK {
		dockerDetail = dockerVersionDetail(docker.Stdout)
	}
	report.Checks = append(report.Checks, Check{Name: "docker", OK: dockerOK, Detail: dockerDetail})

	// Step 3: operating system.
	osRelease, osErr := run(shipssh.OperationOSRelease)
	osName := parseOSRelease(osRelease.Stdout)
	osOK := osErr == nil && osRelease.ExitCode == 0 && osName != ""
	if osOK {
		values["os"] = osName
	}
	report.Checks = append(report.Checks, Check{Name: "os", OK: osOK, Detail: osName})

	// Step 4: architecture.
	architecture, archErr := run(shipssh.OperationArchitecture)
	archValue := normalizeArchitecture(strings.TrimSpace(architecture.Stdout))
	archOK := archErr == nil && architecture.ExitCode == 0 && archValue != ""
	if archOK {
		values["architecture"] = archValue
	}
	report.Checks = append(report.Checks, Check{Name: "architecture", OK: archOK, Detail: archValue})

	// Step 5: resources.
	resources, resourcesErr := run(shipssh.OperationResources)
	parsed, parsedOK := parseResources(resources.Stdout)
	resourcesOK := resourcesErr == nil && resources.ExitCode == 0 && parsedOK
	if resourcesOK {
		encoded, _ := json.Marshal(parsed)
		values["resources"] = string(encoded)
	}
	report.Checks = append(report.Checks, Check{Name: "resources", OK: resourcesOK, Detail: resourcesDetail(parsed)})

	report.Status = "connected"
	if !dockerOK {
		report.Status = "degraded"
	}
	values["status"] = report.Status
	service.saveCheckOutcome(ctx, requestContext, &row, values, report)
	return report, nil
}

// Prepare installs Docker over SSH (SH-044) with the collected log returned,
// then re-runs the checks. The install command is a fixed allowlisted script.
func (service *Service) Prepare(ctx context.Context, requestContext RequestContext, serverID string) (PrepareReport, error) {
	row, err := service.find(ctx, serverID)
	if err != nil {
		return PrepareReport{}, err
	}
	signer, target, err := service.connection(ctx, row)
	if err != nil {
		return PrepareReport{}, err
	}
	command, err := shipssh.Render(shipssh.Request{Operation: shipssh.OperationDockerInstall})
	if err != nil {
		return PrepareReport{}, err
	}
	var log []string
	result, err := service.runner.Run(ctx, target, signer, command, func(line string) {
		log = append(log, line)
	})
	if err != nil {
		return PrepareReport{}, fmt.Errorf("prepare server: %w", err)
	}
	if result.ExitCode != 0 {
		log = append(log, strings.Split(strings.TrimSpace(result.Stderr), "\n")...)
		log = append(log, fmt.Sprintf("preparation exited with status %d", result.ExitCode))
	}
	service.record(ctx, requestContext, "server.prepared", row)
	report, err := service.RunChecks(ctx, requestContext, serverID)
	if err != nil {
		return PrepareReport{}, err
	}
	return PrepareReport{Log: log, Report: report}, nil
}

func (service *Service) connection(ctx context.Context, row migrations.Server) (cryptossh.Signer, shipssh.Target, error) {
	if row.SSHKeyID == nil {
		return nil, shipssh.Target{}, ErrKeyNotFound
	}
	signer, err := service.signers.Signer(ctx, *row.SSHKeyID)
	if err != nil {
		return nil, shipssh.Target{}, err
	}
	address := row.IPAddress
	if address == "" {
		address = row.Hostname
	}
	return signer, shipssh.Target{
		Address: address, Port: row.SSHPort, User: row.SSHUser, KnownHostKey: row.HostKey,
	}, nil
}

func (service *Service) saveCheckOutcome(ctx context.Context, requestContext RequestContext, row *migrations.Server, values map[string]any, report CheckReport) {
	values["updated_at"] = time.Now().UTC()
	_ = service.db.WithContext(ctx).Model(row).Updates(values).Error
	service.record(ctx, requestContext, "server.checked", *row)
}

func reachabilityDetail(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func dockerVersionDetail(stdout string) string {
	var version struct {
		Version string `json:"Version"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &version); err == nil && version.Version != "" {
		return "docker " + version.Version
	}
	return "docker present"
}

func parseOSRelease(stdout string) string {
	values := map[string]string{}
	for _, line := range strings.Split(stdout, "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if found {
			values[key] = strings.Trim(value, `"`)
		}
	}
	if values["ID"] == "" {
		return ""
	}
	return strings.TrimSpace(values["ID"] + " " + values["VERSION_ID"])
}

func normalizeArchitecture(value string) string {
	switch value {
	case "x86_64", "amd64":
		return "amd64"
	case "aarch64", "arm64":
		return "arm64"
	case "":
		return ""
	default:
		return value
	}
}

func parseResources(stdout string) (Resources, bool) {
	resources := Resources{}
	for _, line := range strings.Split(stdout, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		switch fields[0] {
		case "cpu":
			resources.CPUCores, _ = strconv.Atoi(fields[1])
		case "memory":
			resources.MemoryBytes, _ = strconv.ParseInt(fields[1], 10, 64)
		case "disk":
			resources.DiskBytes, _ = strconv.ParseInt(fields[1], 10, 64)
		}
	}
	return resources, resources.CPUCores > 0 && resources.MemoryBytes > 0
}

func resourcesDetail(resources Resources) string {
	if resources.CPUCores == 0 {
		return "resources could not be read"
	}
	return fmt.Sprintf("%d CPU · %.1f GB RAM · %.0f GB disk",
		resources.CPUCores,
		float64(resources.MemoryBytes)/(1024*1024*1024),
		float64(resources.DiskBytes)/(1024*1024*1024))
}
