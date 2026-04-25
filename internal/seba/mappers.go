// Package seba — mappers.go
//
// 𓇽 Phase 1 Universal Mappers — Static Analysis Engine
//
// These mappers work in ANY repo Pantheon attaches to.
// They use git, go/ast, filesystem, and config parsing — no runtime needed.
package seba

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func init() {
	// Register Phase 1 diagram types
	registerMapper(DiagramCallGraph, "Function Call Graph", generateCallGraph)
	registerMapper(DiagramCommandTree, "CLI Command Tree", generateCommandTree)
	registerMapper(DiagramCommandWiring, "Command → Module Wiring", generateCommandWiring)
	registerMapper(DiagramModuleDataFlow, "Per-Module Data Flow", generateModuleDataFlow)
	registerMapper(DiagramCommitHeatmap, "Commit Activity Heatmap", generateCommitHeatmap)
	registerMapper(DiagramFileHotspots, "File Change Frequency", generateFileHotspots)
	registerMapper(DiagramReleaseTimeline, "Release Timeline", generateReleaseTimeline)
	registerMapper(DiagramDepTree, "Dependency Tree", generateDepTree)
	registerMapper(DiagramCIPipeline, "CI/CD Pipeline Flow", generateCIPipeline)
}

// Phase 1 diagram types
const (
	DiagramCallGraph       DiagramType = "callgraph"
	DiagramCommandTree     DiagramType = "commandtree"
	DiagramCommandWiring   DiagramType = "commandwiring"
	DiagramModuleDataFlow  DiagramType = "moduledataflow"
	DiagramCommitHeatmap   DiagramType = "commitheatmap"
	DiagramFileHotspots    DiagramType = "filehotspots"
	DiagramReleaseTimeline DiagramType = "releasetimeline"
	DiagramDepTree         DiagramType = "deptree"
	DiagramCIPipeline      DiagramType = "cipipeline"
)

// mapperFunc is the signature for all diagram generators.
type mapperFunc func(projectRoot string) (*DiagramResult, error)

// mapperRegistry holds all registered mappers.
var mapperRegistry = map[DiagramType]struct {
	title string
	fn    mapperFunc
}{}

func registerMapper(dtype DiagramType, title string, fn mapperFunc) {
	mapperRegistry[dtype] = struct {
		title string
		fn    mapperFunc
	}{title: title, fn: fn}
}

// ── Map 2: Function Call Graph ──────────────────────────────────────
// Parses Go AST to find function-to-function calls within internal/.

func generateCallGraph(projectRoot string) (*DiagramResult, error) {
	internalDir := filepath.Join(projectRoot, "internal")
	if _, err := os.Stat(internalDir); err != nil {
		// Fall back to scanning root for Go files
		internalDir = projectRoot
	}

	type callEdge struct {
		caller string
		callee string
	}

	var edges []callEdge
	edgeSet := map[string]bool{}
	fset := token.NewFileSet()

	_ = filepath.WalkDir(internalDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil
		}

		pkg := f.Name.Name
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}

			callerName := pkg + "." + fn.Name.Name
			if fn.Recv != nil && len(fn.Recv.List) > 0 {
				callerName = pkg + "." + typeName(fn.Recv.List[0].Type) + "." + fn.Name.Name
			}

			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}

				var calleeName string
				switch fun := call.Fun.(type) {
				case *ast.SelectorExpr:
					if ident, ok := fun.X.(*ast.Ident); ok {
						calleeName = ident.Name + "." + fun.Sel.Name
					}
				case *ast.Ident:
					calleeName = pkg + "." + fun.Name
				}

				if calleeName != "" && callerName != calleeName {
					key := callerName + "|" + calleeName
					if !edgeSet[key] {
						edgeSet[key] = true
						edges = append(edges, callEdge{caller: callerName, callee: calleeName})
					}
				}
				return true
			})
		}
		return nil
	})

	// Build mermaid — limit to top 80 edges to keep readable
	if len(edges) > 80 {
		edges = edges[:80]
	}

	var sb strings.Builder
	sb.WriteString("graph LR\n")

	nodeSet := map[string]bool{}
	for _, e := range edges {
		nodeSet[e.caller] = true
		nodeSet[e.callee] = true
	}

	for node := range nodeSet {
		safe := sanitizeMermaidID(node)
		sb.WriteString(fmt.Sprintf("    %s[\"%s\"]\n", safe, node))
	}
	sb.WriteString("\n")
	for _, e := range edges {
		sb.WriteString(fmt.Sprintf("    %s --> %s\n", sanitizeMermaidID(e.caller), sanitizeMermaidID(e.callee)))
	}

	return &DiagramResult{
		Type:    DiagramCallGraph,
		Title:   "𓇽 Function Call Graph — Who Calls Whom",
		Mermaid: sb.String(),
	}, nil
}

// ── Map 11: CLI Command Tree ────────────────────────────────────────
// Scans cmd/ directory for cobra.Command declarations and builds the tree.

func generateCommandTree(projectRoot string) (*DiagramResult, error) {
	cmdDir := filepath.Join(projectRoot, "cmd")
	if _, err := os.Stat(cmdDir); err != nil {
		return nil, fmt.Errorf("no cmd/ directory found")
	}

	type cmdNode struct {
		name   string
		use    string
		short  string
		parent string
	}

	var commands []cmdNode
	fset := token.NewFileSet()

	_ = filepath.WalkDir(cmdDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil
		}

		for _, decl := range f.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range genDecl.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, val := range vs.Values {
					// Look for &cobra.Command{...}
					ue, ok := val.(*ast.UnaryExpr)
					if !ok {
						continue
					}
					cl, ok := ue.X.(*ast.CompositeLit)
					if !ok {
						continue
					}
					if !isCobraCommand(cl.Type) {
						continue
					}

					name := ""
					if i < len(vs.Names) {
						name = vs.Names[i].Name
					}
					use, short := extractCobraFields(cl)
					commands = append(commands, cmdNode{
						name:  name,
						use:   use,
						short: short,
					})
				}
			}
		}

		// Also scan init() functions for AddCommand calls
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name.Name != "init" || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "AddCommand" {
					return true
				}
				parentIdent, ok := sel.X.(*ast.Ident)
				if !ok {
					return true
				}
				for _, arg := range call.Args {
					argIdent, ok := arg.(*ast.Ident)
					if !ok {
						continue
					}
					// Record parent-child
					for ci := range commands {
						if commands[ci].name == argIdent.Name {
							commands[ci].parent = parentIdent.Name
						}
					}
				}
				return true
			})
		}

		return nil
	})

	// Build Mermaid tree
	var sb strings.Builder
	sb.WriteString("graph TD\n")

	for _, cmd := range commands {
		safe := sanitizeMermaidID(cmd.name)
		label := cmd.use
		if label == "" {
			label = cmd.name
		}
		if cmd.short != "" {
			sb.WriteString(fmt.Sprintf("    %s[\"%s<br/><small>%s</small>\"]\n", safe, label, cmd.short))
		} else {
			sb.WriteString(fmt.Sprintf("    %s[\"%s\"]\n", safe, label))
		}
	}
	sb.WriteString("\n")

	for _, cmd := range commands {
		if cmd.parent != "" {
			sb.WriteString(fmt.Sprintf("    %s --> %s\n",
				sanitizeMermaidID(cmd.parent),
				sanitizeMermaidID(cmd.name)))
		}
	}

	return &DiagramResult{
		Type:    DiagramCommandTree,
		Title:   "𓇽 CLI Command Tree — Full Command Hierarchy",
		Mermaid: sb.String(),
	}, nil
}

// ── Map 12: Command → Module Wiring ─────────────────────────────────

func generateCommandWiring(projectRoot string) (*DiagramResult, error) {
	cmdDir := filepath.Join(projectRoot, "cmd")
	if _, err := os.Stat(cmdDir); err != nil {
		return nil, fmt.Errorf("no cmd/ directory found")
	}

	type wireEdge struct {
		cmdFile string
		module  string
	}

	var edges []wireEdge
	fset := token.NewFileSet()
	modulePrefix := detectModulePrefix(projectRoot)

	_ = filepath.WalkDir(cmdDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return nil
		}

		cmdName := strings.TrimSuffix(filepath.Base(path), ".go")

		for _, imp := range f.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			if modulePrefix != "" && strings.Contains(importPath, "internal/") {
				parts := strings.Split(importPath, "internal/")
				if len(parts) >= 2 {
					mod := strings.Split(parts[1], "/")[0]
					edges = append(edges, wireEdge{cmdFile: cmdName, module: mod})
				}
			}
		}
		return nil
	})

	var sb strings.Builder
	sb.WriteString("graph LR\n")
	sb.WriteString("    subgraph CLI[\"🏛️ cmd/sirsi/\"]\n")

	cmdSet := map[string]bool{}
	for _, e := range edges {
		if !cmdSet[e.cmdFile] {
			cmdSet[e.cmdFile] = true
			sb.WriteString(fmt.Sprintf("        cmd_%s[\"%s.go\"]\n", e.cmdFile, e.cmdFile))
		}
	}
	sb.WriteString("    end\n")

	sb.WriteString("    subgraph Modules[\"📦 internal/\"]\n")
	modSet := map[string]bool{}
	for _, e := range edges {
		if !modSet[e.module] {
			modSet[e.module] = true
			sb.WriteString(fmt.Sprintf("        mod_%s[\"%s\"]\n", e.module, e.module))
		}
	}
	sb.WriteString("    end\n\n")

	edgeSet := map[string]bool{}
	for _, e := range edges {
		key := e.cmdFile + "|" + e.module
		if !edgeSet[key] {
			edgeSet[key] = true
			sb.WriteString(fmt.Sprintf("    cmd_%s --> mod_%s\n", e.cmdFile, e.module))
		}
	}

	return &DiagramResult{
		Type:    DiagramCommandWiring,
		Title:   "𓇽 Command → Module Wiring — CLI Import Graph",
		Mermaid: sb.String(),
	}, nil
}

// ── Map 21: Per-Module Data Flow ────────────────────────────────────

func generateModuleDataFlow(projectRoot string) (*DiagramResult, error) {
	internalDir := filepath.Join(projectRoot, "internal")
	entries, err := os.ReadDir(internalDir)
	if err != nil {
		return nil, fmt.Errorf("no internal/ directory: %w", err)
	}

	type moduleIO struct {
		name    string
		reads   []string // File/config reads
		writes  []string // File/config writes
		execs   []string // exec.Command calls
		network bool     // net/http usage
	}

	var modules []moduleIO
	fset := token.NewFileSet()

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		modName := entry.Name()
		modDir := filepath.Join(internalDir, modName)
		mio := moduleIO{name: modName}

		goFiles, _ := filepath.Glob(filepath.Join(modDir, "*.go"))
		for _, gf := range goFiles {
			if strings.HasSuffix(gf, "_test.go") {
				continue
			}
			f, err := parser.ParseFile(fset, gf, nil, 0)
			if err != nil {
				continue
			}

			// Check imports for I/O patterns
			for _, imp := range f.Imports {
				path := strings.Trim(imp.Path.Value, `"`)
				switch {
				case path == "net/http" || path == "net":
					mio.network = true
				case path == "os/exec":
					mio.execs = appendUnique(mio.execs, "exec")
				}
			}

			// Scan for os.ReadFile, os.WriteFile, os.Open, os.Create, exec.Command
			ast.Inspect(f, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				ident, ok := sel.X.(*ast.Ident)
				if !ok {
					return true
				}

				switch {
				case ident.Name == "os" && (sel.Sel.Name == "ReadFile" || sel.Sel.Name == "Open" || sel.Sel.Name == "ReadDir"):
					mio.reads = appendUnique(mio.reads, "filesystem")
				case ident.Name == "os" && (sel.Sel.Name == "WriteFile" || sel.Sel.Name == "Create" || sel.Sel.Name == "MkdirAll"):
					mio.writes = appendUnique(mio.writes, "filesystem")
				case ident.Name == "exec" && sel.Sel.Name == "Command":
					mio.execs = appendUnique(mio.execs, "subprocess")
				case ident.Name == "json" && sel.Sel.Name == "Marshal":
					mio.writes = appendUnique(mio.writes, "json")
				case ident.Name == "json" && sel.Sel.Name == "Unmarshal":
					mio.reads = appendUnique(mio.reads, "json")
				case ident.Name == "yaml" && sel.Sel.Name == "Marshal":
					mio.writes = appendUnique(mio.writes, "yaml")
				case ident.Name == "yaml" && sel.Sel.Name == "Unmarshal":
					mio.reads = appendUnique(mio.reads, "yaml")
				case ident.Name == "http" && (sel.Sel.Name == "Get" || sel.Sel.Name == "Post" || sel.Sel.Name == "Do"):
					mio.network = true
				}
				return true
			})
		}

		if len(mio.reads) > 0 || len(mio.writes) > 0 || len(mio.execs) > 0 || mio.network {
			modules = append(modules, mio)
		}
	}

	var sb strings.Builder
	sb.WriteString("graph LR\n")

	// Data sources
	sb.WriteString("    FS[(\"📁 Filesystem\")]\n")
	sb.WriteString("    JSON[(\"📋 JSON\")]\n")
	sb.WriteString("    YAML[(\"⚙️ YAML\")]\n")
	sb.WriteString("    NET[(\"🌐 Network\")]\n")
	sb.WriteString("    SUB[(\"⚡ Subprocess\")]\n\n")

	for _, m := range modules {
		id := sanitizeMermaidID(m.name)
		sb.WriteString(fmt.Sprintf("    %s[\"%s\"]\n", id, m.name))
	}
	sb.WriteString("\n")

	for _, m := range modules {
		id := sanitizeMermaidID(m.name)
		for _, r := range m.reads {
			switch r {
			case "filesystem":
				sb.WriteString(fmt.Sprintf("    FS -->|read| %s\n", id))
			case "json":
				sb.WriteString(fmt.Sprintf("    JSON -->|parse| %s\n", id))
			case "yaml":
				sb.WriteString(fmt.Sprintf("    YAML -->|parse| %s\n", id))
			}
		}
		for _, w := range m.writes {
			switch w {
			case "filesystem":
				sb.WriteString(fmt.Sprintf("    %s -->|write| FS\n", id))
			case "json":
				sb.WriteString(fmt.Sprintf("    %s -->|encode| JSON\n", id))
			case "yaml":
				sb.WriteString(fmt.Sprintf("    %s -->|encode| YAML\n", id))
			}
		}
		for _, e := range m.execs {
			if e == "subprocess" {
				sb.WriteString(fmt.Sprintf("    %s -->|exec| SUB\n", id))
			}
		}
		if m.network {
			sb.WriteString(fmt.Sprintf("    %s <-->|http| NET\n", id))
		}
	}

	return &DiagramResult{
		Type:    DiagramModuleDataFlow,
		Title:   "𓇽 Per-Module Data Flow — I/O Analysis",
		Mermaid: sb.String(),
	}, nil
}

// ── Map 46: Commit Activity Heatmap ─────────────────────────────────

func generateCommitHeatmap(projectRoot string) (*DiagramResult, error) {
	cmd := exec.Command("git", "log", "--format=%aI", "--since=90 days ago")
	cmd.Dir = projectRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git log: %w", err)
	}

	dayCount := map[string]int{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if len(line) < 10 {
			continue
		}
		day := line[:10] // YYYY-MM-DD
		dayCount[day]++
	}

	// Sort days
	var days []string
	for d := range dayCount {
		days = append(days, d)
	}
	sort.Strings(days)

	// Build a Mermaid xychart
	var sb strings.Builder
	sb.WriteString("xychart-beta\n")
	sb.WriteString("    title \"Commit Activity (Last 90 Days)\"\n")
	sb.WriteString("    x-axis [")
	labels := make([]string, len(days))
	for i, d := range days {
		labels[i] = fmt.Sprintf("\"%s\"", d[5:]) // MM-DD
	}
	sb.WriteString(strings.Join(labels, ", "))
	sb.WriteString("]\n")
	sb.WriteString("    y-axis \"Commits\"\n")
	sb.WriteString("    bar [")
	values := make([]string, len(days))
	for i, d := range days {
		values[i] = strconv.Itoa(dayCount[d])
	}
	sb.WriteString(strings.Join(values, ", "))
	sb.WriteString("]\n")

	return &DiagramResult{
		Type:    DiagramCommitHeatmap,
		Title:   "𓇽 Commit Activity — Last 90 Days",
		Mermaid: sb.String(),
	}, nil
}

// ── Map 48: File Change Frequency ───────────────────────────────────

func generateFileHotspots(projectRoot string) (*DiagramResult, error) {
	cmd := exec.Command("git", "log", "--name-only", "--format=", "--since=90 days ago")
	cmd.Dir = projectRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git log: %w", err)
	}

	fileCount := map[string]int{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fileCount[line]++
	}

	// Sort by change count descending
	type fileFreq struct {
		path  string
		count int
	}
	var sorted []fileFreq
	for f, c := range fileCount {
		sorted = append(sorted, fileFreq{f, c})
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].count > sorted[j].count })

	// Top 25
	if len(sorted) > 25 {
		sorted = sorted[:25]
	}

	var sb strings.Builder
	sb.WriteString("xychart-beta\n")
	sb.WriteString("    title \"Most Changed Files (Last 90 Days)\"\n")
	sb.WriteString("    x-axis [")
	labels := make([]string, len(sorted))
	for i, f := range sorted {
		base := filepath.Base(f.path)
		if len(base) > 20 {
			base = base[:17] + "..."
		}
		labels[i] = fmt.Sprintf("\"%s\"", base)
	}
	sb.WriteString(strings.Join(labels, ", "))
	sb.WriteString("]\n")
	sb.WriteString("    y-axis \"Changes\"\n")
	sb.WriteString("    bar [")
	values := make([]string, len(sorted))
	for i, f := range sorted {
		values[i] = strconv.Itoa(f.count)
	}
	sb.WriteString(strings.Join(values, ", "))
	sb.WriteString("]\n")

	return &DiagramResult{
		Type:    DiagramFileHotspots,
		Title:   "𓇽 File Hotspots — Most Changed Files",
		Mermaid: sb.String(),
	}, nil
}

// ── Map 51: Release Timeline ────────────────────────────────────────

func generateReleaseTimeline(projectRoot string) (*DiagramResult, error) {
	cmd := exec.Command("git", "tag", "-l", "--sort=-creatordate",
		"--format=%(refname:short)|%(creatordate:short)")
	cmd.Dir = projectRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git tag: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("timeline\n")
	sb.WriteString("    title Release History\n")

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, "|", 2)
		if len(parts) < 2 || parts[0] == "" {
			continue
		}
		tag := parts[0]
		date := parts[1]
		sb.WriteString(fmt.Sprintf("    %s : %s\n", date, tag))
	}

	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		sb.WriteString("    No releases : No git tags found\n")
	}

	return &DiagramResult{
		Type:    DiagramReleaseTimeline,
		Title:   "𓇽 Release Timeline — Version History",
		Mermaid: sb.String(),
	}, nil
}

// ── Map 53: Dependency Tree ─────────────────────────────────────────

func generateDepTree(projectRoot string) (*DiagramResult, error) {
	// Try go mod graph first
	cmd := exec.Command("go", "mod", "graph")
	cmd.Dir = projectRoot
	out, err := cmd.Output()
	if err != nil {
		// Fall back to reading go.mod directly
		return generateDepTreeFromGoMod(projectRoot)
	}

	var sb strings.Builder
	sb.WriteString("graph TD\n")

	nodeSet := map[string]bool{}
	edgeCount := 0
	maxEdges := 60 // Keep readable

	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if edgeCount >= maxEdges {
			break
		}
		parts := strings.Fields(line)
		if len(parts) != 2 {
			continue
		}

		from := simplifyModName(parts[0])
		to := simplifyModName(parts[1])

		if !nodeSet[from] {
			nodeSet[from] = true
			sb.WriteString(fmt.Sprintf("    %s[\"%s\"]\n", sanitizeMermaidID(from), from))
		}
		if !nodeSet[to] {
			nodeSet[to] = true
			sb.WriteString(fmt.Sprintf("    %s[\"%s\"]\n", sanitizeMermaidID(to), to))
		}
		sb.WriteString(fmt.Sprintf("    %s --> %s\n", sanitizeMermaidID(from), sanitizeMermaidID(to)))
		edgeCount++
	}

	return &DiagramResult{
		Type:    DiagramDepTree,
		Title:   "𓇽 Dependency Tree — go mod graph",
		Mermaid: sb.String(),
	}, nil
}

func generateDepTreeFromGoMod(projectRoot string) (*DiagramResult, error) {
	gomodPath := filepath.Join(projectRoot, "go.mod")
	data, err := os.ReadFile(gomodPath)
	if err != nil {
		return nil, fmt.Errorf("no go.mod: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("graph TD\n")
	sb.WriteString("    root[\"This Module\"]\n")

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "require") || strings.HasPrefix(line, ")") || strings.HasPrefix(line, "(") || line == "" {
			continue
		}
		if strings.HasPrefix(line, "module ") || strings.HasPrefix(line, "go ") {
			continue
		}
		// Parse: github.com/foo/bar v1.2.3
		parts := strings.Fields(line)
		if len(parts) >= 2 && strings.Contains(parts[0], "/") {
			dep := simplifyModName(parts[0])
			id := sanitizeMermaidID(dep)
			sb.WriteString(fmt.Sprintf("    %s[\"%s<br/>%s\"]\n", id, dep, parts[1]))
			sb.WriteString(fmt.Sprintf("    root --> %s\n", id))
		}
	}

	return &DiagramResult{
		Type:    DiagramDepTree,
		Title:   "𓇽 Dependency Tree — go.mod",
		Mermaid: sb.String(),
	}, nil
}

// ── Map 58: CI/CD Pipeline Flow ─────────────────────────────────────

func generateCIPipeline(projectRoot string) (*DiagramResult, error) {
	// Scan for CI config files
	ciFiles := []struct {
		path   string
		system string
	}{
		{".github/workflows", "GitHub Actions"},
		{".gitlab-ci.yml", "GitLab CI"},
		{".circleci/config.yml", "CircleCI"},
		{"Jenkinsfile", "Jenkins"},
		{".travis.yml", "Travis CI"},
		{"bitbucket-pipelines.yml", "Bitbucket"},
	}

	var found string
	var system string
	for _, cf := range ciFiles {
		p := filepath.Join(projectRoot, cf.path)
		if _, err := os.Stat(p); err == nil {
			found = p
			system = cf.system
			break
		}
	}

	if found == "" {
		return nil, fmt.Errorf("no CI configuration found")
	}

	// For GitHub Actions, parse workflow YAML files
	if system == "GitHub Actions" {
		return parseGitHubActionsWorkflows(found)
	}

	// Generic fallback
	var sb strings.Builder
	sb.WriteString("graph LR\n")
	sb.WriteString(fmt.Sprintf("    CI[\"%s\"]\n", system))
	sb.WriteString(fmt.Sprintf("    Config[\"%s\"]\n", filepath.Base(found)))
	sb.WriteString("    CI --> Config\n")

	return &DiagramResult{
		Type:    DiagramCIPipeline,
		Title:   fmt.Sprintf("𓇽 CI/CD Pipeline — %s", system),
		Mermaid: sb.String(),
	}, nil
}

func parseGitHubActionsWorkflows(workflowDir string) (*DiagramResult, error) {
	entries, err := os.ReadDir(workflowDir)
	if err != nil {
		return nil, fmt.Errorf("read workflows: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("graph TD\n")
	sb.WriteString("    Push[\"🔀 Push / PR\"]\n\n")

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".yml") && !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(workflowDir, entry.Name()))
		if err != nil {
			continue
		}

		wfName := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		wfID := sanitizeMermaidID(wfName)
		sb.WriteString(fmt.Sprintf("    subgraph %s[\"%s\"]\n", wfID, wfName))

		// Simple YAML parsing — look for job names and steps
		var currentJob string
		for _, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)

			// Detect job definitions (lines under 'jobs:' with proper indent)
			if len(line) > 2 && line[0:2] == "  " && line[2] != ' ' && strings.HasSuffix(trimmed, ":") && !strings.Contains(trimmed, " ") {
				jobName := strings.TrimSuffix(trimmed, ":")
				if jobName != "jobs" && jobName != "on" && jobName != "name" && jobName != "env" && jobName != "permissions" {
					currentJob = jobName
					jobID := sanitizeMermaidID(wfName + "_" + jobName)
					sb.WriteString(fmt.Sprintf("        %s[\"%s\"]\n", jobID, jobName))
				}
			}

			// Detect 'needs:' for job dependencies
			if currentJob != "" && strings.HasPrefix(trimmed, "needs:") {
				needsStr := strings.TrimPrefix(trimmed, "needs:")
				needsStr = strings.TrimSpace(needsStr)
				needsStr = strings.Trim(needsStr, "[]")
				for _, need := range strings.Split(needsStr, ",") {
					need = strings.TrimSpace(need)
					need = strings.Trim(need, "\"' ")
					if need != "" {
						fromID := sanitizeMermaidID(wfName + "_" + need)
						toID := sanitizeMermaidID(wfName + "_" + currentJob)
						sb.WriteString(fmt.Sprintf("        %s --> %s\n", fromID, toID))
					}
				}
			}
		}

		sb.WriteString("    end\n")
		sb.WriteString(fmt.Sprintf("    Push --> %s\n\n", wfID))
	}

	return &DiagramResult{
		Type:    DiagramCIPipeline,
		Title:   "𓇽 CI/CD Pipeline — GitHub Actions Workflows",
		Mermaid: sb.String(),
	}, nil
}

// ── Shared Helpers ──────────────────────────────────────────────────

func sanitizeMermaidID(s string) string {
	r := strings.NewReplacer(
		".", "_", "/", "_", "-", "_", "@", "_",
		" ", "_", "(", "_", ")", "_", ":", "_",
		"*", "ptr", "&", "ref",
	)
	id := r.Replace(s)
	// Ensure starts with letter
	if len(id) > 0 && id[0] >= '0' && id[0] <= '9' {
		id = "n" + id
	}
	return id
}

func typeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return typeName(t.X)
	case *ast.SelectorExpr:
		return typeName(t.X) + "_" + t.Sel.Name
	default:
		return "T"
	}
}

func isCobraCommand(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return ident.Name == "cobra" && sel.Sel.Name == "Command"
}

func extractCobraFields(cl *ast.CompositeLit) (use, short string) {
	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		val, ok := kv.Value.(*ast.BasicLit)
		if !ok {
			continue
		}
		switch key.Name {
		case "Use":
			use = strings.Trim(val.Value, `"`)
		case "Short":
			short = strings.Trim(val.Value, `"`)
			if len(short) > 50 {
				short = short[:47] + "..."
			}
		}
	}
	return
}

func simplifyModName(fullPath string) string {
	// Remove version: github.com/foo/bar@v1.2.3 → foo/bar
	if idx := strings.Index(fullPath, "@"); idx != -1 {
		fullPath = fullPath[:idx]
	}
	// Shorten: github.com/SirsiMaster/sirsi-sirsi → sirsi-pantheon
	parts := strings.Split(fullPath, "/")
	if len(parts) >= 3 {
		return strings.Join(parts[2:], "/")
	}
	return fullPath
}

func detectModulePrefix(projectRoot string) string {
	gomod := filepath.Join(projectRoot, "go.mod")
	data, err := os.ReadFile(gomod)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return ""
}

func appendUnique(slice []string, item string) []string {
	for _, s := range slice {
		if s == item {
			return slice
		}
	}
	return append(slice, item)
}
