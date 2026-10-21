package conventions_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"ovc/backend/internal/domain"
)

const domainDirectory = "../domain"

func declaredErrorCodes(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(domainDirectory)
	if err != nil {
		t.Fatal(err)
	}
	var declared []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		declared = append(declared, errorCodeConstants(t, filepath.Join(domainDirectory, entry.Name()))...)
	}
	return declared
}

func errorCodeConstants(t *testing.T, path string) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	var values []string
	for _, declaration := range file.Decls {
		general, isGeneral := declaration.(*ast.GenDecl)
		if !isGeneral || general.Tok != token.CONST {
			continue
		}
		for _, spec := range general.Specs {
			valueSpec, isValueSpec := spec.(*ast.ValueSpec)
			if !isValueSpec {
				t.Fatalf("%s: a const declaration holds a %T", path, spec)
			}
			values = append(values, errorCodeValues(t, path, valueSpec)...)
		}
	}
	return values
}

func errorCodeValues(t *testing.T, path string, spec *ast.ValueSpec) []string {
	t.Helper()
	typeName, isIdent := spec.Type.(*ast.Ident)
	if !isIdent || typeName.Name != "ErrorCode" {
		refuseErrorCodeConversions(t, path, spec)
		return nil
	}
	values := make([]string, 0, len(spec.Values))
	for _, value := range spec.Values {
		literal, isLiteral := value.(*ast.BasicLit)
		if !isLiteral || literal.Kind != token.STRING {
			t.Fatalf("%s: ErrorCode %s is not a string literal", path, spec.Names[0].Name)
		}
		unquoted, err := strconv.Unquote(literal.Value)
		if err != nil {
			t.Fatal(err)
		}
		values = append(values, unquoted)
	}
	return values
}

func refuseErrorCodeConversions(t *testing.T, path string, spec *ast.ValueSpec) {
	t.Helper()
	for _, value := range spec.Values {
		call, isCall := value.(*ast.CallExpr)
		if !isCall {
			continue
		}
		if function, isIdent := call.Fun.(*ast.Ident); isIdent && function.Name == "ErrorCode" {
			t.Fatalf("%s: declare %s with the ErrorCode type, not a conversion", path, spec.Names[0].Name)
		}
	}
}

func TestErrorCodesEnumListsEveryConstant(t *testing.T) {
	declared := declaredErrorCodes(t)

	listed := make([]string, 0, len(declared))
	for _, code := range domain.AllErrorCodes() {
		listed = append(listed, string(code))
	}

	slices.Sort(declared)
	slices.Sort(listed)
	if !slices.Equal(declared, listed) {
		t.Fatalf("ErrorCode constants and AllErrorCodes differ:\nconstants: %v\nlisted:    %v", declared, listed)
	}
}
