package setup

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
)

const routerMCPConfigLimit = 1 << 20

// RouterMCPSelection identifies one existing client entry, never a discovery
// scope. Project is an exact Claude projects-map key, not a path to traverse.
type RouterMCPSelection struct {
	Client  string `json:"client"`
	Config  string `json:"config"`
	Server  string `json:"server"`
	Project string `json:"project,omitempty"`
}

type RouterMCPEngine struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Device uint64 `json:"device"`
	Inode  uint64 `json:"inode"`
}

// RouterMCPMigrationPlan intentionally contains no config/env/cwd values.
// Its digest binds the raw source, target bytes/identity and exact selection.
type RouterMCPMigrationPlan struct {
	Schema          string             `json:"schema"`
	Selection       RouterMCPSelection `json:"selection"`
	OldCommand      string             `json:"old_command"`
	Legacy          RouterMCPEngine    `json:"legacy"`
	NewArguments    []string           `json:"new_arguments"`
	Engine          RouterMCPEngine    `json:"engine"`
	ConfigSHA256    string             `json:"config_sha256"`
	ConfigDevice    uint64             `json:"config_device"`
	ConfigInode     uint64             `json:"config_inode"`
	ConfigMode      uint32             `json:"config_mode"`
	ParentDevice    uint64             `json:"parent_device"`
	ParentInode     uint64             `json:"parent_inode"`
	ParentMode      uint32             `json:"parent_mode"`
	AfterSHA256     string             `json:"after_sha256"`
	PreservedFields []string           `json:"preserved_fields"`
	Backup          string             `json:"backup,omitempty"`
	Status          string             `json:"status"`
	Digest          string             `json:"digest"`
}

type RouterMCPMigrationResult struct {
	Plan          RouterMCPMigrationPlan `json:"plan"`
	Status        string                 `json:"status"`
	Backup        string                 `json:"backup,omitempty"`
	RetainedStage string                 `json:"retained_stage,omitempty"`
}

func routerMCPHash(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }
func routerMCPPlanDigest(p RouterMCPMigrationPlan) string {
	p.Digest = ""
	b, _ := json.Marshal(p)
	return routerMCPHash(b)
}

type mcpJSONNode struct {
	start, end int
	fields     map[string]*mcpJSONNode
}

// Record exact byte spans while rejecting duplicates at EVERY object depth.
// This is not a serializer: unchanged fields, including secrets, stay verbatim.
func parseMCPConfig(data []byte) (*mcpJSONNode, error) {
	if len(data) == 0 || len(data) > routerMCPConfigLimit {
		return nil, errors.New("router MCP config must be nonempty and at most 1 MiB")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	nodes := 0
	var parse func(int) (*mcpJSONNode, error)
	parse = func(depth int) (*mcpJSONNode, error) {
		nodes++
		if depth > 64 || nodes > 32768 {
			return nil, errors.New("router MCP config nesting/entry bound exceeded")
		}
		start := int(d.InputOffset())
		for start < len(data) && strings.ContainsRune(" \r\n\t,:", rune(data[start])) {
			start++
		}
		tok, err := d.Token()
		if err != nil {
			return nil, errors.New("router MCP config is not complete JSON")
		}
		n := &mcpJSONNode{start: start}
		if delim, ok := tok.(json.Delim); ok {
			switch delim {
			case '{':
				n.fields = map[string]*mcpJSONNode{}
				for d.More() {
					key, err := d.Token()
					if err != nil {
						return nil, errors.New("invalid config object")
					}
					name, ok := key.(string)
					if !ok {
						return nil, errors.New("invalid config key")
					}
					if _, exists := n.fields[name]; exists {
						return nil, errors.New("duplicate config key")
					}
					child, err := parse(depth + 1)
					if err != nil {
						return nil, err
					}
					n.fields[name] = child
				}
			case '[':
				for d.More() {
					if _, err := parse(depth + 1); err != nil {
						return nil, err
					}
				}
			default:
				return nil, errors.New("invalid config delimiter")
			}
			if _, err := d.Token(); err != nil {
				return nil, errors.New("unclosed config container")
			}
		}
		n.end = int(d.InputOffset())
		return n, nil
	}
	n, err := parse(0)
	if err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, errors.New("config must contain one JSON document")
	}
	if n.fields == nil {
		return nil, errors.New("config root must be an object")
	}
	return n, nil
}

func exactMCPFields(n *mcpJSONNode, keys ...string) error {
	if n == nil || n.fields == nil {
		return errors.New("selected config block must be an object")
	}
	for field := range n.fields {
		for _, key := range keys {
			if field != key && strings.EqualFold(field, key) {
				return errors.New("case-ambiguous config schema field")
			}
		}
	}
	return nil
}

func selectedMCPEntry(root *mcpJSONNode, s RouterMCPSelection) (*mcpJSONNode, error) {
	if s.Server == "" || len(s.Server) > 256 {
		return nil, errors.New("select one nonempty server key")
	}
	if err := exactMCPFields(root, "mcpServers", "projects"); err != nil {
		return nil, err
	}
	block := root
	if s.Project != "" {
		if s.Client != "claude-code" {
			return nil, errors.New("project scope is supported only for Claude Code")
		}
		projects := root.fields["projects"]
		if projects == nil || projects.fields == nil {
			return nil, errors.New("selected projects map is absent")
		}
		block = projects.fields[s.Project]
	}
	if err := exactMCPFields(block, "mcpServers"); err != nil {
		return nil, err
	}
	servers := block.fields["mcpServers"]
	if servers == nil || servers.fields == nil {
		return nil, errors.New("selected mcpServers map is absent")
	}
	entry := servers.fields[s.Server]
	if err := exactMCPFields(entry, "command", "args", "env", "cwd", "type", "url", "headers", "transport"); err != nil {
		return nil, err
	}
	for _, key := range []string{"url", "headers", "transport"} {
		if entry.fields[key] != nil {
			return nil, errors.New("selected entry contains unsupported non-stdio transport configuration")
		}
	}
	return entry, nil
}

func routerMCPConfigPath(home string, s RouterMCPSelection) error {
	var want string
	switch s.Client {
	case "claude-code":
		want = filepath.Join(home, ".claude.json")
	case "cursor":
		want = filepath.Join(home, ".cursor", "mcp.json")
		if s.Project != "" {
			return errors.New("Cursor project selection is unsupported")
		}
	default:
		return errors.New("select supported client claude-code or cursor")
	}
	if s.Config != want || !filepath.IsAbs(s.Config) || filepath.Clean(s.Config) != s.Config {
		return errors.New("config must be the explicitly selected client's canonical owner-local path")
	}
	return nil
}

func migrateMCPBytes(data []byte, s RouterMCPSelection, legacy, engine string) ([]byte, []string, bool, error) {
	root, err := parseMCPConfig(data)
	if err != nil {
		return nil, nil, false, err
	}
	entry, err := selectedMCPEntry(root, s)
	if err != nil {
		return nil, nil, false, err
	}
	command := entry.fields["command"]
	if command == nil {
		return nil, nil, false, errors.New("selected command is absent")
	}
	var old string
	if err := json.Unmarshal(data[command.start:command.end], &old); err != nil || old == "" {
		return nil, nil, false, errors.New("selected command must be a string")
	}
	argsNode := entry.fields["args"]
	args := []string{}
	if argsNode != nil {
		raw := data[argsNode.start:argsNode.end]
		if len(raw) == 0 || raw[0] != '[' || json.Unmarshal(raw, &args) != nil {
			return nil, nil, false, errors.New("selected args must be an array of strings")
		}
	}
	if typ := entry.fields["type"]; typ != nil {
		var value string
		if json.Unmarshal(data[typ.start:typ.end], &value) != nil || value != "stdio" {
			return nil, nil, false, errors.New("selected server must use stdio")
		}
	}
	if env := entry.fields["env"]; env != nil {
		if env.fields == nil {
			return nil, nil, false, errors.New("selected env must be an object")
		}
		for _, value := range env.fields {
			var text string
			raw := data[value.start:value.end]
			if len(raw) == 0 || raw[0] != '"' || json.Unmarshal(raw, &text) != nil {
				return nil, nil, false, errors.New("selected env values must be strings")
			}
		}
	}
	if cwd := entry.fields["cwd"]; cwd != nil {
		var value string
		raw := data[cwd.start:cwd.end]
		if len(raw) == 0 || raw[0] != '"' || json.Unmarshal(raw, &value) != nil {
			return nil, nil, false, errors.New("selected cwd must be a string")
		}
	}
	preserved := []string{}
	for key := range entry.fields {
		if key != "command" && key != "args" {
			preserved = append(preserved, key)
		}
	}
	sort.Strings(preserved)
	if old == engine && len(args) == 1 && args[0] == "router-mcp" {
		return append([]byte(nil), data...), preserved, true, nil
	}
	if old != legacy || !filepath.IsAbs(old) || len(args) != 0 {
		return nil, nil, false, errors.New("entry is not the exact selected owner-local legacy command with empty argv; unchanged")
	}
	newCommand, _ := json.Marshal(engine)
	type edit struct {
		start, end int
		bytes      []byte
	}
	edits := []edit{{command.start, command.end, newCommand}}
	if argsNode != nil {
		edits = append(edits, edit{argsNode.start, argsNode.end, []byte(`["router-mcp"]`)})
	} else {
		edits = append(edits, edit{entry.end - 1, entry.end - 1, []byte(`,"args":["router-mcp"]`)})
	}
	sort.Slice(edits, func(i, j int) bool { return edits[i].start > edits[j].start })
	after := append([]byte(nil), data...)
	for _, e := range edits {
		after = append(append(append([]byte(nil), after[:e.start]...), e.bytes...), after[e.end:]...)
	}
	if len(after) > routerMCPConfigLimit {
		return nil, nil, false, fmt.Errorf("migrated config exceeds %d bytes", routerMCPConfigLimit)
	}
	return after, preserved, false, nil
}
