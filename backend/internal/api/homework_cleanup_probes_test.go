package api_test

import (
	"strings"
	"testing"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/homework"
	"ovc/backend/internal/testdb"
)

func TestHomeworkCleanupIdleUsesTwoMarkedRowProbes(t *testing.T) {
	t.Parallel()
	f := newErasureFixture(t)
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
	now := h.now(t)
	pool, trace := testdb.NewHomeworkProbePool(t, h.database)
	cleanup := homework.Cleanup{Store: repositories.NewStore(pool), Random: h.random, Logger: h.logger}

	counts, runErr := cleanup.Run(t.Context(), now)
	probes := trace.Take()

	if runErr != nil || counts != (domain.HomeworkCleanupCounts{}) || len(probes) != 2 {
		t.Fatalf("idle cleanup: counts=%+v error=%v queries=%d", counts, runErr, len(probes))
	}
	var begins, commits, autocommitStatements int
	for _, probe := range probes {
		switch strings.ToUpper(strings.TrimSpace(probe.SQL())) {
		case "BEGIN":
			begins++
		case "COMMIT":
			commits++
		default:
			autocommitStatements++
		}
	}
	if begins != 0 || commits != 0 || autocommitStatements != 2 {
		t.Fatalf("idle transaction statements: begin=%d commit=%d autocommit=%d", begins, commits, autocommitStatements)
	}
	for index, probe := range probes {
		expected := []string{"issues_erase_pending_idx", "submissions_release_pending_idx"}[index]
		if !strings.HasPrefix(probe.SQL(), "-- name: List") || !strings.Contains(probe.SQL(), "pending_since IS NOT NULL") {
			t.Fatalf("unexpected idle query: %s", probe.SQL())
		}
		testdb.AssertHomeworkProbeIndex(t, pool, probe, expected)
	}
}
