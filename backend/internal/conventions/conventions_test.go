package conventions_test

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const backendRoot = "../.."

var skippedDirectories = map[string]bool{"bin": true, "dbgen": true, "tmp": true}

func sourceFiles(t *testing.T, extension string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(backendRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && skippedDirectories[entry.Name()] {
			return filepath.SkipDir
		}
		if !entry.IsDir() && strings.HasSuffix(path, extension) {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatalf("no %s files found", extension)
	}
	return files
}

func TestGoSourceHasNoCommentsOrUntypedValues(t *testing.T) {
	files := sourceFiles(t, ".go")

	var problems []string
	for _, path := range files {
		problems = append(problems, goProblems(t, path)...)
	}

	if len(problems) > 0 {
		t.Fatalf("convention violations:\n%s", strings.Join(problems, "\n"))
	}
}

func goProblems(t *testing.T, path string) []string {
	t.Helper()
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	var problems []string
	for _, group := range file.Comments {
		for _, comment := range group.List {
			if strings.HasPrefix(comment.Text, "//go:embed ") || strings.HasPrefix(comment.Text, "//go:build ") {
				continue
			}
			problems = append(problems, fileSet.Position(comment.Pos()).String()+": comment")
		}
	}
	allowed := constraintNodes(file)
	ast.Inspect(file, func(node ast.Node) bool {
		if node == nil {
			return false
		}
		if allowed[node] {
			return false
		}
		switch typed := node.(type) {
		case *ast.Ident:
			if typed.Name == "any" && typed.Obj == nil {
				problems = append(problems, fileSet.Position(typed.Pos()).String()+": any")
			}
		case *ast.InterfaceType:
			if typed.Methods == nil || len(typed.Methods.List) == 0 {
				problems = append(problems, fileSet.Position(typed.Pos()).String()+": empty interface")
			}
		}
		return true
	})
	return problems
}

func constraintNodes(file *ast.File) map[ast.Node]bool {
	allowed := map[ast.Node]bool{}
	mark := func(fields *ast.FieldList) {
		if fields == nil {
			return
		}
		for _, field := range fields.List {
			allowed[field.Type] = true
		}
	}
	ast.Inspect(file, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.FuncType:
			mark(typed.TypeParams)
		case *ast.TypeSpec:
			mark(typed.TypeParams)
		}
		return true
	})
	return allowed
}

func TestSQLHasNoCommentsBeyondGooseMarkers(t *testing.T) {
	files := sourceFiles(t, ".sql")
	markers := map[string]bool{"-- +goose Up": true, "-- +goose Down": true, "-- +goose StatementBegin": true, "-- +goose StatementEnd": true}

	var problems []string
	for _, path := range files {
		handle, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		scanner := bufio.NewScanner(handle)
		line := 0
		for scanner.Scan() {
			line++
			text := strings.TrimSpace(scanner.Text())
			if (strings.Contains(text, "--") && !markers[text]) || strings.Contains(text, "/*") {
				problems = append(problems, path+":"+strconv.Itoa(line)+": comment")
			}
		}
		if err := handle.Close(); err != nil {
			t.Fatal(err)
		}
	}

	if len(problems) > 0 {
		t.Fatalf("SQL comments:\n%s", strings.Join(problems, "\n"))
	}
}
