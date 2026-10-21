package conventions_test

import (
	"bufio"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
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
		if skippedDirectories[entry.Name()] {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return errors.New(path + ": symbolic links are not allowed in the backend tree")
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

var (
	gooseMarkers    = map[string]bool{"-- +goose Up": true, "-- +goose Down": true}
	queryAnnotation = regexp.MustCompile(`^-- name: [A-Z][A-Za-z0-9]* :(one|many|exec|execrows)$`)
)

type sqlFileKind int

const (
	sqlMigration sqlFileKind = iota
	sqlQuery
	sqlOther
)

func sqlKind(root string, path string) sqlFileKind {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return sqlOther
	}
	switch filepath.ToSlash(filepath.Dir(relative)) {
	case "migrations":
		return sqlMigration
	case "queries":
		return sqlQuery
	}
	return sqlOther
}

func sqlLineIsComment(kind sqlFileKind, line string) bool {
	text := strings.TrimRight(line, "\r")
	if strings.Contains(text, "/*") {
		return true
	}
	if !strings.Contains(text, "--") {
		return false
	}
	switch kind {
	case sqlMigration:
		return !gooseMarkers[text]
	case sqlQuery:
		return !queryAnnotation.MatchString(text)
	case sqlOther:
		return true
	}
	return true
}

func TestSQLHasNoCommentsBeyondFormatMarkers(t *testing.T) {
	files := sourceFiles(t, ".sql")

	var problems []string
	for _, path := range files {
		handle, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		kind := sqlKind(backendRoot, path)
		scanner := bufio.NewScanner(handle)
		line := 0
		for scanner.Scan() {
			line++
			if sqlLineIsComment(kind, scanner.Text()) {
				problems = append(problems, path+":"+strconv.Itoa(line)+": comment")
			}
		}
		if err := scanner.Err(); err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if err := handle.Close(); err != nil {
			t.Fatal(err)
		}
	}

	if len(problems) > 0 {
		t.Fatalf("SQL comments:\n%s", strings.Join(problems, "\n"))
	}
}

func TestSQLCommentDetection(t *testing.T) {
	cases := []struct {
		kind    sqlFileKind
		line    string
		comment bool
	}{
		{sqlMigration, "-- +goose Up", false},
		{sqlMigration, "-- +goose Down", false},
		{sqlMigration, "-- +goose StatementBegin", true},
		{sqlMigration, "-- name: ListUsers :many", true},
		{sqlMigration, "CREATE TABLE users (id uuid); -- explain", true},
		{sqlQuery, "-- name: ListUsers :many", false},
		{sqlQuery, "-- name: GetUser :one", false},
		{sqlQuery, "-- name: TouchUser :exec", false},
		{sqlQuery, "-- name: DeleteUsers :execrows", false},
		{sqlQuery, "-- name: ListUsers :many explain", true},
		{sqlQuery, "-- name: ListUsers :copyfrom", true},
		{sqlQuery, "-- name: listUsers :many", true},
		{sqlQuery, "-- +goose Up", true},
		{sqlQuery, "-- explain", true},
		{sqlQuery, "SELECT 1; /* explain */", true},
		{sqlOther, "-- name: ListUsers :many", true},
		{sqlOther, "SELECT 1", false},
		{sqlQuery, "  -- name: ListUsers :many", true},
		{sqlQuery, "-- name: ListUsers :many ", true},
		{sqlMigration, " -- +goose Up", true},
	}

	for _, testCase := range cases {
		found := sqlLineIsComment(testCase.kind, testCase.line)

		if found != testCase.comment {
			t.Errorf("sqlLineIsComment(%d, %q) = %v, want %v", testCase.kind, testCase.line, found, testCase.comment)
		}
	}
}

const repositoryRoot = "../../.."

var skippedRepositoryDirectoryNames = map[string]bool{".git": true, "node_modules": true}

var skippedRepositoryPaths = map[string]bool{
	"bin": true, "backend/bin": true, "backend/tmp": true,
	"frontend/dist": true, "frontend/test-results": true, "frontend/playwright-report": true,
}

var generatedRepositoryFiles = map[string]bool{"pnpm-lock.yaml": true}

var hashCommentExtensions = map[string]bool{".sh": true, ".bash": true, ".yml": true, ".yaml": true, ".toml": true, ".py": true}

var hashCommentNames = map[string]bool{
	".gitignore": true, ".prettierignore": true, ".markdownlintignore": true, ".editorconfig": true, ".env.example": true,
}

func hashCommentFiles(t *testing.T) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(repositoryRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(repositoryRoot, path)
		if err != nil {
			return err
		}
		if entry.IsDir() && (skippedRepositoryDirectoryNames[entry.Name()] || skippedRepositoryPaths[relative]) {
			return filepath.SkipDir
		}
		if entry.IsDir() || generatedRepositoryFiles[entry.Name()] {
			return nil
		}
		if hashCommentNames[entry.Name()] || hashCommentExtensions[filepath.Ext(path)] {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no shell, YAML, TOML, Python or ignore files found")
	}
	return files
}

func opensQuote(previous rune) bool {
	return strings.ContainsRune(" \t:=([{,-", previous)
}

func lineHasHashComment(line string) bool {
	var quote rune
	escaped := false
	previous := ' '
	for _, character := range line {
		switch {
		case escaped:
			escaped = false
		case quote == '"' && character == '\\':
			escaped = true
		case quote != 0:
			if character == quote {
				quote = 0
			}
		case (character == '"' || character == '\'') && opensQuote(previous):
			quote = character
		case character == '#' && (previous == ' ' || previous == '\t'):
			return true
		}
		previous = character
	}
	return false
}

func TestHashCommentedFilesHaveNoComments(t *testing.T) {
	files := hashCommentFiles(t)

	var problems []string
	for _, path := range files {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for index, line := range strings.Split(string(content), "\n") {
			if index == 0 && strings.HasPrefix(line, "#!") {
				continue
			}
			if lineHasHashComment(line) {
				problems = append(problems, path+":"+strconv.Itoa(index+1)+": comment")
			}
		}
	}

	if len(problems) > 0 {
		t.Fatalf("hash comments:\n%s", strings.Join(problems, "\n"))
	}
}

func TestHashCommentDetection(t *testing.T) {
	cases := map[string]bool{
		"# heading":                    true,
		"  # indented":                 true,
		"key: value # trailing":        true,
		"key: 'value # quoted'":        false,
		`echo "count is $#"`:           false,
		"url: http://host/path#anchor": false,
		"${#array[@]}":                 false,
		"key: don't # explain":         true,
		`key: "a \" # quoted"`:         false,
		`key: "a" # after`:             true,
	}

	for line, expected := range cases {
		found := lineHasHashComment(line)

		if found != expected {
			t.Errorf("lineHasHashComment(%q) = %v, want %v", line, found, expected)
		}
	}
}

func TestSQLFileKindFollowsTheExactDirectory(t *testing.T) {
	cases := map[string]sqlFileKind{
		"../../queries/users.sql":            sqlQuery,
		"../../migrations/00001_test.sql":    sqlMigration,
		"../../queries/nested/users.sql":     sqlOther,
		"../../internal/queries/users.sql":   sqlOther,
		"../../queries/migrations/up.sql":    sqlOther,
		"../../migrations/queries/users.sql": sqlOther,
		"../../internal/migrations/up.sql":   sqlOther,
		"../../users.sql":                    sqlOther,
	}

	for path, expected := range cases {
		kind := sqlKind("../..", path)

		if kind != expected {
			t.Errorf("sqlKind(%q) = %d, want %d", path, kind, expected)
		}
	}
}
