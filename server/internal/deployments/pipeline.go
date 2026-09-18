package deployments

import (
	"context"
	"errors"
	"fmt"
	"time"

	redisclient "github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/Jonath-z/ship/server/internal/configuration"
	"github.com/Jonath-z/ship/server/internal/gitsource"
	"github.com/Jonath-z/ship/server/internal/kamal"
	shipcrypto "github.com/Jonath-z/ship/server/internal/platform/crypto"
	"github.com/Jonath-z/ship/server/internal/platform/jobs"
	"github.com/Jonath-z/ship/server/internal/sshkeys"
	"github.com/Jonath-z/ship/server/migrations"
)

// Runner executes deploy jobs on the worker (SH-073): validate → snapshot →
// render → materialize workspace → build when the service deploys from a
// repository → run the engine → finalize, with cleanup on every exit path.
type Runner struct {
	DB            *gorm.DB
	Redis         *redisclient.Client
	Queue         *jobs.Queue
	Configuration *configuration.Repository
	Vault         *shipcrypto.Vault
	SSHKeys       *sshkeys.Service
	Engine        kamal.DeploymentEngine
	Cloner        gitsource.Cloner // nil means the git CLI
	DataDir       string
}

func (runner *Runner) cloner() gitsource.Cloner {
	if runner.Cloner != nil {
		return runner.Cloner
	}
	return gitsource.CLI{}
}

// Handle processes one deploy job. Infrastructure errors before the first
// transition return an error (the queue retries); anything after marks the
// deployment FAILED visibly and returns nil.
func (runner *Runner) Handle(ctx context.Context, job jobs.Job) error {
	deploymentID := job.Payload
	var deployment migrations.Deployment
	if err := runner.DB.WithContext(ctx).First(&deployment, "id = ?", deploymentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil // deleted underneath us; nothing to do
		}
		return fmt.Errorf("load deployment: %w", err)
	}
	if Status(deployment.Status) != StatusQueued {
		return nil // redelivered after completion or crash recovery
	}

	release, err := runner.acquireLock(ctx, deployment.EnvironmentID, deploymentID)
	if err != nil {
		return err
	}
	defer release()

	log, err := newEventLog(ctx, runner.DB, runner.Redis, deploymentID)
	if err != nil {
		return err
	}
	runner.run(ctx, &deployment, log)
	return nil
}

// MarkStale fails a deployment found in the active queue after a worker
// crash — no run may stay stuck in a non-terminal state (SH-071).
func (runner *Runner) MarkStale(ctx context.Context, job jobs.Job, cause error) {
	var deployment migrations.Deployment
	if err := runner.DB.WithContext(ctx).First(&deployment, "id = ?", job.Payload).Error; err != nil {
		return
	}
	if Status(deployment.Status).Terminal() {
		return
	}
	log, err := newEventLog(ctx, runner.DB, runner.Redis, deployment.ID)
	if err != nil {
		return
	}
	if cause != nil {
		log.line(ctx, "system", "deployment aborted: "+cause.Error())
	} else {
		log.line(ctx, "system", "deployment aborted: the worker restarted mid-run")
	}
	now := time.Now().UTC()
	_ = runner.DB.WithContext(ctx).Model(&deployment).UpdateColumns(map[string]any{
		"status": string(StatusFailed), "finished_at": now, "updated_at": now,
	}).Error
	log.status(ctx, StatusFailed)
}

func (runner *Runner) acquireLock(ctx context.Context, environmentID, token string) (func(), error) {
	deadline := time.Now().Add(10 * time.Minute)
	for {
		release, acquired, err := runner.Queue.AcquireEnvironmentLock(ctx, environmentID, token)
		if err != nil {
			return nil, err
		}
		if acquired {
			return release, nil
		}
		if time.Now().After(deadline) {
			return nil, errors.New("another deployment holds the environment lock")
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
}

func (runner *Runner) run(ctx context.Context, deployment *migrations.Deployment, log *eventLog) {
	fail := func(message string) {
		log.line(ctx, "system", message)
		runner.finish(ctx, deployment, log, StatusFailed)
	}

	if err := runner.transition(ctx, deployment, log, StatusValidating); err != nil {
		fail(err.Error())
		return
	}

	isRollback := deployment.SourceDeploymentID != nil
	state, rendered, version, err := runner.resolveConfiguration(ctx, deployment, log, isRollback)
	if err != nil {
		fail(err.Error())
		return
	}

	serviceName, deployYAML, err := runner.serviceDocument(ctx, deployment, rendered)
	if err != nil {
		fail(err.Error())
		return
	}
	if image := state.Services[serviceName].Image; image != "" {
		_ = runner.DB.WithContext(ctx).Model(deployment).UpdateColumn("image", image).Error
	}

	secrets, err := runner.materializeSecrets(ctx, deployment.EnvironmentID, deployment.ServiceID)
	if err != nil {
		fail("collect secrets: " + err.Error())
		return
	}
	keyPEM, err := runner.deployKey(ctx, deployment.EnvironmentID)
	if err != nil {
		fail(err.Error())
		return
	}
	slugs, err := runner.slugs(ctx, deployment.EnvironmentID)
	if err != nil {
		fail(err.Error())
		return
	}
	workspace := kamal.NewWorkspace(kamal.WorkspaceInput{
		DataDir: runner.DataDir, ProjectSlug: slugs[0], EnvironmentSlug: slugs[1],
		DeploymentID: deployment.ID, DeployYAML: deployYAML, Secrets: secrets, SSHKeyPEM: keyPEM,
	})
	// Secrets (and cloned source) never outlive the run: cleanup happens on
	// every exit path.
	defer workspace.Cleanup()

	// Repository-backed services build here: clone into the workspace root so
	// Kamal sees its expected application layout, then build and push the
	// image tagged with the cloned commit SHA. Rollbacks never rebuild — they
	// reactivate the version recorded on the source deployment.
	service := state.Services[serviceName]
	build := !isRollback && service.Repository != "" && service.Image == ""
	if build {
		if err := runner.transition(ctx, deployment, log, StatusBuilding); err != nil {
			fail(err.Error())
			return
		}
		checkout, err := runner.cloner().Clone(ctx, gitsource.Input{
			URL: service.Repository, Branch: service.Branch,
			Dir: workspace.Root, Token: secrets[configuration.GitTokenKey],
		}, func(line string) {
			log.line(ctx, "stdout", line)
		})
		if err != nil {
			fail("fetch source: " + err.Error())
			return
		}
		version = checkout.CommitSHA
		_ = runner.DB.WithContext(ctx).Model(deployment).UpdateColumns(map[string]any{
			"commit_sha": version,
			"image":      builtImage(state, slugs, serviceName, version),
		}).Error
		log.line(ctx, "system", fmt.Sprintf("checked out %s at %s", service.Repository, version))
	}

	if err := workspace.Materialize(); err != nil {
		fail("materialize workspace: " + err.Error())
		return
	}

	if build {
		result, err := runner.Engine.Build(ctx, kamal.DeployRequest{Workspace: workspace, Version: version}, func(line string) {
			log.line(ctx, "stdout", line)
		})
		if err != nil {
			fail("engine: " + err.Error())
			return
		}
		if result.ExitCode != 0 {
			fail(fmt.Sprintf("kamal build exited with status %d", result.ExitCode))
			return
		}
		// Kamal's buildx push happens inside the build; the state is recorded
		// so the timeline still shows the phase completing.
		if err := runner.transition(ctx, deployment, log, StatusPushing); err != nil {
			fail(err.Error())
			return
		}
	}

	running := StatusDeploying
	if isRollback {
		running = StatusRollingBack
	}
	if err := runner.transition(ctx, deployment, log, running); err != nil {
		fail(err.Error())
		return
	}

	result, err := runner.Engine.Deploy(ctx, kamal.DeployRequest{
		Workspace: workspace,
		Rollback:  isRollback && version != "",
		Version:   version,
	}, func(line string) {
		log.line(ctx, "stdout", line)
	})
	if err != nil {
		fail("engine: " + err.Error())
		return
	}
	if result.ExitCode != 0 {
		fail(fmt.Sprintf("kamal exited with status %d", result.ExitCode))
		return
	}
	if isRollback {
		runner.finish(ctx, deployment, log, StatusRolledBack)
		return
	}
	runner.finish(ctx, deployment, log, StatusSuccess)
}

// resolveConfiguration produces the desired state and rendered configs: a
// fresh compile + validation + snapshot for deploys, or the source
// deployment's stored version for rollbacks (SH-076, SH-077). For rollbacks
// it also returns the container version to reactivate — the source
// deployment's commit SHA, when it was a repository build.
func (runner *Runner) resolveConfiguration(ctx context.Context, deployment *migrations.Deployment, log *eventLog, isRollback bool) (configuration.DesiredState, map[string][]byte, string, error) {
	slugs, err := runner.slugs(ctx, deployment.EnvironmentID)
	if err != nil {
		return configuration.DesiredState{}, nil, "", err
	}
	input := configuration.RenderInput{ProjectSlug: slugs[0], EnvironmentSlug: slugs[1]}

	if isRollback {
		var source migrations.Deployment
		if err := runner.DB.WithContext(ctx).First(&source, "id = ?", *deployment.SourceDeploymentID).Error; err != nil {
			return configuration.DesiredState{}, nil, "", fmt.Errorf("load source deployment: %w", err)
		}
		if source.ConfigurationVersionID == nil {
			return configuration.DesiredState{}, nil, "", errors.New("source deployment has no configuration version")
		}
		var version migrations.ConfigurationVersion
		if err := runner.DB.WithContext(ctx).First(&version, "id = ?", *source.ConfigurationVersionID).Error; err != nil {
			return configuration.DesiredState{}, nil, "", fmt.Errorf("load configuration version: %w", err)
		}
		record, err := runner.Configuration.Version(ctx, deployment.EnvironmentID, version.Version)
		if err != nil {
			return configuration.DesiredState{}, nil, "", err
		}
		if err := runner.checkRollbackServers(ctx, record.State); err != nil {
			return configuration.DesiredState{}, nil, "", err
		}
		_ = runner.DB.WithContext(ctx).Model(deployment).UpdateColumns(map[string]any{
			"configuration_version_id": *source.ConfigurationVersionID,
			"commit_sha":               source.CommitSHA,
			"image":                    source.Image,
		}).Error
		log.line(ctx, "system", fmt.Sprintf("rolling back to configuration v%d", version.Version))
		rendered, err := configuration.Render(input, record.State)
		return record.State, rendered, source.CommitSHA, err
	}

	state, facts, err := runner.Configuration.Compile(ctx, deployment.EnvironmentID)
	if err != nil {
		return configuration.DesiredState{}, nil, "", err
	}
	blocking := 0
	for _, violation := range configuration.Validate(state, facts) {
		if violation.Severity == configuration.SeverityBlock {
			blocking++
			log.line(ctx, "system", fmt.Sprintf("validation %s: %s (%s %s)",
				violation.Code, violation.Message, violation.EntityType, violation.EntityName))
		}
	}
	if blocking > 0 {
		return configuration.DesiredState{}, nil, "", fmt.Errorf("%d validation errors block this deployment", blocking)
	}

	record, err := runner.Configuration.Snapshot(ctx, deployment.EnvironmentID, nil, "deployment "+deployment.ID)
	if err != nil {
		return configuration.DesiredState{}, nil, "", fmt.Errorf("snapshot configuration: %w", err)
	}
	var versionRow migrations.ConfigurationVersion
	err = runner.DB.WithContext(ctx).
		Joins("JOIN configurations ON configurations.id = configuration_versions.configuration_id").
		Where("configurations.environment_id = ? AND configuration_versions.version = ?", deployment.EnvironmentID, record.Version).
		First(&versionRow).Error
	if err != nil {
		return configuration.DesiredState{}, nil, "", fmt.Errorf("load snapshot: %w", err)
	}
	_ = runner.DB.WithContext(ctx).Model(deployment).UpdateColumn("configuration_version_id", versionRow.ID).Error
	log.line(ctx, "system", fmt.Sprintf("configuration snapshotted as v%d", record.Version))

	rendered, err := configuration.Render(input, state)
	return state, rendered, "", err
}

// builtImage is the full reference a repository build publishes — what Kamal
// composes from registry server, image, and version — recorded on the
// deployment so history and rollbacks name the exact artifact.
func builtImage(state configuration.DesiredState, slugs [2]string, serviceName, version string) string {
	image := configuration.KamalServiceName(slugs[0], slugs[1], serviceName) + ":" + version
	if server := state.Env[configuration.RegistryServerVar]; server != "" {
		return server + "/" + image
	}
	return image
}

// checkRollbackServers fails a rollback whose recorded hosts left the
// inventory (SH-076 acceptance).
func (runner *Runner) checkRollbackServers(ctx context.Context, state configuration.DesiredState) error {
	for role, hosts := range state.Roles {
		for _, host := range hosts {
			var count int64
			err := runner.DB.WithContext(ctx).Model(&migrations.Server{}).
				Where("ip_address = ? OR hostname = ?", host, host).Count(&count).Error
			if err != nil {
				return err
			}
			if count == 0 {
				return fmt.Errorf("rollback target references server %s (role %s) which no longer exists", host, role)
			}
		}
	}
	return nil
}

func (runner *Runner) serviceDocument(ctx context.Context, deployment *migrations.Deployment, rendered map[string][]byte) (string, []byte, error) {
	var name string
	if err := runner.DB.WithContext(ctx).Model(&migrations.Service{}).
		Where("id = ?", deployment.ServiceID).Pluck("name", &name).Error; err != nil || name == "" {
		return "", nil, errors.New("deployment service no longer exists")
	}
	document, present := rendered[name]
	if !present {
		return "", nil, fmt.Errorf("service %s is not part of the rendered configuration", name)
	}
	return name, document, nil
}

// materializeSecrets decrypts the environment-level and service-scoped
// secrets for the workspace secrets file.
func (runner *Runner) materializeSecrets(ctx context.Context, environmentID, serviceID string) (map[string]string, error) {
	var secretRows []migrations.Secret
	err := runner.DB.WithContext(ctx).
		Where("environment_id = ? AND (service_id IS NULL OR service_id = ?)", environmentID, serviceID).
		Order("name ASC").Find(&secretRows).Error
	if err != nil {
		return nil, err
	}
	values := make(map[string]string, len(secretRows))
	for _, secret := range secretRows {
		var entry migrations.VaultEntry
		err := runner.DB.WithContext(ctx).First(&entry, "secret_id = ?", secret.ID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue // validation already blocks on missing values
		}
		if err != nil {
			return nil, err
		}
		plaintext, err := runner.Vault.Reveal(ctx, entry.ID)
		if err != nil {
			return nil, fmt.Errorf("decrypt secret %s: %w", secret.Name, err)
		}
		values[secret.Name] = string(plaintext)
	}
	return values, nil
}

// deployKey requires one SSH key across the environment's servers — the
// rendered configuration points Kamal at a single key file.
func (runner *Runner) deployKey(ctx context.Context, environmentID string) ([]byte, error) {
	var keyIDs []string
	err := runner.DB.WithContext(ctx).Model(&migrations.Server{}).Distinct("servers.ssh_key_id").
		Joins("JOIN server_group_memberships ON server_group_memberships.server_id = servers.id").
		Joins("JOIN server_groups ON server_groups.id = server_group_memberships.server_group_id").
		Where("server_groups.environment_id = ? AND servers.ssh_key_id IS NOT NULL", environmentID).
		Pluck("servers.ssh_key_id", &keyIDs).Error
	if err != nil {
		return nil, err
	}
	if len(keyIDs) == 0 {
		return nil, errors.New("no server in this environment has an SSH key")
	}
	if len(keyIDs) > 1 {
		return nil, errors.New("servers in this environment use different SSH keys; V1 requires one key per environment")
	}
	return runner.SSHKeys.PrivateKeyPEM(ctx, keyIDs[0])
}

func (runner *Runner) slugs(ctx context.Context, environmentID string) ([2]string, error) {
	var environment migrations.Environment
	if err := runner.DB.WithContext(ctx).Preload("Project").First(&environment, "id = ?", environmentID).Error; err != nil {
		return [2]string{}, fmt.Errorf("load environment: %w", err)
	}
	return [2]string{environment.Project.Slug, environment.Slug}, nil
}

// transition enforces the SH-070 table with an optimistic status guard, so an
// illegal or concurrent change is rejected at the persistence layer.
func (runner *Runner) transition(ctx context.Context, deployment *migrations.Deployment, log *eventLog, to Status) error {
	from := Status(deployment.Status)
	if !CanTransition(from, to) {
		return &ErrIllegalTransition{From: from, To: to}
	}
	now := time.Now().UTC()
	updates := map[string]any{"status": string(to), "updated_at": now}
	if from == StatusQueued {
		updates["started_at"] = now
	}
	if to.Terminal() {
		updates["finished_at"] = now
	}
	result := runner.DB.WithContext(ctx).Model(&migrations.Deployment{}).
		Where("id = ? AND status = ?", deployment.ID, string(from)).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return &ErrIllegalTransition{From: from, To: to}
	}
	deployment.Status = string(to)
	log.status(ctx, to)
	return nil
}

func (runner *Runner) finish(ctx context.Context, deployment *migrations.Deployment, log *eventLog, to Status) {
	if err := runner.transition(ctx, deployment, log, to); err != nil {
		// Force-terminate: a deployment must never stay non-terminal.
		now := time.Now().UTC()
		_ = runner.DB.WithContext(ctx).Model(&migrations.Deployment{}).
			Where("id = ?", deployment.ID).Updates(map[string]any{
			"status": string(StatusFailed), "finished_at": now, "updated_at": now,
		}).Error
		log.status(ctx, StatusFailed)
	}
}
