package api_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

func (h *authHarness) seedSpentUncertainObligations(t *testing.T, lesson string, count int) {
	t.Helper()
	h.exec2(t, `WITH created AS (
    INSERT INTO meetings (id, lesson_id, provider, provider_meeting_id, routing_key, state, created_at, updated_at, ended_at, close_reason)
    SELECT gen_random_uuid(), $1::uuid, 'fake', gen_random_uuid(), gen_random_uuid(), 'closed', earlier, earlier, earlier, 'provider_error'
    FROM lessons CROSS JOIN LATERAL (SELECT lessons.created_at - interval '1 hour' AS earlier) AS seeded CROSS JOIN generate_series(1, $2::text::int)
    WHERE lessons.id = $1::uuid
    RETURNING id, provider_meeting_id, created_at
), ledgers AS (
    INSERT INTO meeting_recovery (provider_meeting_id, provider, meeting_id, origin, create_state, attempt_id, dispatched_at, cleanup_requested,
        cycle_number, cycle_outbox_id, cycle_result, reconcile_cycle_used, created_at, updated_at)
    SELECT provider_meeting_id, 'fake', id, 'attempt', 'uncertain', gen_random_uuid(), created_at, true, 2, gen_random_uuid(), 'unresolved', true, created_at, created_at
    FROM created
    RETURNING provider_meeting_id
)
UPDATE meeting_recovery_capacity SET unsettled = unsettled + (SELECT count(provider_meeting_id) FROM ledgers)`, lesson, fmt.Sprint(count))
}

func TestMeetingQuietSettlementReadmitsTutor(t *testing.T) {
	t.Parallel()
	h := newMeetingHarness(t)
	parties := h.visibilityParties(t)
	lesson := oneToOneLesson(t, h, parties)
	later := h.lessonAt(t, parties.tutorToken, parties.card, firstLessonDate, "18:50")
	h.seedSpentUncertainObligations(t, lesson, domain.MeetingUnsettledPerTutor-1)
	if err := h.provider.SimulateUnavailable(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	failed := h.joinCall(t, parties.tutorToken, lesson)
	if err := h.provider.SimulateUnavailable(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	h.tick(t)
	h.at(t, firstLessonStart.Add(50*time.Minute))
	refused := h.joinCall(t, accessToken(t, h.login(t, tutorEmail, accountPassword)), later)
	h.tick(t)
	atLimit := h.unsettled(t)

	h.at(t, firstLessonStart.Add(65*time.Minute))
	h.tick(t)
	admitted := h.joinCall(t, accessToken(t, h.login(t, tutorEmail, accountPassword)), later)

	expectRefusal(t, "the failed join", failed, http.StatusServiceUnavailable, "provider_unavailable")
	expectRefusal(t, "a tutor at the limit before the window", refused, http.StatusServiceUnavailable, "service_unavailable")
	if atLimit != domain.MeetingUnsettledPerTutor {
		t.Fatalf("%d unsettled at the limit", atLimit)
	}
	if admitted.status != http.StatusOK || h.unsettled(t) != domain.MeetingUnsettledPerTutor {
		t.Fatalf("after quiet settlement the join answered %d %s with %d unsettled", admitted.status, admitted.body, h.unsettled(t))
	}
}
