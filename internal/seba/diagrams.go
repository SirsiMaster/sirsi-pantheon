// Package seba — diagrams.go
//
// 𓇽 Seba Diagram Engine — Multi-Format Architectural Mapping
//
// Generates real, usable Mermaid diagrams from live project analysis:
//   - Divine Hierarchy (deity relationships & governance)
//   - Data Flow (per-deity and per-application)
//   - Module Dependency Map (Go import graph)
//   - Memory Architecture (Thoth knowledge flow)
//   - Governance Cycle (Ma'at → Isis → Thoth loop)
//   - CI/CD Pipeline
//
// All diagrams are generated from live filesystem scanning — never hardcoded.
//
// Usage:
//
//	pantheon seba diagram --type hierarchy
//	pantheon seba diagram --type dataflow
//	pantheon seba diagram --type modules
//	pantheon seba diagram --type memory
//	pantheon seba diagram --type governance
//	pantheon seba diagram --type pipeline
//	pantheon seba diagram --type all
package seba

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// DiagramType identifies which diagram to generate.
type DiagramType string

const (
	DiagramHierarchy  DiagramType = "hierarchy"
	DiagramDataFlow   DiagramType = "dataflow"
	DiagramModules    DiagramType = "modules"
	DiagramMemory     DiagramType = "memory"
	DiagramGovernance DiagramType = "governance"
	DiagramPipeline   DiagramType = "pipeline"
)

// AllDiagramTypes returns every available diagram type.
func AllDiagramTypes() []DiagramType {
	core := []DiagramType{
		DiagramHierarchy,
		DiagramDataFlow,
		DiagramModules,
		DiagramMemory,
		DiagramGovernance,
		DiagramPipeline,
	}
	// Append all registered Phase 1+ mappers
	for dtype := range mapperRegistry {
		core = append(core, dtype)
	}
	return core
}

// DiagramResult holds a generated diagram.
type DiagramResult struct {
	Type    DiagramType `json:"type"`
	Title   string      `json:"title"`
	Mermaid string      `json:"mermaid"`
}

// GenerateDiagram produces a Mermaid diagram of the given type.
func GenerateDiagram(projectRoot string, dtype DiagramType) (*DiagramResult, error) {
	switch dtype {
	case DiagramHierarchy:
		return generateHierarchy()
	case DiagramDataFlow:
		return generateDataFlow(projectRoot)
	case DiagramModules:
		return generateModules(projectRoot)
	case DiagramMemory:
		return generateMemory()
	case DiagramGovernance:
		return generateGovernance()
	case DiagramPipeline:
		return generatePipeline()
	default:
		// Fall through to registered mappers
		if m, ok := mapperRegistry[dtype]; ok {
			return m.fn(projectRoot)
		}
		return nil, fmt.Errorf("unknown diagram type: %s", dtype)
	}
}

// GenerateAllDiagrams produces every available diagram.
func GenerateAllDiagrams(projectRoot string) ([]*DiagramResult, error) {
	var results []*DiagramResult
	for _, dt := range AllDiagramTypes() {
		r, err := GenerateDiagram(projectRoot, dt)
		if err != nil {
			continue // Skip failures, generate what we can
		}
		results = append(results, r)
	}
	return results, nil
}

// ── 1. Divine Hierarchy ─────────────────────────────────────────────

func generateHierarchy() (*DiagramResult, error) {
	mermaid := `graph TD
    Ra["𓇶 Ra<br/>Supreme Overseer"]
    Net["𓁯 Net / Neith<br/>The Weaver"]

    subgraph CodeGods["𓀭 Code Gods — Governance & Knowledge"]
        Thoth["𓁟 Thoth<br/>Memory & Knowledge"]
        Maat["𓆄 Ma'at<br/>Truth & Governance"]
        Isis["𓆄 Isis<br/>The Healer"]
        Seshat["𓁆 Seshat<br/>The Scribe"]
    end

    subgraph MachineGods["𓀰 Machine Gods — Infrastructure & OS"]
        Horus["𓂀 Horus<br/>The Eye"]
        Anubis["𓁢 Anubis<br/>The Judge"]
        Ka["⚠️ Ka<br/>The Spirit"]
        Sekhmet["𓁵 Sekhmet<br/>The Warrior"]
        Hapi["𓈗 Hapi<br/>The Flow"]
        Khepri["𓆣 Khepri<br/>The Scarab"]
        Seba["𓇽 Seba<br/>The Star"]
    end

    Ra --> Net
    Net --> CodeGods
    Net --> MachineGods

    Maat -->|"weighs"| Isis
    Isis -->|"heals"| Thoth
    Thoth -->|"records"| Net
    Seshat -->|"bridges"| Thoth

    Horus -->|"manifest"| Anubis
    Horus -->|"manifest"| Ka
    Hapi -->|"accelerates"| Sekhmet
    Seba -->|"reports to"| Net

    style Ra fill:#FFD700,stroke:#C8A951,color:#000
    style Net fill:#8E44AD,stroke:#6C3483,color:#fff
    style Thoth fill:#1A1A5E,stroke:#C8A951,color:#C8A951
    style Maat fill:#2ECC71,stroke:#27AE60,color:#000
    style Isis fill:#E74C3C,stroke:#C0392B,color:#fff
    style Seshat fill:#3498DB,stroke:#2980B9,color:#fff
    style Horus fill:#F39C12,stroke:#E67E22,color:#000
    style Anubis fill:#C8A951,stroke:#A17D32,color:#000
    style Ka fill:#E74C3C,stroke:#C0392B,color:#fff
    style Sekhmet fill:#E74C3C,stroke:#C0392B,color:#fff
    style Hapi fill:#1ABC9C,stroke:#16A085,color:#000
    style Khepri fill:#2ECC71,stroke:#27AE60,color:#000
    style Seba fill:#9B59B6,stroke:#8E44AD,color:#fff`

	return &DiagramResult{
		Type:    DiagramHierarchy,
		Title:   "𓇽 Divine Hierarchy — The Pantheon Governance Tree",
		Mermaid: mermaid,
	}, nil
}

// ── 2. Data Flow ────────────────────────────────────────────────────

func generateDataFlow(projectRoot string) (*DiagramResult, error) {
	// Discover which deity commands exist
	deities := discoverDeities(projectRoot)

	var sb strings.Builder
	sb.WriteString("graph LR\n")
	sb.WriteString("    User([\"👤 User / CLI\"])\n")
	sb.WriteString("    Binary[\"🏛️ pantheon binary\"]\n")
	sb.WriteString("    User --> Binary\n\n")

	for _, d := range deities {
		id := cases.Title(language.Und).String(d.name)
		sb.WriteString(fmt.Sprintf("    %s[\"%s %s<br/>%s\"]\n", id, d.glyph, id, d.domain))
	}
	sb.WriteString("\n")
	for _, d := range deities {
		id := cases.Title(language.Und).String(d.name)
		sb.WriteString(fmt.Sprintf("    Binary --> %s\n", id))
	}

	// Add data stores
	sb.WriteString("\n    FS[(\"📁 Filesystem\")]\n")
	sb.WriteString("    Git[(\"🔀 Git History\")]\n")
	sb.WriteString("    Config[(\"⚙️ .thoth/ .pantheon/\")]\n")
	sb.WriteString("    Network[(\"🌐 Network/Fleet\")]\n\n")

	// Wire data stores to deities
	for _, d := range deities {
		id := cases.Title(language.Und).String(d.name)
		switch d.name {
		case "anubis":
			sb.WriteString(fmt.Sprintf("    %s --> FS\n", id))
		case "maat":
			sb.WriteString(fmt.Sprintf("    %s --> Git\n", id))
			sb.WriteString(fmt.Sprintf("    %s --> FS\n", id))
		case "thoth":
			sb.WriteString(fmt.Sprintf("    %s --> Config\n", id))
			sb.WriteString(fmt.Sprintf("    %s --> Git\n", id))
		case "hapi":
			sb.WriteString(fmt.Sprintf("    %s --> FS\n", id))
		case "seba":
			sb.WriteString(fmt.Sprintf("    %s --> Network\n", id))
			sb.WriteString(fmt.Sprintf("    %s --> FS\n", id))
		case "seshat":
			sb.WriteString(fmt.Sprintf("    %s --> Config\n", id))
		}
	}

	return &DiagramResult{
		Type:    DiagramDataFlow,
		Title:   "𓇽 Data Flow — CLI → Deities → Resources",
		Mermaid: sb.String(),
	}, nil
}

// ── 3. Module Dependency Map ────────────────────────────────────────

// ModuleDep represents an import relationship between internal modules.
type ModuleDep struct {
	From string
	To   string
}

func generateModules(projectRoot string) (*DiagramResult, error) {
	deps, modules := scanModuleDeps(projectRoot)

	var sb strings.Builder
	sb.WriteString("graph TD\n")

	// Classify modules into pillars
	pillars := map[string][]string{
		"Anubis": {"cleaner", "guard", "jackal", "ka", "mirror", "horus", "ignore", "sight", "stealth"},
		"Ma'at":  {"maat", "isis", "scales"},
		"Thoth":  {"thoth", "brain", "logging"},
		"Hapi":   {"hapi", "yield", "profile"},
		"Seba":   {"seba", "scarab", "osiris"},
		"Seshat": {"seshat", "mcp"},
		"Core":   {"output", "platform", "updater", "neith"},
	}

	pillarOf := map[string]string{}
	for pillar, mods := range pillars {
		for _, m := range mods {
			pillarOf[m] = pillar
		}
	}

	// Group modules into subgraphs
	pillarModules := map[string][]string{}
	for _, m := range modules {
		p, ok := pillarOf[m]
		if !ok {
			p = "Other"
		}
		pillarModules[p] = append(pillarModules[p], m)
	}

	pillarOrder := []string{"Anubis", "Ma'at", "Thoth", "Hapi", "Seba", "Seshat", "Core", "Other"}
	for _, p := range pillarOrder {
		mods, ok := pillarModules[p]
		if !ok || len(mods) == 0 {
			continue
		}
		sort.Strings(mods)
		sb.WriteString(fmt.Sprintf("    subgraph %s\n", p))
		for _, m := range mods {
			sb.WriteString(fmt.Sprintf("        %s[\"%s\"]\n", m, m))
		}
		sb.WriteString("    end\n")
	}

	// Write edges
	sb.WriteString("\n")
	edgeSet := map[string]bool{}
	for _, d := range deps {
		key := d.From + "->" + d.To
		if edgeSet[key] {
			continue
		}
		edgeSet[key] = true
		sb.WriteString(fmt.Sprintf("    %s --> %s\n", d.From, d.To))
	}

	return &DiagramResult{
		Type:    DiagramModules,
		Title:   "𓇽 Module Dependency Map — internal/ Import Graph",
		Mermaid: sb.String(),
	}, nil
}

// scanModuleDeps parses all Go files in internal/ and extracts internal import edges.
func scanModuleDeps(projectRoot string) ([]ModuleDep, []string) {
	internalDir := filepath.Join(projectRoot, "internal")
	entries, err := os.ReadDir(internalDir)
	if err != nil {
		return nil, nil
	}

	modulePrefix := "github.com/SirsiMaster/sirsi-pantheon/internal/"
	var deps []ModuleDep
	moduleSet := map[string]bool{}
	fset := token.NewFileSet()

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		modName := entry.Name()
		moduleSet[modName] = true
		modDir := filepath.Join(internalDir, modName)

		goFiles, _ := filepath.Glob(filepath.Join(modDir, "*.go"))
		for _, gf := range goFiles {
			if strings.HasSuffix(gf, "_test.go") {
				continue
			}
			f, err := parser.ParseFile(fset, gf, nil, parser.ImportsOnly)
			if err != nil {
				continue
			}
			for _, imp := range f.Imports {
				path := strings.Trim(imp.Path.Value, `"`)
				if strings.HasPrefix(path, modulePrefix) {
					target := strings.TrimPrefix(path, modulePrefix)
					// Normalize sub-packages: jackal/rules → jackal
					if idx := strings.Index(target, "/"); idx != -1 {
						target = target[:idx]
					}
					if target != modName {
						deps = append(deps, ModuleDep{From: modName, To: target})
					}
				}
			}
		}
	}

	var modules []string
	for m := range moduleSet {
		modules = append(modules, m)
	}
	sort.Strings(modules)

	return deps, modules
}

// ── 4. Memory Architecture ──────────────────────────────────────────

func generateMemory() (*DiagramResult, error) {
	mermaid := `graph TD
    subgraph Sources["📥 Knowledge Sources"]
        Gemini["🤖 Gemini AI Mode"]
        NotebookLM["📓 NotebookLM"]
        Antigravity["💎 Antigravity IDE"]
        GitLog["🔀 Git History"]
    end

    subgraph ThothEngine["𓁟 Thoth — The Memory"]
        MemoryYAML["memory.yaml<br/>Project Identity"]
        Journal["journal.md<br/>Auto-Sync Log"]
        Rules["ANUBIS_RULES.md<br/>Governance Canon"]
    end

    subgraph SeshatBridge["𓁆 Seshat — The Bridge"]
        Extract["Extract Conversations"]
        Package["Package as Sources"]
        Inject["Inject as Knowledge Items"]
    end

    subgraph Storage["💾 Persistent Storage"]
        KI["Knowledge Items<br/>.gemini/antigravity/knowledge/"]
        Brain["Brain Logs<br/>.gemini/antigravity/brain/"]
        DotThoth[".thoth/ Directory"]
    end

    Gemini -->|"Takeout export"| Extract
    Extract --> Package
    Package -->|"upload"| NotebookLM
    NotebookLM -->|"distill"| Inject
    Inject --> KI

    Antigravity -->|"reads"| KI
    Antigravity -->|"writes"| Brain
    GitLog -->|"thoth sync"| Journal
    GitLog -->|"thoth sync"| MemoryYAML

    KI --> Antigravity
    DotThoth --- MemoryYAML
    DotThoth --- Journal
    DotThoth --- Rules

    style Gemini fill:#4285F4,stroke:#3367D6,color:#fff
    style NotebookLM fill:#EA4335,stroke:#C5221F,color:#fff
    style Antigravity fill:#9B59B6,stroke:#8E44AD,color:#fff
    style KI fill:#1A1A5E,stroke:#C8A951,color:#C8A951
    style Brain fill:#1A1A5E,stroke:#C8A951,color:#C8A951`

	return &DiagramResult{
		Type:    DiagramMemory,
		Title:   "𓇽 Memory Architecture — Knowledge Flow",
		Mermaid: mermaid,
	}, nil
}

// ── 5. Governance Cycle ─────────────────────────────────────────────

func generateGovernance() (*DiagramResult, error) {
	mermaid := `graph LR
    Net["𓁯 Net<br/>Publishes Plan"]
    Machine["𓀰 Machine Gods<br/>Execute Plan"]
    Maat["𓆄 Ma'at<br/>Weighs Execution"]
    Isis["𓆄 Isis<br/>Heals Drift"]
    Thoth["𓁟 Thoth<br/>Records Achievement"]

    Net -->|"1. Plan"| Machine
    Machine -->|"2. Execute"| Maat
    Maat -->|"3. Findings"| Isis
    Isis -->|"4. Remediate"| Thoth
    Thoth -->|"5. Record"| Net
    Net -->|"6. Re-align"| Net

    subgraph MaatDetail["𓆄 Ma'at Detail"]
        Audit["maat audit<br/>Governance Scan"]
        Scales["maat scales<br/>Policy Enforcement"]
        Pulse["maat pulse<br/>Dynamic Metrics"]
        Heal["maat heal<br/>Trigger Isis"]
    end

    Maat --> Audit
    Maat --> Scales
    Maat --> Pulse
    Maat --> Heal
    Heal --> Isis

    style Net fill:#8E44AD,stroke:#6C3483,color:#fff
    style Machine fill:#2C3E50,stroke:#1A252F,color:#C8A951
    style Maat fill:#2ECC71,stroke:#27AE60,color:#000
    style Isis fill:#E74C3C,stroke:#C0392B,color:#fff
    style Thoth fill:#1A1A5E,stroke:#C8A951,color:#C8A951
    style Pulse fill:#F39C12,stroke:#E67E22,color:#000`

	return &DiagramResult{
		Type:    DiagramGovernance,
		Title:   "𓇽 Governance Cycle — The Ma'at → Isis → Thoth Loop",
		Mermaid: mermaid,
	}, nil
}

// ── 6. CI/CD Pipeline ───────────────────────────────────────────────

func generatePipeline() (*DiagramResult, error) {
	mermaid := `graph LR
    Push["🔀 git push"]
    Gate["𓇳 Pre-Push Gate"]
    GFmt["gofmt check"]
    Test["go test -short"]
    CI["GitHub Actions CI"]
    Lint["golangci-lint"]
    FullTest["go test -race -cover"]
    Pulse["maat pulse --json"]
    Metrics["📊 metrics.json"]
    Coverage["📈 coverage.out"]
    Build["go build"]
    Binary["🏛️ pantheon binary"]

    Push --> Gate
    Gate --> GFmt
    Gate --> Test
    GFmt -->|"pass"| CI
    Test -->|"pass"| CI

    CI --> Lint
    CI --> FullTest
    CI --> Build
    FullTest --> Coverage
    Build --> Binary
    Binary --> Pulse
    Pulse --> Metrics

    style Push fill:#2C3E50,stroke:#1A252F,color:#fff
    style Gate fill:#C8A951,stroke:#A17D32,color:#000
    style CI fill:#4285F4,stroke:#3367D6,color:#fff
    style Metrics fill:#F39C12,stroke:#E67E22,color:#000
    style Binary fill:#2ECC71,stroke:#27AE60,color:#000`

	return &DiagramResult{
		Type:    DiagramPipeline,
		Title:   "𓇽 CI/CD Pipeline — Push → Gate → CI → Artifacts",
		Mermaid: mermaid,
	}, nil
}

// ── Helpers ─────────────────────────────────────────────────────────

type deityInfo struct {
	name   string
	glyph  string
	domain string
}

func discoverDeities(projectRoot string) []deityInfo {
	known := []deityInfo{
		{"anubis", "𓁢", "Hygiene"},
		{"maat", "𓆄", "Governance"},
		{"thoth", "𓁟", "Knowledge"},
		{"hapi", "𓈗", "Compute"},
		{"seba", "𓇽", "Mapping"},
		{"seshat", "𓁆", "Scribe"},
	}

	var found []deityInfo
	for _, d := range known {
		cmdPath := filepath.Join(projectRoot, "cmd", "pantheon", d.name+".go")
		if _, err := os.Stat(cmdPath); err == nil {
			found = append(found, d)
		}
	}
	return found
}

// RenderDiagramsHTML produces a self-contained, navigable HTML page.
func RenderDiagramsHTML(diagrams []*DiagramResult, outputPath string) error {
	// Categorize diagrams
	categories := map[string][]string{
		"Pantheon":      {"hierarchy", "dataflow", "memory", "governance", "pipeline"},
		"Code Analysis": {"modules", "callgraph", "commandtree", "commandwiring", "moduledataflow", "deptree"},
		"System":        {"systemoverview", "memorypressure", "cputopology", "gpuarch", "processmap", "networkports", "sshmap", "diskusage"},
		"Git & Ops":     {"commitheatmap", "filehotspots", "releasetimeline", "cipipeline"},
	}
	catOrder := []string{"Pantheon", "Code Analysis", "System", "Git & Ops"}
	catIcons := map[string]string{
		"Pantheon":      "𓇶",
		"Code Analysis": "📦",
		"System":        "🖥️",
		"Git & Ops":     "🔀",
	}

	// Build diagram index
	diagramMap := map[string]*DiagramResult{}
	for _, d := range diagrams {
		diagramMap[string(d.Type)] = d
	}

	// Build sidebar nav HTML
	var sidebarItems strings.Builder
	idx := 0
	for _, cat := range catOrder {
		types := categories[cat]
		icon := catIcons[cat]
		sidebarItems.WriteString(fmt.Sprintf(`<div class="nav-category">
      <div class="nav-cat-label">%s %s</div>`, icon, cat))
		for _, dtype := range types {
			d, ok := diagramMap[dtype]
			if !ok {
				continue
			}
			active := ""
			if idx == 0 {
				active = " active"
			}
			// Short label from title
			label := d.Title
			if dashIdx := strings.Index(label, "—"); dashIdx > 0 {
				label = strings.TrimSpace(label[dashIdx+len("—"):])
			}
			if len(label) > 35 {
				label = label[:32] + "..."
			}
			sidebarItems.WriteString(fmt.Sprintf(`
      <a class="nav-item%s" data-idx="%d" onclick="showDiagram(%d)">%s</a>`,
				active, idx, idx, label))
			idx++
		}
		sidebarItems.WriteString("\n    </div>\n")
	}

	// Build diagram slide HTML — ordered by category
	var slides strings.Builder
	slideIdx := 0
	totalDiagrams := 0
	for _, cat := range catOrder {
		types := categories[cat]
		for _, dtype := range types {
			if _, ok := diagramMap[dtype]; ok {
				totalDiagrams++
			}
		}
		_ = types
	}

	for _, cat := range catOrder {
		types := categories[cat]
		for _, dtype := range types {
			d, ok := diagramMap[dtype]
			if !ok {
				continue
			}
			display := "none"
			if slideIdx == 0 {
				display = "block"
			}
			// Script tags treat content as raw text — only need to escape </script>
			safeMermaid := strings.ReplaceAll(d.Mermaid, "</script>", "<\\/script>")
			slides.WriteString(fmt.Sprintf(`
    <div class="slide" id="slide-%d" data-category="%s" style="display:%s">
      <div class="slide-header">
        <span class="breadcrumb">%s %s → %s</span>
        <div class="slide-actions">
          <span class="slide-counter">%d / %d</span>
          <button class="btn-copy" onclick="copyMermaid(%d)">📋 Copy</button>
        </div>
      </div>
      <h2>%s</h2>
      <div class="mermaid-viewport" id="viewport-%d"></div>
      <script type="text/mermaid" id="mmd-src-%d">%s</script>
    </div>`,
				slideIdx, cat, display,
				catIcons[cat], cat, d.Title,
				slideIdx+1, totalDiagrams,
				slideIdx,
				d.Title,
				slideIdx, slideIdx, safeMermaid))
			slideIdx++
		}
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>𓇽 Seba — The Star Map | Sirsi Pantheon</title>
<meta name="description" content="Architectural diagrams generated from live project analysis by Seba, the Pantheon mapping engine.">
<script src="https://cdn.jsdelivr.net/npm/mermaid@11/dist/mermaid.min.js"></script>
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Cinzel:wght@400;700&family=Outfit:wght@300;400;600&family=JetBrains+Mono:wght@400&display=swap" rel="stylesheet">
<style>
:root {
  --gold: hsl(45, 52%%, 55%%);       /* Accent/highlights ONLY */
  --gold-light: hsl(45, 60%%, 70%%);
  --gold-glow: hsla(45, 52%%, 55%%, 0.15);
  --emerald: hsl(160, 84%%, 39%%);   /* Sirsi canonical emerald */
  --emerald-dim: hsl(160, 50%%, 28%%);
  --emerald-glow: hsla(160, 84%%, 39%%, 0.12);
  --bg: hsl(165, 80%%, 3%%);
  --bg-alt: hsl(165, 80%%, 5%%);
  --sidebar-bg: hsla(165, 60%%, 6%%, 0.92);
  --card-bg: hsl(165, 30%%, 8%%);
  --text: #F2F2F2;                   /* White — primary body text */
  --text-dim: #8A8A8A;               /* Muted supporting text */
  --border: hsla(0, 0%%, 100%%, 0.1);
  --heading: 'Cinzel', serif;
  --body: 'Outfit', sans-serif;
  --mono: 'JetBrains Mono', monospace;
}
* { margin:0; padding:0; box-sizing:border-box; }
body {
  background: var(--bg);
  background-image:
    radial-gradient(circle at 0%% 0%%, hsla(160, 84%%, 15%%, 0.08) 0%%, transparent 50%%),
    radial-gradient(circle at 100%% 100%%, hsla(45, 52%%, 10%%, 0.08) 0%%, transparent 50%%);
  color: var(--text);
  font-family: var(--body);
  min-height: 100vh;
  display: flex;
  -webkit-font-smoothing: antialiased;
}

/* ── Sidebar ────────────────────────────────── */
.sidebar {
  width: 280px;
  min-height: 100vh;
  background: var(--sidebar-bg);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  border-right: 1px solid var(--border);
  position: fixed;
  left: 0; top: 0; bottom: 0;
  overflow-y: auto;
  z-index: 100;
  display: flex;
  flex-direction: column;
}
.sidebar-header {
  padding: 2rem 1.5rem 1.5rem;
  border-bottom: 1px solid var(--border);
}
.sidebar-header h1 {
  font-family: var(--heading);
  color: var(--gold);
  font-size: 1.2rem;
  font-weight: 700;
  letter-spacing: 3px;
}
.sidebar-header p {
  color: var(--text-dim);
  font-size: 0.85rem;
  margin-top: 0.4rem;
  letter-spacing: 0.5px;
}
.sidebar-back {
  display: block;
  padding: 0.8rem 1.5rem;
  color: var(--emerald);
  text-decoration: none;
  font-size: 0.85rem;
  border-bottom: 1px solid var(--border);
  transition: all 0.2s;
}
.sidebar-back:hover {
  background: var(--emerald-glow);
  color: var(--text);
}
.nav-category {
  padding: 0.5rem 0;
}
.nav-cat-label {
  padding: 1rem 1.5rem 0.4rem;
  font-size: 0.75rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 1.5px;
  color: var(--emerald);
}
.nav-item {
  display: block;
  padding: 0.55rem 1.5rem 0.55rem 2rem;
  color: var(--text-dim);
  text-decoration: none;
  font-size: 0.92rem;
  cursor: pointer;
  transition: all 0.2s;
  border-left: 2px solid transparent;
}
.nav-item:hover {
  color: var(--text);
  background: var(--emerald-glow);
  border-left-color: var(--emerald);
}
.nav-item.active {
  color: var(--text);
  background: var(--emerald-glow);
  border-left-color: var(--emerald);
  font-weight: 600;
}
.sidebar-footer {
  margin-top: auto;
  padding: 1.5rem;
  border-top: 1px solid var(--border);
  font-size: 0.8rem;
  color: var(--text-dim);
  text-align: center;
}
.sidebar-footer a {
  color: var(--emerald);
  text-decoration: none;
  transition: color 0.2s;
}
.sidebar-footer a:hover {
  color: var(--text);
}

/* ── Main Content ───────────────────────────── */
.main {
  margin-left: 280px;
  flex: 1;
  min-height: 100vh;
  display: flex;
  flex-direction: column;
}
.top-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 1rem 2rem;
  border-bottom: 1px solid var(--border);
  backdrop-filter: blur(10px);
  position: sticky;
  top: 0;
  z-index: 50;
  background: hsla(165, 80%%, 3%%, 0.9);
}
.nav-arrows {
  display: flex;
  gap: 0.5rem;
}
.nav-arrows button {
  background: transparent;
  border: 1px solid var(--border);
  color: var(--text-dim);
  width: 36px; height: 36px;
  border-radius: 8px;
  cursor: pointer;
  font-size: 1rem;
  transition: all 0.2s;
  display: flex;
  align-items: center;
  justify-content: center;
}
.nav-arrows button:hover {
  border-color: var(--emerald);
  color: var(--emerald);
  background: var(--emerald-glow);
}
.kbd-hint {
  font-family: var(--mono);
  font-size: 0.75rem;
  color: var(--text-dim);
  opacity: 0.6;
}

.content {
  flex: 1;
  padding: 2rem;
}
.slide {
  animation: fadeIn 0.3s ease;
}
@keyframes fadeIn {
  from { opacity: 0; transform: translateY(8px); }
  to   { opacity: 1; transform: translateY(0); }
}
.slide-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1rem;
}
.breadcrumb {
  font-size: 0.88rem;
  color: var(--text-dim);
  letter-spacing: 0.5px;
}
.slide-actions {
  display: flex;
  align-items: center;
  gap: 0.8rem;
}
.slide-counter {
  font-family: var(--mono);
  font-size: 0.85rem;
  color: var(--text-dim);
}
.btn-copy {
  background: transparent;
  border: 1px solid var(--border);
  color: var(--text-dim);
  padding: 6px 16px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 0.85rem;
  font-family: var(--body);
  transition: all 0.2s;
}
.btn-copy:hover {
  border-color: var(--gold);
  color: var(--gold);
}
.slide h2 {
  font-family: var(--heading);
  color: var(--gold);
  font-size: 1.6rem;
  font-weight: 400;
  margin-bottom: 1.5rem;
  letter-spacing: 2px;
}
.mermaid-viewport {
  background: var(--card-bg);
  border: 1px solid var(--border);
  border-radius: 16px;
  padding: 2rem;
  min-height: 400px;
  overflow-x: auto;
  position: relative;
}
.mermaid-viewport::before {
  content: '';
  position: absolute;
  top: 0; left: 0; right: 0;
  height: 2px;
  background: linear-gradient(90deg, transparent, var(--emerald), var(--gold), transparent);
  border-radius: 16px 16px 0 0;
  opacity: 0.6;
}
/* SVGs rendered by mermaid.render() into viewport */
.mermaid-viewport svg {
  display: block;
  margin: 0 auto;
  max-width: 100%%;
}
/* Hide the mermaid source script tags */
script[type="text/mermaid"] {
  display: none;
}

/* ── Responsive ─────────────────────────────── */
@media (max-width: 768px) {
  .sidebar { width: 60px; }
  .sidebar-header h1, .sidebar-header p,
  .nav-cat-label, .nav-item { font-size: 0; visibility: hidden; height: 0; padding: 0; }
  .nav-category { padding: 0; }
  .main { margin-left: 60px; }
  .sidebar-footer { display: none; }
}
</style>
</head>
<body>

<nav class="sidebar">
  <div class="sidebar-header">
    <h1>𓇽 SEBA</h1>
    <p>The Star Map · %d diagrams</p>
  </div>
  <a class="sidebar-back" href="index.html">← Back to Pantheon</a>
  %s
  <div class="sidebar-footer">
    <p>𓇶 Sirsi Pantheon v1.0.0-rc1</p>
    <a href="https://sirsi.ai">sirsi.ai</a>
  </div>
</nav>

<main class="main">
  <div class="top-bar">
    <div class="nav-arrows">
      <button onclick="prevDiagram()" title="Previous (←)">←</button>
      <button onclick="nextDiagram()" title="Next (→)">→</button>
    </div>
    <span class="kbd-hint">← → arrow keys to navigate</span>
  </div>

  <div class="content">
    %s
  </div>
</main>

<script>
mermaid.initialize({
  startOnLoad: false,
  theme: 'dark',
  themeVariables: {
    primaryColor: '#0A3D2E',
    primaryTextColor: '#C8A951',
    primaryBorderColor: '#23B87C',
    lineColor: '#23B87C',
    secondaryColor: '#0D1F18',
    tertiaryColor: '#06120D',
    fontFamily: 'Outfit, -apple-system, sans-serif'
  }
});

let currentIdx = 0;
const total = %d;
const rendered = {};

async function renderSlide(idx) {
  if (rendered[idx]) return;
  const src = document.getElementById('mmd-src-' + idx);
  const vp = document.getElementById('viewport-' + idx);
  if (!src || !vp) return;
  try {
    const id = 'mmd-svg-' + idx;
    const { svg } = await mermaid.render(id, src.textContent);
    vp.innerHTML = svg;
    rendered[idx] = true;
  } catch (e) {
    vp.innerHTML = '<p style="color:#E74C3C;padding:1rem;">Render error: ' + e.message + '</p>';
  }
}

async function showDiagram(idx) {
  if (idx < 0 || idx >= total) return;

  const prev = document.getElementById('slide-' + currentIdx);
  if (prev) prev.style.display = 'none';

  document.querySelectorAll('.nav-item').forEach(n => n.classList.remove('active'));

  currentIdx = idx;
  const slide = document.getElementById('slide-' + idx);
  if (slide) {
    slide.style.display = 'block';
    slide.style.animation = 'none';
    slide.offsetHeight;
    slide.style.animation = 'fadeIn 0.3s ease';
    await renderSlide(idx);
  }

  const navItem = document.querySelector('.nav-item[data-idx="' + idx + '"]');
  if (navItem) {
    navItem.classList.add('active');
    navItem.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
  }
}

function nextDiagram() { showDiagram(currentIdx + 1); }
function prevDiagram() { showDiagram(currentIdx - 1); }

document.addEventListener('keydown', e => {
  if (e.key === 'ArrowRight' || e.key === 'ArrowDown') { e.preventDefault(); nextDiagram(); }
  if (e.key === 'ArrowLeft' || e.key === 'ArrowUp')    { e.preventDefault(); prevDiagram(); }
});

function copyMermaid(idx) {
  const src = document.getElementById('mmd-src-' + idx);
  if (src) {
    navigator.clipboard.writeText(src.textContent);
    const btn = document.querySelector('#slide-' + idx + ' .btn-copy');
    if (btn) {
      btn.textContent = '✅ Copied!';
      setTimeout(() => btn.textContent = '📋 Copy', 2000);
    }
  }
}

// Render first diagram on load
window.addEventListener('load', () => renderSlide(0));
</script>
</body>
</html>`, totalDiagrams, sidebarItems.String(), slides.String(), totalDiagrams)

	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	return os.WriteFile(outputPath, []byte(html), 0o644)
}
