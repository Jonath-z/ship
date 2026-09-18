package configuration

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Kamal's conventional names for registry authentication. Stored as ordinary
// Ship variables/secrets, but they are deploy-time credentials: the renderer
// lifts them into the registry block and keeps them out of application env.
const (
	RegistryServerVar   = "KAMAL_REGISTRY_SERVER"   // clear variable, e.g. ghcr.io
	RegistryUsernameKey = "KAMAL_REGISTRY_USERNAME" // secret, or clear variable
	RegistryPasswordKey = "KAMAL_REGISTRY_PASSWORD" // secret
)

// GitTokenKey is the conventional secret holding a Git access token for
// cloning private repositories (mirrors gitsource.TokenSecretName). Like the
// registry credentials it is deploy-time-only.
const GitTokenKey = "GIT_TOKEN"

// WebhookSecretKey is the conventional secret shared with GitHub to verify
// push webhook signatures for auto-deploys.
const WebhookSecretKey = "GITHUB_WEBHOOK_SECRET"

// isDeployTimeKey marks credentials Ship itself consumes; they never reach
// application container env.
func isDeployTimeKey(name string) bool {
	return name == RegistryServerVar ||
		name == RegistryUsernameKey ||
		name == RegistryPasswordKey ||
		name == GitTokenKey ||
		name == WebhookSecretKey
}

// RenderInput names the environment so Kamal service names are unique per
// Docker host even when several environments share servers.
type RenderInput struct {
	ProjectSlug     string
	EnvironmentSlug string
}

var slugCleanPattern = regexp.MustCompile(`[^a-z0-9]+`)

// kamalConfig mirrors Kamal's deploy.yml shape (spec §19, §44,
// docs/domain-conventions.md). Field order is fixed by the struct, map keys
// are sorted by yaml.v3, and every slice is sorted upstream — rendering the
// same state twice yields byte-identical output.
//
// Every Ship service renders to its own independent Kamal application
// configuration. Dependencies are Ship metadata and are never emitted.
type kamalConfig struct {
	Service     string                    `yaml:"service"`
	Image       string                    `yaml:"image"`
	Registry    *kamalRegistry            `yaml:"registry,omitempty"`
	Builder     kamalBuilder              `yaml:"builder"`
	Servers     map[string]kamalRole      `yaml:"servers"`
	SSH         *kamalSSH                 `yaml:"ssh,omitempty"`
	Proxy       *kamalProxy               `yaml:"proxy,omitempty"`
	Env         *kamalEnv                 `yaml:"env,omitempty"`
	Volumes     []string                  `yaml:"volumes,omitempty"`
	Accessories map[string]kamalAccessory `yaml:"accessories,omitempty"`
}

// kamalRegistry authenticates image pulls on the target servers. Username is
// either a literal string or a one-element list referencing .kamal/secrets;
// the password is always a secret reference, never a value.
type kamalRegistry struct {
	Server   string   `yaml:"server,omitempty"`
	Username any      `yaml:"username,omitempty"`
	Password []string `yaml:"password"`
}

// kamalBuilder targets the image at the platform of the servers that run it —
// arch describes where the image runs, not where it is built. Kamal 2 refuses
// any config without it, even when the deploy never builds. A scalar for the
// common single-architecture case, a list when a role mixes fleets.
type kamalBuilder struct {
	Arch any `yaml:"arch"`
}

// DefaultArch covers hosts whose checks never recorded an architecture.
const DefaultArch = "amd64"

func renderBuilder(service ServiceSpec) kamalBuilder {
	switch len(service.Arch) {
	case 0:
		return kamalBuilder{Arch: DefaultArch}
	case 1:
		return kamalBuilder{Arch: service.Arch[0]}
	default:
		return kamalBuilder{Arch: service.Arch}
	}
}

// WorkspaceSSHKeyPath is where the deploy workspace materializes the private
// key; the rendered config references it relative to the workspace root.
const WorkspaceSSHKeyPath = ".kamal/ssh_key"

type kamalSSH struct {
	User string   `yaml:"user"`
	Port int      `yaml:"port,omitempty"`
	Keys []string `yaml:"keys"`
}

type kamalRole struct {
	Hosts []string `yaml:"hosts"`
	Cmd   string   `yaml:"cmd,omitempty"`
}

type kamalProxy struct {
	Host    string   `yaml:"host,omitempty"`
	Hosts   []string `yaml:"hosts,omitempty"`
	SSL     bool     `yaml:"ssl"`
	AppPort int      `yaml:"app_port,omitempty"`
}

type kamalEnv struct {
	Clear  map[string]string `yaml:"clear,omitempty"`
	Secret []string          `yaml:"secret,omitempty"`
}

type kamalAccessory struct {
	Image   string   `yaml:"image"`
	Host    string   `yaml:"host,omitempty"`
	Hosts   []string `yaml:"hosts,omitempty"`
	Port    int      `yaml:"port,omitempty"`
	Volumes []string `yaml:"volumes,omitempty"`
}

// Render translates the desired state into one Kamal configuration per
// service (SH-055), keyed by service name. Accessories have their own
// lifecycle but must live in some config for Kamal to manage them; each is
// rendered into the config of the alphabetically-first service that depends
// on it, falling back to the alphabetically-first service.
func Render(input RenderInput, state DesiredState) (map[string][]byte, error) {
	serviceNames := sortedKeys(state.Services)
	if len(serviceNames) == 0 {
		return map[string][]byte{}, nil
	}
	accessoryHome := assignAccessories(state, serviceNames)

	rendered := make(map[string][]byte, len(serviceNames))
	for _, name := range serviceNames {
		service := state.Services[name]
		config := kamalConfig{
			Service:  kamalName(input.ProjectSlug, input.EnvironmentSlug, name),
			Image:    serviceImage(input, name, service),
			Registry: renderRegistry(state),
			Builder:  renderBuilder(service),
			Servers:  map[string]kamalRole{},
			Volumes:  volumeMounts(service.Volumes),
			Env:      renderEnv(state, service),
		}
		role := service.Role
		if role == "" {
			role = "web"
		}
		config.Servers[role] = kamalRole{Hosts: service.Hosts, Cmd: service.Command}
		if state.SSH.User != "" {
			config.SSH = &kamalSSH{User: state.SSH.User, Port: state.SSH.Port, Keys: []string{WorkspaceSSHKeyPath}}
		}
		config.Proxy = renderProxy(service)

		for _, accessoryName := range accessoryHome[name] {
			accessory := state.Accessories[accessoryName]
			if config.Accessories == nil {
				config.Accessories = map[string]kamalAccessory{}
			}
			config.Accessories[accessoryName] = renderAccessory(accessory)
		}

		document, err := yaml.Marshal(config)
		if err != nil {
			return nil, fmt.Errorf("render service %s: %w", name, err)
		}
		rendered[name] = document
	}
	return rendered, nil
}

// assignAccessories picks one home config per accessory: the first service
// depending on it, else the first service overall. Deterministic by sorted
// iteration.
func assignAccessories(state DesiredState, serviceNames []string) map[string][]string {
	home := map[string][]string{}
	for _, accessoryName := range sortedKeys(state.Accessories) {
		owner := serviceNames[0]
		for _, serviceName := range serviceNames {
			if dependsOnAccessory(state.Services[serviceName], accessoryName) {
				owner = serviceName
				break
			}
		}
		home[owner] = append(home[owner], accessoryName)
	}
	return home
}

func dependsOnAccessory(service ServiceSpec, accessoryName string) bool {
	for _, target := range service.DependsOn {
		if target == "accessory:"+accessoryName {
			return true
		}
	}
	return false
}

// serviceImage picks the deployable image. Repository-backed services build
// through Ship's pipeline, which tags the image with the cloned commit SHA —
// the image field must stay untagged because Kamal appends its own
// `:<version>` (a tag here would render an invalid double-tagged reference).
func serviceImage(input RenderInput, name string, service ServiceSpec) string {
	if service.Image != "" {
		return service.Image
	}
	return kamalName(input.ProjectSlug, input.EnvironmentSlug, name)
}

// KamalServiceName exposes the deterministic Kamal service name so the
// deployment pipeline can record the image reference a build will publish.
func KamalServiceName(projectSlug, environmentSlug, serviceName string) string {
	return kamalName(projectSlug, environmentSlug, serviceName)
}

func renderProxy(service ServiceSpec) *kamalProxy {
	if len(service.Domains) == 0 {
		return nil
	}
	proxy := &kamalProxy{AppPort: service.Port}
	hostnames := make([]string, 0, len(service.Domains))
	for _, domain := range service.Domains {
		hostnames = append(hostnames, domain.Hostname)
		if domain.SSLEnabled {
			proxy.SSL = true
		}
	}
	sort.Strings(hostnames)
	if len(hostnames) == 1 {
		proxy.Host = hostnames[0]
	} else {
		proxy.Hosts = hostnames
	}
	return proxy
}

// renderRegistry lifts Kamal's conventional registry credentials out of the
// environment's variables/secrets. Emitted only when a password secret is
// set; without it Kamal cannot log in on the target servers before pulling.
func renderRegistry(state DesiredState) *kamalRegistry {
	if !slices.Contains(state.SecretRefs, RegistryPasswordKey) {
		return nil
	}
	registry := &kamalRegistry{
		Server:   state.Env[RegistryServerVar],
		Password: []string{RegistryPasswordKey},
	}
	if slices.Contains(state.SecretRefs, RegistryUsernameKey) {
		registry.Username = []string{RegistryUsernameKey}
	} else if username := state.Env[RegistryUsernameKey]; username != "" {
		registry.Username = username
	}
	return registry
}

// renderEnv merges environment-level values with service overrides (the
// service wins) and lists secrets by name only — values are materialized
// exclusively into the deployment workspace (E6), never into configuration.
// Registry credentials and the Git token are deploy-time-only and excluded
// from application env.
func renderEnv(state DesiredState, service ServiceSpec) *kamalEnv {
	clearValues := map[string]string{}
	for name, value := range state.Env {
		if !isDeployTimeKey(name) {
			clearValues[name] = value
		}
	}
	for name, value := range service.Env {
		if !isDeployTimeKey(name) {
			clearValues[name] = value
		}
	}
	secretSet := map[string]bool{}
	for _, name := range state.SecretRefs {
		if !isDeployTimeKey(name) {
			secretSet[name] = true
		}
	}
	for _, name := range service.SecretRefs {
		if !isDeployTimeKey(name) {
			secretSet[name] = true
		}
	}
	if len(clearValues) == 0 && len(secretSet) == 0 {
		return nil
	}
	env := &kamalEnv{Secret: sortedKeys(secretSet)}
	if len(clearValues) > 0 {
		env.Clear = clearValues
	}
	return env
}

func renderAccessory(accessory Accessory) kamalAccessory {
	rendered := kamalAccessory{
		Image: accessory.Image, Port: accessory.Port,
		Volumes: volumeMounts(accessory.Volumes),
	}
	if len(accessory.Hosts) == 1 {
		rendered.Host = accessory.Hosts[0]
	} else {
		rendered.Hosts = accessory.Hosts
	}
	return rendered
}

func volumeMounts(volumes []Volume) []string {
	mounts := make([]string, 0, len(volumes))
	for _, volume := range volumes {
		mounts = append(mounts, volume.Source+":"+volume.Destination)
	}
	sort.Strings(mounts)
	return mounts
}

// kamalName builds the unique, Docker-safe Kamal service identifier.
func kamalName(projectSlug, environmentSlug, serviceName string) string {
	parts := []string{projectSlug, environmentSlug, slugify(serviceName)}
	return strings.Join(parts, "-")
}

func slugify(value string) string {
	slug := slugCleanPattern.ReplaceAllString(strings.ToLower(value), "-")
	return strings.Trim(slug, "-")
}
