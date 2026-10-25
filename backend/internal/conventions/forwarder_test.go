package conventions_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const (
	forwarderPackage   = modulePath + "/internal/testdb/forwarder"
	forwarderDirectory = "internal/testdb/forwarder"
	listenerLiveness   = "internal/adapters/listener/liveness_test.go"
)

func forwarderImportAllowed(file string) bool {
	return file == listenerLiveness || filepath.ToSlash(filepath.Dir(file)) == forwarderDirectory
}

func forwarderImporters(imports map[string][]string) []string {
	var refused []string
	for file, imported := range imports {
		if slices.Contains(imported, forwarderPackage) && !forwarderImportAllowed(file) {
			refused = append(refused, file)
		}
	}
	sort.Strings(refused)
	return refused
}

func importsPerFile(t *testing.T) map[string][]string {
	t.Helper()
	imports := map[string][]string{}
	for _, path := range sourceFiles(t, ".go") {
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		relative, err := filepath.Rel(backendRoot, path)
		if err != nil {
			t.Fatal(err)
		}
		relative = filepath.ToSlash(relative)
		imports[relative] = []string{}
		for _, spec := range file.Imports {
			imported, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			imports[relative] = append(imports[relative], imported)
		}
	}
	return imports
}

func TestOnlyTheListenerTestsUseTheForwarder(t *testing.T) {
	imports := importsPerFile(t)

	refused := forwarderImporters(imports)

	if len(refused) > 0 {
		t.Fatalf("only the listener adapter's tests may use the PostgreSQL forwarder: %s", strings.Join(refused, ", "))
	}
	if !slices.Contains(imports[listenerLiveness], forwarderPackage) {
		t.Fatal("the listener liveness tests no longer import the forwarder, so the guard checks nothing")
	}
}

func TestTheForwarderGuardRefusesOtherImporters(t *testing.T) {
	imports := map[string][]string{
		"internal/adapters/listener/listener.go":             {forwarderPackage},
		"internal/adapters/listener/liveness_test.go":        {forwarderPackage},
		"internal/adapters/listener/signals_test.go":         {forwarderPackage},
		"internal/api/auth_test.go":                          {forwarderPackage},
		"internal/adapters/mail/sink_test.go":                {forwarderPackage},
		"internal/testdb/forwarder/forwarder_test.go":        {forwarderPackage},
		"internal/adapters/listener/nested/liveness_test.go": {forwarderPackage},
		"internal/api/server.go":                             {modulePath + "/internal/testdb"},
	}

	refused := forwarderImporters(imports)

	want := []string{
		"internal/adapters/listener/listener.go",
		"internal/adapters/listener/nested/liveness_test.go",
		"internal/adapters/listener/signals_test.go",
		"internal/adapters/mail/sink_test.go",
		"internal/api/auth_test.go",
	}
	if !slices.Equal(refused, want) {
		t.Fatalf("the guard refused %v, want %v", refused, want)
	}
}

func exportedSurface(t *testing.T, directory string) []string {
	t.Helper()
	var surface []string
	for _, path := range sourceFiles(t, ".go") {
		relative, err := filepath.Rel(backendRoot, path)
		if err != nil {
			t.Fatal(err)
		}
		if filepath.ToSlash(filepath.Dir(relative)) != directory || strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range file.Decls {
			surface = append(surface, exportedNames(declaration)...)
		}
	}
	sort.Strings(surface)
	return surface
}

func exportedNames(declaration ast.Decl) []string {
	var names []string
	switch typed := declaration.(type) {
	case *ast.FuncDecl:
		if !typed.Name.IsExported() {
			return nil
		}
		if typed.Recv == nil {
			return []string{typed.Name.Name}
		}
		receiver := receiverName(typed.Recv.List[0].Type)
		if !ast.IsExported(receiver) {
			return nil
		}
		return []string{receiver + "." + typed.Name.Name}
	case *ast.GenDecl:
		for _, spec := range typed.Specs {
			names = append(names, exportedSpecNames(spec)...)
		}
	}
	return names
}

func exportedSpecNames(spec ast.Spec) []string {
	var names []string
	switch typed := spec.(type) {
	case *ast.TypeSpec:
		if typed.Name.IsExported() {
			names = append(names, typed.Name.Name)
		}
		if structure, ok := typed.Type.(*ast.StructType); ok {
			for _, field := range structure.Fields.List {
				for _, name := range field.Names {
					if name.IsExported() {
						names = append(names, typed.Name.Name+"."+name.Name)
					}
				}
			}
		}
	case *ast.ValueSpec:
		for _, name := range typed.Names {
			if name.IsExported() {
				names = append(names, name.Name)
			}
		}
	}
	return names
}

func receiverName(expression ast.Expr) string {
	if star, ok := expression.(*ast.StarExpr); ok {
		expression = star.X
	}
	if identifier, ok := expression.(*ast.Ident); ok {
		return identifier.Name
	}
	return "?"
}

func TestTheForwarderExposesOnlyItsAddressAndOneStop(t *testing.T) {
	surface := exportedSurface(t, forwarderDirectory)

	want := []string{"Forwarder", "Forwarder.Stop", "Forwarder.URL", "New"}
	if !slices.Equal(surface, want) {
		t.Fatalf("the forwarder exports %v, want %v", surface, want)
	}
}
