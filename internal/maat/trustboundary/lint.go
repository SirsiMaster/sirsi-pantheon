// Package trustboundary is Ma'at's Trust-Boundary Law lint (ADR-076).
//
// It is a set of cheap heuristics over Go (go/ast) and TypeScript/shell (regex)
// that flag the recurring defect classes A–H the 2026-10-05/06 security sweep
// caught in every agent commit. A finding names its rule letter; a line (or
// the line above it) carrying `trust-boundary: <reason>` silences it. The
// lint is a tripwire for review, not a proof of absence — the checklist in
// ADR-076 is still reported item by item.
package trustboundary

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Finding is one flagged line.
type Finding struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Rule    string `json:"rule"` // A–H
	Message string `json:"message"`
}

func (f Finding) String() string {
	return fmt.Sprintf("%s:%d [%s] %s", f.File, f.Line, f.Rule, f.Message)
}

// Allowlist marker: a line, or the line immediately above, carrying this
// token silences every finding on that line. The reason after the colon is
// the record a reviewer reads.
const allowMarker = "trust-boundary:"

// gateDirective declares, per file, the gate functions every request-handling
// function in that file must call (rule D). Example:
//
//	// trust-boundary-gate: RequireDeliveryReview, CheckEmailOptOut, AllowedRedirect
const gateDirective = "trust-boundary-gate:"

// Rule messages — one voice per letter so reports read consistently.
var ruleText = map[string]string{
	"A": "loop/allocation bound derived from a client-supplied number; take bounds from server constants or contiguous validated input",
	"B": "per-item output emission inside a loop; cap pages/bytes from a server constant and return a typed refusal above it",
	"C": "client-supplied provenance field (hash/receipt/signature); the server attests provenance — never trust a hash the client sends with the bytes it also sends",
	"D": "request path does not call the sibling gate(s) declared for this file; a new path through a resource calls the same gate functions as existing paths",
	"E": "merge-write on a persisted map; replace the whole map so stale keys from a prior version cannot survive",
	"F": "sensitive field written to a document; classify every stored field by reader — PII is server-only and encrypted, never on a client-readable path",
	"G": "value normalised into a different meaning or accepted input dropped silently; refuse with a typed error instead",
	"H": "process/algorithm defect: verifier exit status masked by a pipe, depth cap that rejects valid input, or self-exemption",
}

var (
	goExts   = map[string]bool{".go": true}
	tsExts   = map[string]bool{".ts": true, ".tsx": true, ".js": true, ".mjs": true, ".cjs": true, ".jsx": true}
	shExts   = map[string]bool{".sh": true, ".bash": true, ".zsh": true, ".yml": true, ".yaml": true}
	skipDirs = map[string]bool{".git": true, "node_modules": true, "vendor": true, "dist": true, "build": true,
		"testdata": true, ".claude": true, "gen": true, "coverage": true, "ios": true, "android": true}
)

// LintTree lints every supported file under root.
func LintTree(root string) ([]Finding, error) {
	var paths []string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != root && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		paths = append(paths, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return LintFiles(root, paths)
}

// LintFiles lints the given root-relative paths; unsupported or missing files
// are skipped. Generated and test files are skipped: they are not request paths.
func LintFiles(root string, paths []string) ([]Finding, error) {
	var out []Finding
	for _, rel := range paths {
		if !Lintable(rel) {
			continue
		}
		abs := filepath.Join(root, rel)
		src, err := os.ReadFile(abs)
		if err != nil {
			if os.IsNotExist(err) {
				continue // deleted in the range
			}
			return nil, err
		}
		out = append(out, lintSource(rel, src)...)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].Line < out[j].Line
	})
	return out, nil
}

// Lintable reports whether the lint has anything to say about a path.
func Lintable(rel string) bool {
	base := filepath.Base(rel)
	ext := filepath.Ext(base)
	if strings.HasSuffix(base, "_test.go") || strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") ||
		strings.HasSuffix(base, ".gen.ts") || strings.HasSuffix(base, ".pb.go") || strings.HasSuffix(base, ".connect.go") ||
		strings.HasSuffix(base, ".d.ts") {
		return false
	}
	for _, seg := range strings.Split(filepath.ToSlash(rel), "/") {
		if skipDirs[seg] {
			return false
		}
	}
	if base == "pre-push" || base == "pre-commit" || base == "pre-receive" {
		return true
	}
	return goExts[ext] || tsExts[ext] || shExts[ext]
}

func lintSource(rel string, src []byte) []Finding {
	lines := strings.Split(string(src), "\n")
	var raw []Finding
	ext := filepath.Ext(rel)
	switch {
	case goExts[ext]:
		raw = lintGo(rel, src, lines)
	case tsExts[ext]:
		raw = lintTS(rel, lines)
	default:
		raw = lintShell(rel, lines)
	}
	var out []Finding
	for _, f := range raw {
		if allowed(lines, f.Line) {
			continue
		}
		out = append(out, f)
	}
	return out
}

func allowed(lines []string, line int) bool {
	for _, i := range []int{line - 1, line - 2} { // this line, the one above (1-based → 0-based)
		if i >= 0 && i < len(lines) && strings.Contains(lines[i], allowMarker) {
			return true
		}
	}
	return false
}

func gatesDeclared(lines []string) []string {
	for _, l := range lines {
		if i := strings.Index(l, gateDirective); i >= 0 {
			var names []string
			for _, n := range strings.Split(l[i+len(gateDirective):], ",") {
				if n = strings.TrimSpace(n); n != "" {
					names = append(names, n)
				}
			}
			return names
		}
	}
	return nil
}

// ── shared vocabularies ─────────────────────────────────────────────────────

var (
	provenanceField = regexp.MustCompile(`(?i)(hash|receipt|checksum|digest|signature|provenance|attest|generatedby)`)
	requestType     = regexp.MustCompile(`(Request|Req|Input|Body|Payload|Params|Submission)$`)
	sensitiveKey    = regexp.MustCompile(`(?i)^(ssn|social_?security\w*|tax_?id|ein|dob|date_?of_?birth|birth_?date|answers|form_?answers|form_?data|passport\w*|drivers?_?licen[cs]e\w*|bank_?account\w*|account_?number|routing_?number|medical\w*|diagnosis|hipaa\w*|password|pin|card_?number)$`)
	moneyName       = regexp.MustCompile(`(?i)(amount|money|price|cents|total|balance|dollar|fee|cost|usd|payment|refund|charge)`)
	depthName       = regexp.MustCompile(`(?i)(depth|level|nesting)`)
	pageEmitters    = map[string]bool{"InsertPages": true, "ImportPages": true, "AddPages": true, "AppendPages": true, "CopyPages": true,
		"InsertPage": true, "ImportPage": true, "AddPage": true, "AppendPage": true, "MergeFile": true, "AppendFile": true}
	// Request-shaped parameter types that mark a function as a request path (rule D).
	requestParamType = regexp.MustCompile(`http\.Request|http\.ResponseWriter|connect\.Request|gin\.Context|echo\.Context|context\.Context`)
	depthCap         = 64
)

// ── Go ──────────────────────────────────────────────────────────────────────

func lintGo(rel string, src []byte, lines []string) []Finding {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, rel, src, parser.ParseComments)
	if err != nil {
		return nil // unparsable Go is the compiler's problem, not ours
	}
	var out []Finding
	add := func(pos token.Pos, rule string, extra string) {
		msg := ruleText[rule]
		if extra != "" {
			msg = extra + " — " + msg
		}
		out = append(out, Finding{File: rel, Line: fset.Position(pos).Line, Rule: rule, Message: msg})
	}
	gates := gatesDeclared(lines)
	gateSet := map[string]bool{}
	for _, g := range gates {
		gateSet[g] = true
	}

	// C: request structs carrying provenance fields.
	ast.Inspect(file, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok || !requestType.MatchString(ts.Name.Name) {
			return true
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			return true
		}
		for _, f := range st.Fields.List {
			name := ""
			if len(f.Names) > 0 {
				name = f.Names[0].Name
			}
			tag := ""
			if f.Tag != nil {
				tag = f.Tag.Value
			}
			if provenanceField.MatchString(name) || provenanceField.MatchString(tag) {
				add(f.Pos(), "C", ts.Name.Name+"."+name)
			}
		}
		return true
	})

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		lintGoFunc(fn, add)
		// D: declared gates must be called from every request-shaped function.
		if len(gates) > 0 && !gateSet[fn.Name.Name] && isRequestFunc(fn) {
			called := map[string]bool{}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if c, ok := n.(*ast.CallExpr); ok {
					called[calleeName(c)] = true
				}
				return true
			})
			var missing []string
			for _, g := range gates {
				if !called[g] {
					missing = append(missing, g)
				}
			}
			if len(missing) > 0 {
				add(fn.Pos(), "D", fn.Name.Name+" skips "+strings.Join(missing, ", "))
			}
		}
	}
	return out
}

func isRequestFunc(fn *ast.FuncDecl) bool {
	for _, p := range fn.Type.Params.List {
		if requestParamType.MatchString(exprString(p.Type)) {
			return true
		}
	}
	return false
}

func exprString(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + exprString(t.X)
	case *ast.SelectorExpr:
		return exprString(t.X) + "." + t.Sel.Name
	case *ast.IndexExpr:
		return exprString(t.X) + "[" + exprString(t.Index) + "]"
	case *ast.ArrayType:
		return "[]" + exprString(t.Elt)
	case *ast.MapType:
		return "map[" + exprString(t.Key) + "]" + exprString(t.Value)
	}
	return ""
}

func calleeName(c *ast.CallExpr) string {
	switch f := c.Fun.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		return f.Sel.Name
	}
	return ""
}

func isStrconvParse(c *ast.CallExpr) bool {
	sel, ok := c.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok || pkg.Name != "strconv" {
		return false
	}
	switch sel.Sel.Name {
	case "Atoi", "ParseInt", "ParseUint", "ParseFloat":
		return true
	}
	return false
}

func identsIn(e ast.Expr) []string {
	var names []string
	ast.Inspect(e, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok {
			names = append(names, id.Name)
		}
		return true
	})
	return names
}

func lintGoFunc(fn *ast.FuncDecl, add func(token.Pos, string, string)) {
	// A: taint identifiers assigned from strconv parses, propagate through
	// plain assignments (covers `if n > max { max = n }`), then flag bounds.
	tainted := map[string]bool{}
	assigns := func(lhs []ast.Expr, rhs []ast.Expr) {
		for i, l := range lhs {
			id, ok := l.(*ast.Ident)
			if !ok || id.Name == "_" {
				continue
			}
			var r ast.Expr
			if len(rhs) == len(lhs) {
				r = rhs[i]
			} else if len(rhs) == 1 {
				r = rhs[0]
			}
			if r == nil {
				continue
			}
			if c, ok := r.(*ast.CallExpr); ok && isStrconvParse(c) {
				tainted[id.Name] = true
				continue
			}
			for _, n := range identsIn(r) {
				if tainted[n] {
					tainted[id.Name] = true
				}
			}
		}
	}
	for pass := 0; pass < 3; pass++ { // ponytail: 3 fixed passes cover a→b→c chains; a worklist if real code nests deeper
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			switch s := n.(type) {
			case *ast.AssignStmt:
				assigns(s.Lhs, s.Rhs)
			case *ast.ValueSpec:
				var lhs []ast.Expr
				for _, nm := range s.Names {
					lhs = append(lhs, nm)
				}
				assigns(lhs, s.Values)
			}
			return true
		})
	}
	anyTainted := func(e ast.Expr) bool {
		for _, n := range identsIn(e) {
			if tainted[n] {
				return true
			}
		}
		return false
	}

	var loopDepth int
	var walk func(n ast.Node)
	walk = func(n ast.Node) {
		ast.Inspect(n, func(n ast.Node) bool {
			switch s := n.(type) {
			case *ast.ForStmt:
				if s.Cond != nil && anyTainted(s.Cond) {
					add(s.Pos(), "A", "loop condition uses a parsed client number")
				}
				loopDepth++
				walk(s.Body)
				loopDepth--
				return false
			case *ast.RangeStmt:
				if _, isIdent := s.X.(*ast.Ident); isIdent && anyTainted(s.X) {
					add(s.Pos(), "A", "range over a parsed client number")
				}
				loopDepth++
				walk(s.Body)
				loopDepth--
				return false
			case *ast.CallExpr:
				name := calleeName(s)
				if name == "make" && len(s.Args) >= 2 {
					for _, a := range s.Args[1:] {
						if anyTainted(a) {
							add(s.Pos(), "A", "make() sized by a parsed client number")
							break
						}
					}
				}
				if loopDepth > 0 && pageEmitters[name] {
					add(s.Pos(), "B", name+" inside a loop")
				}
				if name == "Set" || name == "Create" || name == "Update" {
					lintFirestoreWrite(s, add)
				}
				// G: non-digits stripped from a money-named value ('-500' → '500').
				if name == "ReplaceAllString" && len(s.Args) == 2 {
					if lit, ok := s.Args[1].(*ast.BasicLit); ok && lit.Value == `""` {
						for _, id := range identsIn(s.Args[0]) {
							if moneyName.MatchString(id) {
								add(s.Pos(), "G", "non-digits stripped from "+id)
								break
							}
						}
					}
				}
			case *ast.BinaryExpr:
				if s.Op == token.GTR || s.Op == token.GEQ || s.Op == token.LSS || s.Op == token.LEQ {
					if lit, name, ok := depthCompare(s); ok {
						add(s.Pos(), "H", fmt.Sprintf("%s capped at %d", name, lit))
					}
				}
			}
			return true
		})
	}
	walk(fn.Body)
}

func depthCompare(b *ast.BinaryExpr) (int, string, bool) {
	try := func(x, y ast.Expr) (int, string, bool) {
		id, ok := x.(*ast.Ident)
		if !ok || !depthName.MatchString(id.Name) {
			return 0, "", false
		}
		lit, ok := y.(*ast.BasicLit)
		if !ok || lit.Kind != token.INT {
			return 0, "", false
		}
		n, err := strconv.Atoi(lit.Value)
		if err != nil || n <= 0 || n >= depthCap {
			return 0, "", false
		}
		return n, id.Name, true
	}
	if n, name, ok := try(b.X, b.Y); ok {
		return n, name, true
	}
	return try(b.Y, b.X)
}

// lintFirestoreWrite covers E (MergeAll on a map) and F (sensitive keys in a
// document write) for Set/Create/Update calls.
func lintFirestoreWrite(c *ast.CallExpr, add func(token.Pos, string, string)) {
	hasMergeAll := false
	for _, a := range c.Args {
		if sel, ok := a.(*ast.SelectorExpr); ok && sel.Sel.Name == "MergeAll" {
			hasMergeAll = true
		}
	}
	for _, a := range c.Args {
		cl, ok := a.(*ast.CompositeLit)
		if !ok {
			continue
		}
		_, isMap := cl.Type.(*ast.MapType)
		if hasMergeAll && isMap {
			add(c.Pos(), "E", "firestore.MergeAll with a map value")
		}
		for _, el := range cl.Elts {
			kv, ok := el.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			if lit, ok := kv.Key.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				if key, err := strconv.Unquote(lit.Value); err == nil && sensitiveKey.MatchString(key) {
					add(kv.Pos(), "F", "field "+strconv.Quote(key))
				}
			}
		}
	}
	if hasMergeAll {
		// Map held in a variable: `data := map[string]any{...}; ref.Set(ctx, data, firestore.MergeAll)`
		for _, a := range c.Args {
			if id, ok := a.(*ast.Ident); ok && (strings.Contains(strings.ToLower(id.Name), "map") || strings.HasSuffix(id.Name, "data") || strings.HasSuffix(id.Name, "Data") || strings.HasSuffix(id.Name, "fields") || strings.HasSuffix(id.Name, "Fields")) {
				add(c.Pos(), "E", "firestore.MergeAll with "+id.Name)
			}
		}
	}
}

// ── TypeScript / JavaScript (regex heuristics) ──────────────────────────────

var (
	tsKeysNumber   = regexp.MustCompile(`(?:const|let|var)\s+(\w+)\s*=\s*[^;\n]*Object\.keys\([^)]*\)[^;\n]*\.map\(\s*(?:Number|parseInt|\(?\s*\w+\s*\)?\s*=>\s*(?:Number|parseInt))`)
	tsForIn        = regexp.MustCompile(`for\s*\(\s*(?:const|let|var)\s+(\w+)\s+in\s+`)
	tsKeysCallback = regexp.MustCompile(`Object\.(?:keys|entries)\([^)]*\)\s*\.\s*(?:forEach|map|reduce|some|every|filter)\(\s*\(?\s*\[?\s*(\w+)`)
	tsNumberOf     = regexp.MustCompile(`(?:const|let|var)\s+(\w+)\s*=\s*(?:Number|parseInt|parseFloat)\(\s*(\w+)`)
	tsAssignFrom   = regexp.MustCompile(`\b(\w+)\s*=\s*(?:Math\.max\([^;\n]*\b(\w+)\b|(\w+))\s*;?`)
	tsBoundUse     = regexp.MustCompile(`(?:Array\.from\(\s*\{\s*length\s*:\s*(\w+)|new\s+Array\(\s*(\w+)\s*\)|;\s*\w+\s*<=?\s*(\w+)\s*;|\.fill\([^)]*\)\s*\.\s*map|Array\(\s*(\w+)\s*\))`)
	tsPageEmit     = regexp.MustCompile(`\.(addPage|copyPages|insertPage|embedPage|addPages)\(`)
	tsLoopOpen     = regexp.MustCompile(`\bfor\s*\(|\.forEach\(|\.map\(|\bwhile\s*\(|\.reduce\(`)
	tsRequestIface = regexp.MustCompile(`(?:interface|type)\s+(\w+(?:Request|Req|Input|Body|Payload|Params|Submission))\b`)
	tsFieldDecl    = regexp.MustCompile(`^\s*(?:readonly\s+)?(\w+)\??\s*:`)
	tsMergeTrue    = regexp.MustCompile(`merge\s*:\s*true`)
	tsDocWrite     = regexp.MustCompile(`\b(setDoc|updateDoc|addDoc|set|update|add|create)\(`)
	tsObjKey       = regexp.MustCompile(`(?m)^\s*['"]?(\w+)['"]?\s*:`)
	tsDigitStrip   = regexp.MustCompile(`\.replace\(\s*/(?:\\D|\[\^0-9\]|\[\^\\d\])/g?\s*,\s*['"]{2}\s*\)`)
	tsDepthCompare = regexp.MustCompile(`\b(\w*(?:[dD]epth|[lL]evel|[nN]esting)\w*)\s*(?:>=|>)\s*(\d+)\b|\b(MAX_DEPTH|maxDepth|MaxDepth|MAX_NESTING|maxNesting)\w*\s*=\s*(\d+)\b`)
	tsExportFn     = regexp.MustCompile(`(?m)^export\s+(?:default\s+)?(?:async\s+)?function\s+(\w+)|^export\s+const\s+(\w+)\s*=\s*(?:async\s*)?\(`)
)

func lintTS(rel string, lines []string) []Finding {
	var out []Finding
	add := func(line int, rule, extra string) {
		msg := ruleText[rule]
		if extra != "" {
			msg = extra + " — " + msg
		}
		out = append(out, Finding{File: rel, Line: line, Rule: rule, Message: msg})
	}
	text := strings.Join(lines, "\n")
	lineOf := func(off int) int { return strings.Count(text[:off], "\n") + 1 }

	// A: numeric bounds derived from object keys.
	tainted := map[string]bool{}
	keyVars := map[string]bool{}
	for _, m := range tsForIn.FindAllStringSubmatch(text, -1) {
		keyVars[m[1]] = true
	}
	for _, m := range tsKeysCallback.FindAllStringSubmatch(text, -1) {
		keyVars[m[1]] = true
	}
	for _, m := range tsKeysNumber.FindAllStringSubmatch(text, -1) {
		tainted[m[1]] = true
	}
	for _, m := range tsNumberOf.FindAllStringSubmatch(text, -1) {
		if keyVars[m[2]] {
			tainted[m[1]] = true
		}
	}
	for pass := 0; pass < 3; pass++ {
		for _, m := range tsAssignFrom.FindAllStringSubmatch(text, -1) {
			if tainted[m[2]] || tainted[m[3]] {
				tainted[m[1]] = true
			}
		}
	}
	if len(tainted) > 0 {
		for _, loc := range tsBoundUse.FindAllStringSubmatchIndex(text, -1) {
			for g := 1; g <= 4; g++ {
				if loc[2*g] < 0 {
					continue
				}
				if name := text[loc[2*g]:loc[2*g+1]]; tainted[name] {
					add(lineOf(loc[0]), "A", "bound "+name+" comes from object keys")
				}
			}
		}
	}

	// B: page emitters inside loop bodies (brace-depth tracking).
	loopStack := []int{} // brace depth at which each open loop started
	depth := 0
	for i, l := range lines {
		if tsLoopOpen.MatchString(l) {
			loopStack = append(loopStack, depth)
		}
		if len(loopStack) > 0 && tsPageEmit.MatchString(l) {
			add(i+1, "B", tsPageEmit.FindStringSubmatch(l)[1]+" inside a loop")
		}
		depth += strings.Count(l, "{") - strings.Count(l, "}")
		for len(loopStack) > 0 && depth <= loopStack[len(loopStack)-1] && strings.Contains(l, "}") {
			loopStack = loopStack[:len(loopStack)-1]
		}
	}

	// C: provenance fields on request-shaped types.
	for _, loc := range tsRequestIface.FindAllStringSubmatchIndex(text, -1) {
		name := text[loc[2]:loc[3]]
		end := strings.Index(text[loc[1]:], "}")
		if end < 0 {
			continue
		}
		body := text[loc[1] : loc[1]+end]
		start := lineOf(loc[1])
		for j, bl := range strings.Split(body, "\n") {
			if m := tsFieldDecl.FindStringSubmatch(bl); m != nil && provenanceField.MatchString(m[1]) {
				add(start+j, "C", name+"."+m[1])
			}
		}
	}

	// D: declared gates per exported function.
	if gates := gatesDeclared(lines); len(gates) > 0 {
		locs := tsExportFn.FindAllStringSubmatchIndex(text, -1)
		for k, loc := range locs {
			name := ""
			for g := 1; g <= 2; g++ {
				if loc[2*g] >= 0 {
					name = text[loc[2*g]:loc[2*g+1]]
				}
			}
			end := len(text)
			if k+1 < len(locs) {
				end = locs[k+1][0]
			}
			body := text[loc[0]:end]
			isGate := false
			var missing []string
			for _, g := range gates {
				if g == name {
					isGate = true
				}
				if !strings.Contains(body, g+"(") {
					missing = append(missing, g)
				}
			}
			if !isGate && len(missing) > 0 {
				add(lineOf(loc[0]), "D", name+" skips "+strings.Join(missing, ", "))
			}
		}
	}

	// E/F: document writes.
	for _, loc := range tsDocWrite.FindAllStringSubmatchIndex(text, -1) {
		region := text[loc[0]:min(len(text), loc[0]+800)]
		if end := closingParen(region); end > 0 {
			region = region[:end]
		}
		line := lineOf(loc[0])
		if tsMergeTrue.MatchString(region) && (strings.Contains(region, "...") || regexp.MustCompile(`\(\s*\w+\s*,\s*\w+\s*,\s*\{\s*merge`).MatchString(region)) {
			add(line, "E", "merge: true on a spread/variable document")
		}
		for _, m := range tsObjKey.FindAllStringSubmatch(region, -1) {
			if sensitiveKey.MatchString(m[1]) {
				add(line, "F", "field "+strconv.Quote(m[1]))
			}
		}
	}

	// G: digit-stripping on money-named values.
	for i, l := range lines {
		if tsDigitStrip.MatchString(l) {
			ctx := l
			if i > 0 {
				ctx = lines[i-1] + " " + l
			}
			if moneyName.MatchString(ctx) {
				add(i+1, "G", "non-digits stripped from a money value ('-500' becomes '500')")
			}
		}
	}

	// H: depth caps.
	for i, l := range lines {
		for _, m := range tsDepthCompare.FindAllStringSubmatch(l, -1) {
			lit := m[2]
			if lit == "" {
				lit = m[4]
			}
			if n, err := strconv.Atoi(lit); err == nil && n > 0 && n < depthCap {
				add(i+1, "H", fmt.Sprintf("depth capped at %d", n))
			}
		}
	}
	return out
}

func closingParen(s string) int {
	depth := 0
	for i, r := range s {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// ── shell / CI yaml (rule H) ────────────────────────────────────────────────

var (
	shPipedVerifier = regexp.MustCompile(`\b(verify|check|test|lint|gate|audit)[^|\n]*\|\s*(tail|head|grep|wc|cat)\b`)
	shNoVerify      = regexp.MustCompile(`git\s+push[^\n]*--no-verify`)
)

func lintShell(rel string, lines []string) []Finding {
	var out []Finding
	pipefail := false
	for i, l := range lines {
		if strings.Contains(l, "pipefail") {
			pipefail = true
		}
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "#") {
			continue
		}
		if !pipefail && shPipedVerifier.MatchString(l) {
			out = append(out, Finding{File: rel, Line: i + 1, Rule: "H", Message: "verifier piped through a filter without pipefail; the pipe's exit status is the filter's — gate on the verifier's own status"})
		}
		if shNoVerify.MatchString(l) {
			out = append(out, Finding{File: rel, Line: i + 1, Rule: "H", Message: "git push --no-verify in a script bypasses the gate — " + ruleText["H"]})
		}
	}
	return out
}
