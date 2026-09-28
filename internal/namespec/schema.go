package namespec

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Schema is the ADR-072 C2 registry schema: the versioned, origin-pinned set of
// allowed agent and project values and machine aliases. It is loaded from
// contracts/naming/registry-schema-vN.json read from origin/main (the router
// never authorizes from a working-tree copy — the origin read is P2). This type
// only answers "is this name's agent/project/machine in the allowed set"; it does
// NOT bind a machine alias to a credentialed machine-id (C3, runtime).
type Schema struct {
	SchemaVersion      int                     `json:"schema_version"`
	Agents             []string                `json:"agents"`
	Projects           []string                `json:"projects"`
	Machines           map[string]MachineAlias `json:"machines"`
	ADR                string                  `json:"adr,omitempty"`
	Description        string                  `json:"description,omitempty"`
	Task               *TaskSpec               `json:"task,omitempty"`
	PromotionAuthority string                  `json:"promotion_authority,omitempty"`
}

// MachineAlias is one entry of the schema's machine-alias table.
type MachineAlias struct {
	DesignatedNamePrefix string `json:"designated_name_prefix"`
	Note                 string `json:"note,omitempty"`
}

// TaskSpec documents the task slot's grammar; the task is free-form within the
// grammar and not enumerated.
type TaskSpec struct {
	Grammar string `json:"grammar,omitempty"`
	Note    string `json:"note,omitempty"`
}

// LoadSchema parses and sanity-checks a registry schema document. It rejects a
// schema with no version, no agents, no projects, or no machines — an empty set
// would authorize nothing (or, worse, silently pass an unchecked name), so an
// empty schema is a load error, not a permissive default.
func LoadSchema(raw []byte) (Schema, error) {
	var s Schema
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&s); err != nil {
		return Schema{}, fmt.Errorf("namespec: parse registry schema: %w", err)
	}
	if s.SchemaVersion < 1 {
		return Schema{}, fmt.Errorf("namespec: registry schema has no schema_version")
	}
	if len(s.Agents) == 0 || len(s.Projects) == 0 || len(s.Machines) == 0 {
		return Schema{}, fmt.Errorf("namespec: registry schema is missing agents, projects, or machines")
	}
	// Every enumerated value must itself be a valid slot, or a name built from it
	// could never parse — a schema that lists "Foo" or "a-b" is malformed.
	for _, a := range s.Agents {
		if !slotRe.MatchString(a) {
			return Schema{}, fmt.Errorf("namespec: schema agent %q is not a valid slot [a-z0-9]+", a)
		}
	}
	for _, p := range s.Projects {
		if !slotRe.MatchString(p) {
			return Schema{}, fmt.Errorf("namespec: schema project %q is not a valid slot [a-z0-9]+", p)
		}
	}
	for m := range s.Machines {
		if !slotRe.MatchString(m) {
			return Schema{}, fmt.Errorf("namespec: schema machine alias %q is not a valid slot [a-z0-9]+", m)
		}
	}
	return s, nil
}

// Allows reports whether n's agent, project, and machine are all in the schema's
// allowed sets. The task is free-form within the grammar (not enumerated). A name
// whose agent/project/machine is not enumerated is refused — this is the check
// the router applies (against the origin-pinned schema) before it hands out a
// name. Binding the machine alias to the credentialed machine-id is a separate
// runtime step (C3).
func (s Schema) Allows(n Name) error {
	if err := n.Validate(); err != nil {
		return err
	}
	if !contains(s.Agents, n.Agent) {
		return fmt.Errorf("namespec: agent %q is not in the registry schema (v%d)", n.Agent, s.SchemaVersion)
	}
	if !contains(s.Projects, n.Project) {
		return fmt.Errorf("namespec: project %q is not in the registry schema (v%d)", n.Project, s.SchemaVersion)
	}
	if _, ok := s.Machines[n.Machine]; !ok {
		return fmt.Errorf("namespec: machine alias %q is not in the registry schema (v%d)", n.Machine, s.SchemaVersion)
	}
	return nil
}

// SchemaReader reads a file at a git ref from a repo. The router injects an
// origin/main reader (e.g. a GitHub reader); a working-tree reader is NEVER used
// to authorize an identity (ADR-072 C2). Signature mirrors the stacklab reader.
type SchemaReader interface {
	ReadFile(repo, path, ref string) (content []byte, exists bool, err error)
}

// SchemaPath is the canonical origin path of the C2 registry schema.
const SchemaPath = "contracts/naming/registry-schema-v1.json"

// LoadSchemaFromOrigin reads the registry schema from origin/main of repo via
// reader and loads it (ADR-072 C2). The router authorizes a constructed name
// against THIS schema — never a working-tree copy — so a stale/partial local
// checkout can never skew or bypass the allowed sets. A missing file on origin
// is an error (the roster/schema source must exist on origin, A37), never a
// permissive empty default.
func LoadSchemaFromOrigin(reader SchemaReader, repo string) (Schema, error) {
	raw, exists, err := reader.ReadFile(repo, SchemaPath, "main")
	if err != nil {
		return Schema{}, fmt.Errorf("namespec: read registry schema from origin/main of %s: %w", repo, err)
	}
	if !exists {
		return Schema{}, fmt.Errorf("namespec: registry schema %s is not on origin/main of %s (ADR-072 C2/A37)", SchemaPath, repo)
	}
	return LoadSchema(raw)
}

// AssignWith is the router's name-authority core (ADR-072 P2): given a lane's
// COMPONENTS {agent, project, task} plus the machine the router gleaned, it
// constructs the canonical name and validates it against the grammar AND the
// schema's allowed sets. A lane never supplies the name string — it presents
// components and the router returns the one canonical name — so a lane cannot
// self-name into non-conformance. This is pure: the origin READ of the schema,
// the credentialed machine-id binding (C3), and the durable name→thread-id
// mapping (C4) are the router-side I/O that wraps this.
func AssignWith(agent, project, task, machine string, schema Schema) (Name, error) {
	n, err := Construct(agent, project, machine, task)
	if err != nil {
		return Name{}, err
	}
	if err := schema.Allows(n); err != nil {
		return Name{}, err
	}
	return n, nil
}

// Assign is AssignWith with the machine gleaned from this host's designated name.
func Assign(agent, project, task string, schema Schema) (Name, error) {
	machine, err := GleanMachine()
	if err != nil {
		return Name{}, err
	}
	return AssignWith(agent, project, task, machine, schema)
}

func contains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
