package setup

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestRouterMCPMigrationPreservesExactUnselectedBytes(t *testing.T) {
	legacy, engine := "/owned/.local/bin/sirsi-router-mcp", "/Applications/Pantheon.app/Contents/MacOS/sirsi"
	for _, project := range []string{"", "/owned/repo"} {
		for _, args := range []string{`, "args": []`, ""} {
			entry := `{"command": "` + legacy + `"` + args + `, "env":{"TOKEN":"private-fixture-value"}, "cwd":"/private/fixture", "disabled":false}`
			servers := `{"router": ` + entry + `, "sirsi":{"command":"/usr/local/bin/sirsi","args":["mcp"]}, "other":{"command":"do-not-change","args":["--flag"]}}`
			data := `{ "mcpServers":` + servers + `, "unknown": {"literal":1.20e+2} }`
			if project != "" {
				data = `{ "projects": {"/owned/repo": {"mcpServers":` + servers + `,"trusted":true}}, "mcpServers":{"other":{"command":"untouched"}} }`
			}
			selection := RouterMCPSelection{Client: "claude-code", Server: "router", Project: project}
			after, _, noop, err := migrateMCPBytes([]byte(data), selection, legacy, engine)
			if err != nil || noop {
				t.Fatalf("valid migration refused: %v", err)
			}
			wantEntry := strings.Replace(entry, `"command": "`+legacy+`"`, `"command": "`+engine+`"`, 1)
			if args != "" {
				wantEntry = strings.Replace(wantEntry, `"args": []`, `"args": ["router-mcp"]`, 1)
			} else {
				wantEntry = strings.TrimSuffix(wantEntry, "}") + `,"args":["router-mcp"]}`
			}
			want := strings.Replace(data, entry, wantEntry, 1)
			if string(after) != want {
				t.Fatal("bytes outside selected command/args changed")
			}
			again, _, noop, err := migrateMCPBytes(after, selection, legacy, engine)
			if err != nil || !noop || !bytes.Equal(again, after) {
				t.Fatal("already migrated entry was not a verified byte-preserving no-op")
			}
		}
	}
}

func TestRouterMCPMigrationRejectsAmbiguousOrNonLegacyEntries(t *testing.T) {
	legacy, engine := "/owned/.local/bin/sirsi-router-mcp", "/managed/sirsi"
	for _, entry := range []string{
		`{"command":"sirsi-router-mcp","args":[]}`,
		`{"command":"/bin/sh","args":["/owned/.local/bin/sirsi-router-mcp"]}`,
		`{"command":"/managed/sirsi","args":["mcp"]}`,
		`{"command":"` + legacy + `","args":["--help"]}`,
		`{"command":"` + legacy + `","args":null}`,
		`{"command":"` + legacy + `","args":""}`,
		`{"command":"` + legacy + `","command":"` + legacy + `","args":[]}`,
		`{"command":"` + legacy + `","Command":"` + legacy + `","args":[]}`,
		`{"command":"` + legacy + `","Args":[]}`,
		`{"command":"` + legacy + `","args":[],"env":{"X":"a","X":"b"}}`,
		`{"command":"` + legacy + `","args":[],"env":{"X":null}}`,
		`{"command":"` + legacy + `","args":[],"env":null}`,
		`{"command":"` + legacy + `","args":[],"cwd":null}`,
		`{"command":"` + legacy + `","args":[],"url":"http://loopback.invalid"}`,
		`{"command":"` + legacy + `","args":[],"type":"http"}`,
		`null`,
	} {
		data := []byte(`{"mcpServers":{"router":` + entry + `}}`)
		if _, _, _, err := migrateMCPBytes(data, RouterMCPSelection{Client: "cursor", Server: "router"}, legacy, engine); err == nil {
			t.Fatal("unsafe/ambiguous entry accepted")
		}
	}
	for _, data := range []string{`{"McpServers":{}}`, `{"mcpServers":{},"mcpServers":{}}`, `{"mcpServers":{},"other":{"x":1,"x":2}}`, `{} {}`, `{"mcpServers":`} {
		if _, _, _, err := migrateMCPBytes([]byte(data), RouterMCPSelection{Client: "cursor", Server: "router"}, legacy, engine); err == nil {
			t.Fatal("unclosed or ambiguous config accepted")
		}
	}
}

func TestRouterMCPMigrationBoundsAndSelection(t *testing.T) {
	if _, err := parseMCPConfig([]byte(`{"padding":"` + strings.Repeat("x", routerMCPConfigLimit) + `"}`)); err == nil {
		t.Fatal("oversize document accepted")
	}
	if _, err := parseMCPConfig([]byte(strings.Repeat("[", 66) + "0" + strings.Repeat("]", 66))); err == nil {
		t.Fatal("excessive nesting accepted")
	}
	for _, s := range []RouterMCPSelection{
		{Client: "unknown", Config: "/owned/.claude.json"},
		{Client: "claude-code", Config: "/foreign/.claude.json"},
		{Client: "cursor", Config: "/owned/.cursor/mcp.json", Project: "/repo"},
	} {
		if routerMCPConfigPath("/owned", s) == nil {
			t.Fatal("unsupported scope accepted")
		}
	}
	p := RouterMCPMigrationPlan{Selection: RouterMCPSelection{Client: "cursor", Config: "/owned/.cursor/mcp.json", Server: "router"}, ConfigSHA256: "a", Engine: RouterMCPEngine{SHA256: "b"}}
	first := routerMCPPlanDigest(p)
	p.ConfigSHA256 = "c"
	if first == routerMCPPlanDigest(p) {
		t.Fatal("confirmation omitted source bytes")
	}
	encoded, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("private-fixture-value")) {
		t.Fatal("preview contained env values")
	}
}
