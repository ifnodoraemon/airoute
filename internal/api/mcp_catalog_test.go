package api

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// dispatchedMCPToolNames parses the MCP dispatcher sources for
// `case "airoute_…":` labels. The dispatch is a switch chain, so the source is the
// only place that knows the full set; a reflection-based test would have to call
// the tools and several of them hit an upstream.
func dispatchedMCPToolNames(t *testing.T) []string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate the test file")
	}
	dir := filepath.Dir(thisFile)

	sources, err := filepath.Glob(filepath.Join(dir, "mcp_tools*.go"))
	if err != nil {
		t.Fatalf("glob dispatcher sources: %v", err)
	}
	if len(sources) == 0 {
		t.Fatal("no MCP tool dispatcher sources found")
	}

	names := make(map[string]bool)
	fset := token.NewFileSet()
	for _, path := range sources {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		if _, statErr := os.Stat(path); statErr != nil {
			continue
		}
		parsed, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", filepath.Base(path), parseErr)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			switch stmt := node.(type) {
			case *ast.CaseClause:
				for _, expr := range stmt.List {
					lit, isLit := expr.(*ast.BasicLit)
					if !isLit || lit.Kind != token.STRING {
						continue
					}
					name := strings.Trim(lit.Value, `"`)
					if strings.HasPrefix(name, "airoute_") {
						names[name] = true
					}
				}
			}
			return true
		})
	}

	out := make([]string, 0, len(names))
	for name := range names {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// builtinMCPToolNames lists the tools the server advertises through tools/list.
func builtinMCPToolNames() []string {
	tools := getBuiltinMCPTools()
	out := make([]string, 0, len(tools))
	for _, entry := range tools {
		if name, ok := entry["name"].(string); ok {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// D-06: `tools/list` is the only way a client discovers a tool, so a dispatched
// built-in tool that is missing from the catalog is unusable in practice, and a
// catalog entry with no dispatcher is a promise the server cannot keep. Both
// directions are asserted here.
func TestBuiltinMCPToolsMatchDispatcher(t *testing.T) {
	dispatched := dispatchedMCPToolNames(t)
	if len(dispatched) == 0 {
		t.Fatal("parsed no airoute_ tool cases; the test would pass vacuously")
	}

	catalog := make(map[string]bool)
	for _, name := range builtinMCPToolNames() {
		if strings.HasPrefix(name, "airoute_") {
			catalog[name] = true
		}
	}

	for _, name := range dispatched {
		if !catalog[name] {
			t.Errorf("tool %q has a handler but is not in tools/list", name)
		}
	}
	for name := range catalog {
		found := false
		for _, dispatchedName := range dispatched {
			if dispatchedName == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("tool %q is advertised in tools/list but has no handler", name)
		}
	}
}

// The two utilities the audit found missing must stay discoverable.
func TestBuiltinMCPToolCatalogIncludesUtilityTools(t *testing.T) {
	catalog := make(map[string]bool)
	for _, name := range builtinMCPToolNames() {
		catalog[name] = true
	}
	for _, name := range []string{"airoute_get_current_time", "airoute_calc_eval"} {
		if !catalog[name] {
			t.Errorf("tool %q is missing from the builtin catalog", name)
		}
	}
}
