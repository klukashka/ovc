package api_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/homework"
	"ovc/backend/internal/testdb"
)

func (f erasureFixture) seedManyIssues(t *testing.T) {
	t.Helper()
	h := f.h
	if _, err := h.database.Pool.Exec(t.Context(), `INSERT INTO issues (id, tutor_id, template_id, relationship_id, title, snapshot, snapshot_bytes, max_points, has_manual_questions, answer_policy, status, version, issued_at, updated_at)
 SELECT gen_random_uuid(), tutor_id, template_id, relationship_id, title, snapshot, snapshot_bytes, max_points, has_manual_questions, answer_policy, status, version, issued_at, updated_at
 FROM issues CROSS JOIN generate_series(1,2000) WHERE id = $1::uuid`, f.issue); err != nil {
		t.Fatal(err)
	}
	if _, err := h.database.Pool.Exec(t.Context(), `INSERT INTO submissions (id, issue_id, tutor_id, relationship_id, student_id, status, issued_at, version, created_at, updated_at)
 SELECT gen_random_uuid(), id, tutor_id, relationship_id, $2::uuid, 'not_started', issued_at, 1, issued_at, updated_at
 FROM issues WHERE id <> $1::uuid`, f.issue, f.student.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := h.database.Pool.Exec(t.Context(), "ANALYZE issues; ANALYZE submissions"); err != nil {
		t.Fatal(err)
	}
}

func generatedQuery(t *testing.T, file string, name string) string {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), filepath.Join("..", "dbgen", file), nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	for _, declaration := range parsed.Decls {
		general, isGeneral := declaration.(*ast.GenDecl)
		if !isGeneral || general.Tok != token.CONST {
			continue
		}
		for _, spec := range general.Specs {
			value, isValue := spec.(*ast.ValueSpec)
			if !isValue || len(value.Names) != 1 || value.Names[0].Name != name || len(value.Values) != 1 {
				continue
			}
			literal, isLiteral := value.Values[0].(*ast.BasicLit)
			if !isLiteral {
				t.Fatalf("%s is not a string literal", name)
			}
			query, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Fatal(err)
			}
			return query
		}
	}
	t.Fatalf("%s is not declared in %s", name, file)
	return ""
}

func TestHomeworkCleanupIdleUsesTwoMarkedRowProbes(t *testing.T) {
	t.Parallel()
	f := newErasureFixture(t)
	h := f.h
	f.seedManyIssues(t)
	now := h.now(t)
	cleanup := homework.Cleanup{Store: repositories.NewStore(h.database.Pool), Random: h.random, Logger: h.logger}
	window := testdb.WatchStatements(t, h.database)

	counts, runErr := cleanup.Run(t.Context(), now)
	executed := window.Executed(t)

	if runErr != nil || counts != (domain.HomeworkCleanupCounts{}) || len(executed) != 2 {
		t.Fatalf("idle cleanup: counts=%+v error=%v statements=%+v", counts, runErr, executed)
	}
	probes := map[string]string{}
	for _, statement := range executed {
		upper := strings.ToUpper(statement.Query)
		if statement.Calls != 1 || strings.Contains(upper, "BEGIN") || strings.Contains(upper, "COMMIT") {
			t.Fatalf("the idle cleanup ran %+v", statement)
		}
		switch {
		case strings.Contains(statement.Query, "FROM issues") && strings.Contains(statement.Query, "erase_pending_since IS NOT NULL"):
			probes["issues_erase_pending_idx"] = statement.Query
		case strings.Contains(statement.Query, "FROM submissions") && strings.Contains(statement.Query, "release_pending_since IS NOT NULL"):
			probes["submissions_release_pending_idx"] = statement.Query
		default:
			t.Fatalf("unexpected idle statement: %s", statement.Query)
		}
	}
	if len(probes) != 2 {
		t.Fatalf("the idle cleanup did not run both marked-row probes: %v", probes)
	}
	for index, query := range probes {
		if plan := testdb.GenericPlan(t, h.database, query); !strings.Contains(plan, index) {
			t.Fatalf("the probe does not use %s:\n%s", index, plan)
		}
	}
}

func (f erasureFixture) spreadIssuesOverCards(t *testing.T) {
	t.Helper()
	h := f.h
	if _, err := h.database.Pool.Exec(t.Context(), `INSERT INTO relationships (id, tutor_id, state, display_name, email, notes, created_at, updated_at)
 SELECT gen_random_uuid(), $1, 'invited', 'Spread ' || n, 'spread-' || n || '@example.org', '', $2, $2 FROM generate_series(1, 2000) AS n`,
		f.tutor, h.now(t)); err != nil {
		t.Fatal(err)
	}
	if _, err := h.database.Pool.Exec(t.Context(), `WITH cards AS (SELECT id, row_number() OVER (ORDER BY id) AS n FROM relationships WHERE display_name LIKE 'Spread %'),
 work AS (SELECT id, row_number() OVER (ORDER BY id) AS n FROM issues WHERE id <> $1::uuid)
 UPDATE issues SET relationship_id = cards.id FROM work JOIN cards ON cards.n = work.n WHERE issues.id = work.id`, f.issue); err != nil {
		t.Fatal(err)
	}
	if _, err := h.database.Pool.Exec(t.Context(), `UPDATE submissions SET relationship_id = issues.relationship_id FROM issues
 WHERE submissions.issue_id = issues.id AND issues.id <> $1::uuid`, f.issue); err != nil {
		t.Fatal(err)
	}
	if _, err := h.database.Pool.Exec(t.Context(), "ANALYZE relationships; ANALYZE issues; ANALYZE submissions"); err != nil {
		t.Fatal(err)
	}
}

func TestLockLifecycleIssuesUsesIndexes(t *testing.T) {
	t.Parallel()
	f := newErasureFixture(t)
	f.seedManyIssues(t)
	f.spreadIssuesOverCards(t)
	query := generatedQuery(t, "homework_cleanup.sql.go", "lockLifecycleIssues")

	plan := testdb.GenericPlan(t, f.h.database, query)

	if strings.Contains(plan, "Seq Scan") || !strings.Contains(plan, "issues_relationship_idx") || !strings.Contains(plan, "submissions_relationship_idx") {
		t.Fatalf("the lifecycle issue lock does not use the relationship indexes:\n%s", plan)
	}
}
