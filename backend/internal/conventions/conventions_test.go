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

var skippedDirectories = map[string]bool{"bin": true, "internal/dbgen": true, "tmp": true}

func sourceFiles(t *testing.T, extension string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(backendRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(backendRoot, path)
		if err != nil {
			return err
		}
		if entry.IsDir() && skippedDirectories[filepath.ToSlash(relative)] {
			return filepath.SkipDir
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
	gooseMarkers = map[string]bool{
		"-- +goose Up": true, "-- +goose Down": true, statementBeginMarker: true, statementEndMarker: true,
	}
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
		{sqlMigration, "-- +goose StatementBegin", false},
		{sqlMigration, "-- +goose StatementEnd", false},
		{sqlMigration, "-- +goose StatementBegin x", true},
		{sqlQuery, "-- +goose StatementBegin", true},
		{sqlOther, "-- +goose StatementEnd", true},
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

const (
	statementBeginMarker = "-- +goose StatementBegin"
	statementEndMarker   = "-- +goose StatementEnd"
	statementMarkerText  = "+goose statement"
)

var (
	functionStatementHead = regexp.MustCompile(`(?is)^CREATE\s+(OR\s+REPLACE\s+)?FUNCTION\s+[A-Za-z_][A-Za-z0-9_.]*\s*\(`)
	bodyOpening           = regexp.MustCompile(`(?is)^[^;'"$]*\sAS\s+(\$([A-Za-z_][A-Za-z0-9_]*)?\$)`)
)

func argumentListEnd(text string, opening int) int {
	depth := 0
	for index := opening; index < len(text); index++ {
		switch text[index] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return index + 1
			}
		case '\'', '"', '$', ';':
			return -1
		}
	}
	return -1
}

func isSingleFunctionStatement(statement string) bool {
	text := strings.TrimSpace(statement)
	head := functionStatementHead.FindStringIndex(text)
	if head == nil {
		return false
	}
	afterArguments := argumentListEnd(text, head[1]-1)
	if afterArguments < 0 {
		return false
	}
	opening := bodyOpening.FindStringSubmatchIndex(text[afterArguments:])
	if opening == nil {
		return false
	}
	tagStart, tagEnd := afterArguments+opening[2], afterArguments+opening[3]
	tag := text[tagStart:tagEnd]
	bodyLength := strings.Index(text[tagEnd:], tag)
	if bodyLength < 0 || !strings.Contains(text[tagEnd:tagEnd+bodyLength], ";") {
		return false
	}
	rest := text[tagEnd+bodyLength+len(tag):]
	return strings.Count(rest, ";") == 1 && strings.HasSuffix(rest, ";")
}

func lineProblem(line int, message string) string {
	return strconv.Itoa(line) + ": " + message
}

func statementMarkerProblems(kind sqlFileKind, content string) []string {
	var problems []string
	var statement []string
	open := false
	openedAt := 0
	for index, line := range strings.Split(content, "\n") {
		number := index + 1
		text := strings.TrimRight(line, "\r")
		isMigration := kind == sqlMigration
		switch {
		case isMigration && text == statementBeginMarker:
			if open {
				problems = append(problems, lineProblem(number, "StatementBegin inside an open statement"))
			}
			open, openedAt, statement = true, number, nil
		case isMigration && text == statementEndMarker:
			if !open {
				problems = append(problems, lineProblem(number, "StatementEnd without StatementBegin"))
				continue
			}
			if !isSingleFunctionStatement(strings.Join(statement, "\n")) {
				problems = append(problems, lineProblem(openedAt, "statement markers wrap something other than one function"))
			}
			open = false
		case strings.Contains(strings.ToLower(text), statementMarkerText):
			problems = append(problems, lineProblem(number, "statement marker with other text or outside migrations"))
		case open:
			statement = append(statement, text)
		}
	}
	if open {
		problems = append(problems, lineProblem(openedAt, "StatementBegin without StatementEnd"))
	}
	return problems
}

const wrappedFunction = `-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION signal_recheck() RETURNS trigger AS $$
BEGIN
    PERFORM pg_notify('ovc_realtime', NEW.user_id::text);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER sessions_revoked AFTER UPDATE ON sessions FOR EACH ROW EXECUTE FUNCTION signal_recheck();

-- +goose Down
DROP TRIGGER sessions_revoked ON sessions;
DROP FUNCTION signal_recheck();
`

func TestMigrationStatementMarkersOnlyWrapFunctionBodies(t *testing.T) {
	accepted := map[string]string{
		"a function body with a dollar tag": wrappedFunction,
		"a plain function with a named dollar tag": "-- +goose StatementBegin\n" +
			"CREATE FUNCTION one() RETURNS integer AS $body$ SELECT 1; $body$ LANGUAGE sql;\n" +
			"-- +goose StatementEnd\n",
		"no statement markers": "-- +goose Up\nCREATE TABLE users (id uuid);\n",
	}
	refused := map[string]string{
		"a marker with trailing text": strings.Replace(wrappedFunction, statementBeginMarker, statementBeginMarker+" ok", 1),
		"an indented marker":          strings.Replace(wrappedFunction, statementEndMarker, " "+statementEndMarker, 1),
		"an unpaired begin":           strings.Replace(wrappedFunction, statementEndMarker+"\n", "", 1),
		"an unpaired end":             strings.Replace(wrappedFunction, statementBeginMarker+"\n", "", 1),
		"a nested begin": strings.Replace(
			wrappedFunction, "BEGIN\n", statementBeginMarker+"\nBEGIN\n", 1,
		),
		"a table":     "-- +goose StatementBegin\nCREATE TABLE users (id uuid);\n-- +goose StatementEnd\n",
		"a procedure": "-- +goose StatementBegin\nCREATE PROCEDURE p() AS $$ SELECT 1; $$ LANGUAGE sql;\n-- +goose StatementEnd\n",
		"a do block":  "-- +goose StatementBegin\nDO $$ BEGIN PERFORM 1; END; $$;\n-- +goose StatementEnd\n",
		"a function and another statement": strings.Replace(
			wrappedFunction, "$$ LANGUAGE plpgsql;\n", "$$ LANGUAGE plpgsql;\nDROP TABLE users;\n", 1,
		),
		"an unquoted function body": "-- +goose StatementBegin\n" +
			"CREATE FUNCTION one() RETURNS integer LANGUAGE sql RETURN 1;\n-- +goose StatementEnd\n",
		"an empty wrapping": "-- +goose StatementBegin\n-- +goose StatementEnd\n",
		"a dollar tag inside the function name": "-- +goose StatementBegin\n" +
			"CREATE FUNCTION a$x$b() RETURNS void AS $$ BEGIN END; $$ LANGUAGE plpgsql; DROP TABLE users; $x$;\n" +
			"-- +goose StatementEnd\n",
		"a dollar quote inside a single-quoted literal": "-- +goose StatementBegin\n" +
			"CREATE FUNCTION one(x text DEFAULT '$$') RETURNS text AS 'a'; DROP TABLE users; SELECT '$$' LANGUAGE sql;\n" +
			"-- +goose StatementEnd\n",
		"a body without a semicolon": "-- +goose StatementBegin\n" +
			"CREATE FUNCTION one() RETURNS integer AS $$ SELECT 1 $$ LANGUAGE sql;\n-- +goose StatementEnd\n",
	}
	refusedOutsideMigrations := wrappedFunction

	for name, content := range accepted {
		t.Run("accepts "+name, func(t *testing.T) {
			problems := statementMarkerProblems(sqlMigration, content)

			if len(problems) > 0 {
				t.Errorf("unexpected problems: %v", problems)
			}
		})
	}
	for name, content := range refused {
		t.Run("refuses "+name, func(t *testing.T) {
			problems := statementMarkerProblems(sqlMigration, content)

			if len(problems) == 0 {
				t.Error("expected a problem, found none")
			}
		})
	}
	for name, kind := range map[string]sqlFileKind{"queries": sqlQuery, "other SQL files": sqlOther} {
		t.Run("refuses markers in "+name, func(t *testing.T) {
			problems := statementMarkerProblems(kind, refusedOutsideMigrations)

			if len(problems) != 2 {
				t.Errorf("expected two problems, found %v", problems)
			}
		})
	}
	t.Run("accepts the backend SQL files", func(t *testing.T) {
		files := sourceFiles(t, ".sql")

		var problems []string
		for _, path := range files {
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, problem := range statementMarkerProblems(sqlKind(backendRoot, path), string(content)) {
				problems = append(problems, path+":"+problem)
			}
		}

		if len(problems) > 0 {
			t.Fatalf("misplaced goose statement markers:\n%s", strings.Join(problems, "\n"))
		}
	})
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
