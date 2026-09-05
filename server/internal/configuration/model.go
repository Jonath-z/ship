package configuration

// DesiredState is the canonical description of what should be running in one
// environment (spec §54). It is the single source of truth: the UI edits it,
// the validator checks it, the renderer translates it, and drift detection
// compares it against observed reality (§55).
//
// It deliberately contains no Kamal concepts and never carries secret values —
// secrets appear as names only. All maps serialize with sorted keys and all
// slices are sorted by the compiler, so the same state always marshals to the
// same bytes.
type DesiredState struct {
	EnvironmentID string                 `json:"environmentId"`
	Services      map[string]ServiceSpec `json:"services"`
	Accessories   map[string]Accessory   `json:"accessories"`
	Roles         map[string][]string    `json:"roles"`                // role name -> sorted host addresses
	Env           map[string]string      `json:"env,omitempty"`        // environment-level clear variables
	SecretRefs    []string               `json:"secretRefs,omitempty"` // environment-level secret names
}

type ServiceSpec struct {
	Type       string            `json:"type"`
	Repository string            `json:"repository,omitempty"`
	Branch     string            `json:"branch,omitempty"`
	Image      string            `json:"image,omitempty"`
	Port       int               `json:"port,omitempty"`
	Command    string            `json:"command,omitempty"`
	Role       string            `json:"role,omitempty"`
	Hosts      []string          `json:"hosts,omitempty"` // resolved from the role, sorted
	Domains    []Domain          `json:"domains,omitempty"`
	Volumes    []Volume          `json:"volumes,omitempty"`
	Env        map[string]string `json:"env,omitempty"`        // service-level overrides
	SecretRefs []string          `json:"secretRefs,omitempty"` // service-level secret names
	DependsOn  []string          `json:"dependsOn,omitempty"`  // "service:<name>" or "accessory:<name>"
}

type Accessory struct {
	Type    string   `json:"type"`
	Image   string   `json:"image"`
	Role    string   `json:"role,omitempty"`  // server group placement
	Hosts   []string `json:"hosts,omitempty"` // resolved placement, sorted
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
