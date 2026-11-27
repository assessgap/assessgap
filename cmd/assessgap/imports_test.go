package main

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const module = "github.com/assessgap/assessgap/"

// allowedImports is the dependency rule from docs/architecture.md: the only
// packages of this module each package's production code (not its tests) may
// import. Leaves (bor, comps, search, socrata) import none, presentation never
// reaches the database build, and nothing but cli imports web. A new edge or a
// new package is a design decision: update docs/architecture.md with it.
var allowedImports = map[string][]string{
	"cmd/assessgap": {"internal/cli"},

	// Leaves: rules and facts with no internal imports.
	"internal/bor":     nil,
	"internal/comps":   nil,
	"internal/search":  nil,
	"internal/socrata": nil,

	// Building the database (the write side).
	"internal/store":   {"internal/socrata"},
	"internal/ingest":  {"internal/socrata", "internal/store"},
	"internal/refresh": {"internal/ingest"},
	"internal/live":    {"internal/comps", "internal/ingest", "internal/socrata", "internal/store"},

	// Presentation (the read side).
	"internal/packet":    {"internal/bor", "internal/comps"},
	"internal/web/views": {"internal/bor"},
	"internal/web":       {"internal/bor", "internal/comps", "internal/packet", "internal/search", "internal/web/views"},

	// The command wires everything together.
	"internal/cli": {"internal/bor", "internal/comps", "internal/ingest", "internal/live", "internal/packet",
		"internal/refresh", "internal/search", "internal/store", "internal/web"},

	// Test support, imported only from _test.go files. internal/e2e (PR #7,
	// open on its own branch) holds only tests and a doc.go; it's listed ahead
	// so either PR can merge first.
	"internal/fixture": {"internal/ingest", "internal/store"},
	"internal/golden":  nil,
	"internal/e2e":     nil,

	// Developer tools.
	"tools/fixture": {"internal/ingest", "internal/store"},
	"tools/ogimage": nil,
	"tools/prcheck": nil,
	"tools/prloc":   nil,
	"tools/tune":    {"internal/comps", "internal/store"},
}

// TestImports fails when a package imports one of this module's packages that
// the rule above doesn't allow, or when a package isn't in the rule at all.
// (An entry for a package that doesn't exist yet is allowed, so a branch that
// adds a package can be listed before it merges.)
func TestImports(t *testing.T) {
	for pkg, imports := range moduleImports(t, filepath.Join("..", "..")) {
		allowed, known := allowedImports[pkg]
		if !known {
			t.Errorf("%s is a new package: add it and its allowed imports to allowedImports and docs/architecture.md", pkg)
			continue
		}
		for _, imp := range imports {
			if !slices.Contains(allowed, imp) {
				t.Errorf("%s imports %s, which the dependency rule doesn't allow (docs/architecture.md)", pkg, imp)
			}
		}
	}
}

// moduleImports maps each package directory under root (as a slash path) to
// the module packages its non-test files import, sorted. Git-ignored scratch
// directories (data*, bin) are skipped.
func moduleImports(t *testing.T, root string) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if path != root && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "data") ||
				name == "bin" || name == "dist" || name == "testdata" || name == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		pkg := filepath.ToSlash(rel)
		if _, ok := out[pkg]; !ok {
			out[pkg] = nil
		}
		for _, spec := range f.Imports {
			p, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			if imp, ok := strings.CutPrefix(p, module); ok && !slices.Contains(out[pkg], imp) {
				out[pkg] = append(out[pkg], imp)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, imps := range out {
		sort.Strings(imps)
	}
	return out
}
