package router

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"strings"
	"testing"
)

// Bulk thread upserts intentionally treat lost fences as silent no-ops. Router
// lifecycle mutations must instead use the per-target CAS and observe its result.
// Match selectors, including method values, so assigning a bulk method before
// calling it cannot bypass this guard. This is syntactic, not receiver typing.
func fenceSilentThreadSelectors(file *ast.File) []*ast.SelectorExpr {
	var found []*ast.SelectorExpr
	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if ok && (selector.Sel.Name == "UpsertThreads" || selector.Sel.Name == "UpsertThreadsScoped") {
			found = append(found, selector)
		}
		return true
	})
	return found
}

func TestRouterProductionRejectsFenceSilentThreadUpserts(t *testing.T) {
	fset := token.NewFileSet()
	// Go tests execute in their package directory. ParseDir includes files for
	// every platform regardless of build tags; fixture/test seeding is excluded.
	packages, err := parser.ParseDir(fset, ".", func(info fs.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, parser.AllErrors)
	if err != nil {
		t.Fatal(err)
	}
	productionFiles := 0
	for _, pkg := range packages {
		for _, file := range pkg.Files {
			productionFiles++
			for _, selector := range fenceSilentThreadSelectors(file) {
				t.Errorf("%s: router production must not use fence-silent .%s; use the lifecycle CAS and check its result", fset.Position(selector.Pos()), selector.Sel.Name)
			}
		}
	}
	if productionFiles == 0 {
		t.Fatal("guard inspected no production Go files")
	}
}

func TestFenceSilentThreadGuardNegativeFixtures(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{"bulk-call", "store.UpsertThreads(records)", "UpsertThreads"},
		{"scoped-call", "store.UpsertThreadsScoped(records, scope)", "UpsertThreadsScoped"},
		{"parenthesized-call", "(store.UpsertThreads)(records)", "UpsertThreads"},
		{"method-value", "save := store.UpsertThreadsScoped; save(records, scope)", "UpsertThreadsScoped"},
		{"target-cas", "store.UpsertThreadCAS(record)", ""},
		{"comment-and-string", "/* store.UpsertThreads(records) */ _ = `store.UpsertThreadsScoped(records, scope)`", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", "package fixture\nfunc mutate() { "+tc.body+" }", parser.AllErrors)
			if err != nil {
				t.Fatal(err)
			}
			found := fenceSilentThreadSelectors(file)
			if tc.want == "" {
				if len(found) != 0 {
					t.Fatalf("safe fixture rejected: %v", found)
				}
				return
			}
			if len(found) != 1 || found[0].Sel.Name != tc.want {
				t.Fatalf("regression fixture escaped guard: found %v, want .%s", found, tc.want)
			}
		})
	}
}
