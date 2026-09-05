package configuration

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

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
	Servers     map[string]kamalRole      `yaml:"servers"`
	Proxy       *kamalProxy               `yaml:"proxy,omitempty"`
	Env         *kamalEnv                 `yaml:"env,omitempty"`
	Volumes     []string                  `yaml:"volumes,omitempty"`
	Accessories map[string]kamalAccessory `yaml:"accessories,omitempty"`
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
			Service: kamalName(input.ProjectSlug, input.EnvironmentSlug, name),
			Image:   serviceImage(input, name, service),
			Servers: map[string]kamalRole{},
			Volumes: volumeMounts(service.Volumes),
			Env:     renderEnv(state, service),
		}
		role := service.Role
		if role == "" {
			role = "web"
		}
		config.Servers[role] = kamalRole{Hosts: service.Hosts, Cmd: service.Command}
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
// through Ship in a later epic; until then the rendered config carries the
// deterministic image name those builds will publish.
func serviceImage(input RenderInput, name string, service ServiceSpec) string {
	if service.Image != "" {
		return service.Image
	}
	return kamalName(input.ProjectSlug, input.EnvironmentSlug, name) + ":latest"
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

// renderEnv merges environment-level values with service overrides (the
// service wins) and lists secrets by name only — values are materialized
// exclusively into the deployment workspace (E6), never into configuration.
func renderEnv(state DesiredState, service ServiceSpec) *kamalEnv {
	clearValues := map[string]string{}
	for name, value := range state.Env {
		clearValues[name] = value
	}
	for name, value := range service.Env {
		clearValues[name] = value
	}
	secretSet := map[string]bool{}
	for _, name := range state.SecretRefs {
		secretSet[name] = true
	}
	for _, name := range service.SecretRefs {
		secretSet[name] = true
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
