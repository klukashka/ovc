package api_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

const (
	mirroredLinks = `SELECT relationships.id::text, relationships.state, COALESCE(relationships.discard_reason, ''), relationships.tutor_id::text,
    COALESCE(relationships.student_id::text, ''), relationships.ended_at, channels.id::text, channels.read_only_since,
    channels.discarded_at IS NOT NULL
FROM relationships LEFT JOIN channels ON channels.relationship_id = relationships.id`
	mirroredGroups = `SELECT groups.id::text, groups.tutor_id::text, groups.archived_at, channels.id::text, channels.read_only_since,
    ARRAY(
        SELECT relationships.student_id::text FROM group_members
        JOIN relationships ON relationships.id = group_members.relationship_id
        WHERE group_members.group_id = groups.id AND relationships.student_id IS NOT NULL
    )
FROM groups LEFT JOIN channels ON channels.group_id = groups.id`
	insertPendingChatUpload = `INSERT INTO attachments (id, uploader_id, context, scope_kind, scope_id, storage_key, filename, media_type, size_bytes,
    sha256, state, expires_at, created_at, updated_at)
VALUES ($1::uuid, $2::uuid, 'chat', 'channel', $3::uuid, 'chat/' || $1, 'notes.txt', 'text/plain', 10, sha256('x'), 'pending',
    now() + interval '1 day', now(), now())`
)

type mirroredLink struct {
	id, state, reason, tutor, student string
	endedAt                           *time.Time
	channel                           *string
	readOnlySince                     *time.Time
	discarded                         bool
}

func (h *authHarness) mirrorProblems(t *testing.T) []string {
	t.Helper()
	var problems []string
	rows, err := h.database.Pool.Query(context.Background(), mirroredLinks)
	if err != nil {
		t.Fatal(err)
	}
	var links []mirroredLink
	for rows.Next() {
		var link mirroredLink
		if err := rows.Scan(&link.id, &link.state, &link.reason, &link.tutor, &link.student, &link.endedAt, &link.channel,
			&link.readOnlySince, &link.discarded); err != nil {
			t.Fatal(err)
		}
		links = append(links, link)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for _, link := range links {
		problems = append(problems, h.directProblems(t, link)...)
	}
	problems = append(problems, h.groupProblems(t)...)
	return problems
}

func (h *authHarness) directProblems(t *testing.T, link mirroredLink) []string {
	t.Helper()
	state := domain.RelationshipState(link.state)
	switch state {
	case domain.RelationshipInvited:
		if link.channel != nil {
			return []string{"the invited card " + link.id + " has a channel"}
		}
		return nil
	case domain.RelationshipDiscarded:
		if link.channel == nil {
			return nil
		}
		members := h.channelMembers(t, *link.channel)
		if link.reason == string(domain.DiscardStudentPurged) {
			if link.discarded || len(members) != 1 || members[link.tutor].User == "" || link.readOnlySince == nil {
				return []string{fmt.Sprintf("the purged student's channel %s is discarded %t with members %v", *link.channel, link.discarded, members)}
			}
			return nil
		}
		if !link.discarded || len(members) != 0 {
			return []string{fmt.Sprintf("the discarded link's channel %s is discarded %t with members %v", *link.channel, link.discarded, members)}
		}
		return nil
	case domain.RelationshipProvisional, domain.RelationshipActive, domain.RelationshipEnded, domain.RelationshipReclaimPending:
	}
	if link.channel == nil || link.discarded {
		return []string{fmt.Sprintf("the %s relationship %s has no live channel", state, link.id)}
	}
	members := h.channelMembers(t, *link.channel)
	tutor, tutorListed := members[link.tutor]
	student, studentListed := members[link.student]
	var problems []string
	if len(members) != 2 || !tutorListed || !studentListed || tutor.Suspended || student.Suspended != (state == domain.RelationshipReclaimPending) {
		problems = append(problems, fmt.Sprintf("the %s relationship's channel holds %v", state, members))
	}
	wantReadOnly := state == domain.RelationshipEnded
	if (link.readOnlySince != nil) != wantReadOnly || (wantReadOnly && !link.readOnlySince.Equal(*link.endedAt)) {
		problems = append(problems, fmt.Sprintf("the %s relationship's channel is read-only since %v", state, link.readOnlySince))
	}
	return problems
}

func (h *authHarness) groupProblems(t *testing.T) []string {
	t.Helper()
	rows, err := h.database.Pool.Query(context.Background(), mirroredGroups)
	if err != nil {
		t.Fatal(err)
	}
	type mirroredGroup struct {
		id, tutor     string
		archivedAt    *time.Time
		channel       *string
		readOnlySince *time.Time
		students      []string
	}
	var groups []mirroredGroup
	for rows.Next() {
		var group mirroredGroup
		if err := rows.Scan(&group.id, &group.tutor, &group.archivedAt, &group.channel, &group.readOnlySince, &group.students); err != nil {
			t.Fatal(err)
		}
		groups = append(groups, group)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	var problems []string
	for _, group := range groups {
		if group.channel == nil {
			problems = append(problems, "the group "+group.id+" has no channel")
			continue
		}
		members := h.channelMembers(t, *group.channel)
		want := append([]string{group.tutor}, group.students...)
		slices.Sort(want)
		got := activeMemberSet(members)
		if !slices.Equal(got, want) || len(members) != len(got) {
			problems = append(problems, fmt.Sprintf("the group %s channel holds %v instead of %v", group.id, members, want))
		}
		sameSince := (group.archivedAt == nil) == (group.readOnlySince == nil) &&
			(group.archivedAt == nil || group.archivedAt.Equal(*group.readOnlySince))
		if !sameSince {
			problems = append(problems, fmt.Sprintf("the group %s is archived at %v and its channel read-only since %v", group.id, group.archivedAt, group.readOnlySince))
		}
	}
	return problems
}

func (h *authHarness) assertMirrored(t *testing.T) {
	t.Helper()
	if problems := h.mirrorProblems(t); len(problems) > 0 {
		t.Fatalf("channel membership drifted from relationships and groups:\n%s", strings.Join(problems, "\n"))
	}
}

func (h *authHarness) pendingChatUpload(t *testing.T, id string, uploader domain.UserID, channel string) {
	t.Helper()
	h.exec3(t, insertPendingChatUpload, id, uploader.String(), channel)
}

func (h *authHarness) uploadState(t *testing.T, id string) string {
	t.Helper()
	if h.countRows(t, "SELECT count(id) FROM attachments WHERE id = $1::uuid", id) == 0 {
		return "gone"
	}
	var state string
	if err := h.database.Pool.QueryRow(context.Background(), "SELECT state FROM attachments WHERE id = $1::uuid", id).Scan(&state); err != nil {
		t.Fatal(err)
	}
	return state
}

func (h *authHarness) deletionQueued(t *testing.T, id string) bool {
	t.Helper()
	return h.countRows(t, "SELECT count(id) FROM outbox WHERE lane = 'storage_delete' AND payload->>'attachment' = $1", id) > 0
}
