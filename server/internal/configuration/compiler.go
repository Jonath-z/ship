package configuration

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
)

// Facts are operational details the validator needs but that do not belong in
// the desired state itself (the state describes intent, not runtime status).
type Facts struct {
	HostStatus          map[string]string // host address -> server status
	SecretsWithoutValue []string          // secret names with no stored value
}

// Compile builds the desired-state document for one environment. Compiling the
// same database state twice produces byte-identical output: rows are loaded in
// a fixed order, every slice is sorted here, and maps serialize with sorted
// keys.
func (repository *Repository) Compile(ctx context.Context, environmentID string) (DesiredState, Facts, error) {
	rows, err := repository.Load(ctx, environmentID)
	if err != nil {
		return DesiredState{}, Facts{}, err
	}
	state, facts := compile(environmentID, rows)
	return state, facts, nil
}

func compile(environmentID string, rows EnvironmentRows) (DesiredState, Facts) {
	state := DesiredState{
		EnvironmentID: environmentID,
		Services:      map[string]ServiceSpec{},
		Accessories:   map[string]Accessory{},
		Roles:         map[string][]string{},
	}

	groupNames := map[string]string{}
	for _, group := range rows.Groups {
		hosts := append([]string(nil), rows.GroupHosts[group.ID]...)
		sort.Strings(hosts)
		state.Roles[group.Name] = hosts
		groupNames[group.ID] = group.Name
	}

	serviceNames := map[string]string{}
	accessoryNames := map[string]string{}
	for _, service := range rows.Services {
		serviceNames[service.ID] = service.Name
	}
	for _, accessory := range rows.Accessories {
		accessoryNames[accessory.ID] = accessory.Name
	}

	serviceVolumes := map[string][]Volume{}
	accessoryVolumes := map[string][]Volume{}
	for _, volume := range rows.Volumes {
		value := Volume{Name: volume.Name, Source: volume.Source, Destination: volume.Destination}
		if volume.ServiceID != nil {
			serviceVolumes[*volume.ServiceID] = append(serviceVolumes[*volume.ServiceID], value)
		} else if volume.AccessoryID != nil {
			accessoryVolumes[*volume.AccessoryID] = append(accessoryVolumes[*volume.AccessoryID], value)
		}
	}

	serviceDomains := map[string][]Domain{}
	for _, domain := range rows.Domains {
		serviceDomains[domain.ServiceID] = append(serviceDomains[domain.ServiceID], Domain{
			Hostname: domain.Hostname, SSLEnabled: domain.SSLEnabled,
		})
	}

	serviceEnv := map[string]map[string]string{}
	for _, variable := range rows.Variables {
		if variable.ServiceID == nil {
			if state.Env == nil {
				state.Env = map[string]string{}
			}
			state.Env[variable.Name] = variable.Value
			continue
		}
		if serviceEnv[*variable.ServiceID] == nil {
			serviceEnv[*variable.ServiceID] = map[string]string{}
		}
		serviceEnv[*variable.ServiceID][variable.Name] = variable.Value
	}

	facts := Facts{HostStatus: rows.HostStatus}
	serviceSecrets := map[string][]string{}
	for _, secret := range rows.Secrets {
		if !rows.SecretHasValue[secret.ID] {
			facts.SecretsWithoutValue = append(facts.SecretsWithoutValue, secret.Name)
		}
		if secret.ServiceID == nil {
			state.SecretRefs = append(state.SecretRefs, secret.Name)
		} else {
			serviceSecrets[*secret.ServiceID] = append(serviceSecrets[*secret.ServiceID], secret.Name)
		}
	}
	sort.Strings(state.SecretRefs)
	sort.Strings(facts.SecretsWithoutValue)

	dependsOn := map[string][]string{}
	for _, dependency := range rows.Dependencies {
		var target string
		if dependency.TargetServiceID != nil {
			target = "service:" + serviceNames[*dependency.TargetServiceID]
		} else if dependency.TargetAccessoryID != nil {
			target = "accessory:" + accessoryNames[*dependency.TargetAccessoryID]
		}
		if target != "" {
			dependsOn[dependency.SourceServiceID] = append(dependsOn[dependency.SourceServiceID], target)
		}
	}

	for _, service := range rows.Services {
		spec := ServiceSpec{
			Type:       service.Type,
			Repository: service.Repository,
			Branch:     service.Branch,
			Image:      service.Image,
			Command:    service.Command,
			Domains:    serviceDomains[service.ID],
			Volumes:    serviceVolumes[service.ID],
			Env:        serviceEnv[service.ID],
			SecretRefs: serviceSecrets[service.ID],
			DependsOn:  dependsOn[service.ID],
		}
		if service.Port != nil {
			spec.Port = *service.Port
		}
		if service.ServerGroupID != nil {
			spec.Role = groupNames[*service.ServerGroupID]
			spec.Hosts = state.Roles[spec.Role]
		}
		sort.Slice(spec.Domains, func(i, j int) bool { return spec.Domains[i].Hostname < spec.Domains[j].Hostname })
		sort.Slice(spec.Volumes, func(i, j int) bool { return spec.Volumes[i].Source < spec.Volumes[j].Source })
		sort.Strings(spec.SecretRefs)
		sort.Strings(spec.DependsOn)
		state.Services[service.Name] = spec
	}

	for _, accessory := range rows.Accessories {
		spec := Accessory{
			Type:    accessory.Type,
			Image:   accessory.Image,
			Volumes: accessoryVolumes[accessory.ID],
		}
		if accessory.Port != nil {
			spec.Port = *accessory.Port
		}
		if accessory.ServerGroupID != nil {
			spec.Role = groupNames[*accessory.ServerGroupID]
			spec.Hosts = state.Roles[spec.Role]
		} else if accessory.ServerID != nil {
			if address, ok := serverAddress(rows, *accessory.ServerID); ok {
				spec.Hosts = []string{address}
			}
		}
		sort.Slice(spec.Volumes, func(i, j int) bool { return spec.Volumes[i].Source < spec.Volumes[j].Source })
		state.Accessories[accessory.Name] = spec
	}
	return state, facts
}

// serverAddress resolves a directly-placed accessory server. Direct placement
// bypasses groups, so the address is looked up separately by the repository.
func serverAddress(rows EnvironmentRows, serverID string) (string, bool) {
	address, ok := rows.ServerAddress[serverID]
	return address, ok
}

// CanonicalJSON is the byte representation stored in configuration versions
// and compared by tests. encoding/json sorts map keys, and the compiler sorts
// every slice, so equal states always produce equal bytes.
func CanonicalJSON(state DesiredState) ([]byte, error) {
	document, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("marshal desired state: %w", err)
	}
	return document, nil
}
