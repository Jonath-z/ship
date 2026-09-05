package configuration

import (
	"fmt"
	"sort"
	"strings"
)

type Severity string

const (
	SeverityBlock Severity = "block" // deployment must not proceed
	SeverityWarn  Severity = "warn"  // surfaced on save, does not stop a deploy
)

// Violation identifies one broken rule. Codes are stable identifiers the UI
// and tests key on; EntityType/EntityName link to the offending entity.
type Violation struct {
	Code       string   `json:"code"`
	Severity   Severity `json:"severity"`
	Message    string   `json:"message"`
	EntityType string   `json:"entityType"`
	EntityName string   `json:"entityName"`
}

// Validate rejects impossible states before anything runs (SH-052). It is a
// pure function of the compiled state and facts, so the same input always
// yields the same violations in the same order.
func Validate(state DesiredState, facts Facts) []Violation {
	var violations []Violation
	add := func(code string, severity Severity, entityType, entityName, message string) {
		violations = append(violations, Violation{
			Code: code, Severity: severity, Message: message,
			EntityType: entityType, EntityName: entityName,
		})
	}

	hostPorts := map[string]map[int][]string{} // host -> port -> user names
	claimPort := func(hosts []string, port int, name string) {
		if port == 0 {
			return
		}
		for _, host := range hosts {
			if hostPorts[host] == nil {
				hostPorts[host] = map[int][]string{}
			}
			hostPorts[host][port] = append(hostPorts[host][port], name)
		}
	}

	for _, name := range sortedKeys(state.Services) {
		service := state.Services[name]
		if len(service.Hosts) == 0 {
			add("service_unplaced", SeverityBlock, "service", name,
				"service has no servers; assign a role with at least one server")
		}
		if len(service.Domains) > 0 && service.Port == 0 {
			add("domain_service_port_missing", SeverityBlock, "service", name,
				"service has a domain but no port; the proxy needs an app port")
		}
		for _, domain := range service.Domains {
			if domain.SSLEnabled && len(service.Hosts) > 1 {
				add("ssl_multi_host", SeverityWarn, "domain", domain.Hostname,
					"automatic SSL with multiple hosts requires DNS records for every host")
			}
		}
		claimPort(service.Hosts, service.Port, "service "+name)
	}

	for _, name := range sortedKeys(state.Accessories) {
		accessory := state.Accessories[name]
		if len(accessory.Hosts) == 0 {
			add("accessory_unplaced", SeverityBlock, "accessory", name,
				"accessory has no server; place it on a server or group before deploying")
		}
		for _, host := range accessory.Hosts {
			if status, known := facts.HostStatus[host]; known && status != "connected" {
				add("accessory_server_unreachable", SeverityWarn, "accessory", name,
					fmt.Sprintf("server %s is %s", host, status))
			}
		}
		claimPort(accessory.Hosts, accessory.Port, "accessory "+name)
	}

	for _, host := range sortedKeys(hostPorts) {
		for port, users := range hostPorts[host] {
			if len(users) > 1 {
				sort.Strings(users)
				add("port_conflict", SeverityBlock, "server", host,
					fmt.Sprintf("port %d is claimed by %s", port, strings.Join(users, " and ")))
			}
		}
	}

	for _, name := range facts.SecretsWithoutValue {
		add("secret_missing_value", SeverityBlock, "secret", name,
			"secret has no stored value; set it before deploying")
	}

	for _, cycle := range dependencyCycles(state) {
		add("dependency_cycle", SeverityBlock, "service", cycle,
			"service is part of a dependency cycle")
	}

	sort.SliceStable(violations, func(i, j int) bool {
		if violations[i].Code != violations[j].Code {
			return violations[i].Code < violations[j].Code
		}
		return violations[i].EntityName < violations[j].EntityName
	})
	return violations
}

// dependencyCycles returns the names of services on a service-to-service
// dependency cycle, sorted: a service is on a cycle exactly when it can reach
// itself. Accessories cannot have outgoing edges.
func dependencyCycles(state DesiredState) []string {
	edges := map[string][]string{}
	for name, spec := range state.Services {
		for _, target := range spec.DependsOn {
			if targetName, isService := strings.CutPrefix(target, "service:"); isService {
				edges[name] = append(edges[name], targetName)
			}
		}
	}
	var names []string
	for _, name := range sortedKeys(state.Services) {
		visited := map[string]bool{}
		stack := append([]string(nil), edges[name]...)
		for len(stack) > 0 {
			current := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if current == name {
				names = append(names, name)
				break
			}
			if visited[current] {
				continue
			}
			visited[current] = true
			stack = append(stack, edges[current]...)
		}
	}
	return names
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
