package configuration

// DesiredState is the canonical description of what should be running in one
// environment (spec §54). It is the single source of truth: the UI edits it,
// the validator checks it, the renderer translates it, and drift detection
// compares it against observed reality (§55).
//
// It deliberately contains no Kamal concepts.
type DesiredState struct {
	EnvironmentID string                 `json:"environmentId"`
	Version       int                    `json:"version"`
	Services      map[string]ServiceSpec `json:"services"`
	Accessories   map[string]Accessory   `json:"accessories"`
	Roles         map[string][]string    `json:"roles"` // role name -> server IDs
	Env           map[string]string      `json:"env"`
	SecretRefs    []string               `json:"secretRefs"` // names only, never values
}

type ServiceSpec struct {
	Servers      []string  `json:"servers"`
	Role         string    `json:"role,omitempty"`
	Port         int       `json:"port"`
	Command      string    `json:"command,omitempty"`
	Image        string    `json:"image,omitempty"`
	Domains      []Domain  `json:"domains,omitempty"`
	Volumes      []Volume  `json:"volumes,omitempty"`
	HealthCheck  string    `json:"healthCheck,omitempty"`
	DependsOn    []string  `json:"dependsOn,omitempty"`
}

type Accessory struct {
	Type    string   `json:"type"`
	Image   string   `json:"image"`
	Server  string   `json:"server"`
	Port    int      `json:"port,omitempty"`
	Volumes []Volume `json:"volumes,omitempty"`
}

type Domain struct {
	Hostname   string `json:"hostname"`
	SSLEnabled bool   `json:"sslEnabled"`
}

type Volume struct {
	Name        string `json:"name"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
}
