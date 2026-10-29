package startup_test

import (
	"context"
	"testing"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/testdb"
)

const (
	lastMigrationBeforeChannels = 14
	insertChatEraLink           = `INSERT INTO relationships (id, tutor_id, student_id, state, display_name, colour, linked_at, ended_at, ended_by,
    discard_reason, discarded_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, 'A student', $5, $6, $7, $8, $9, $10, $11, $11)`
	insertChatEraGroup = `INSERT INTO groups (id, tutor_id, name, colour, archived_at, created_at, updated_at)
VALUES ($1, $2, 'A group', 'moss', $3, $4, $4)`
	insertChatEraMember = `INSERT INTO group_members (group_id, relationship_id, tutor_id, added_at) VALUES ($1, $2, $3, $4)`
)

type chatEraSeed struct {
	tutor       domain.UserID
	students    map[domain.RelationshipState]domain.UserID
	links       map[domain.RelationshipState]domain.RelationshipID
	invited     domain.RelationshipID
	discarded   domain.RelationshipID
	group       domain.GroupID
	archived    domain.GroupID
	linkedAt    time.Time
	endedAt     time.Time
	archivedAt  time.Time
	groupedFrom time.Time
}

type backfilledChannel struct {
	kind                  string
	readOnlySince         *time.Time
	lastActivityAt        time.Time
	provisionalThroughSeq *int64
	members               map[domain.UserID]bool
}

func seedChatEra(t *testing.T, database *testdb.Database) chatEraSeed {
	t.Helper()
	ctx := context.Background()
	created := time.Date(2026, time.October, 25, 9, 0, 0, 0, time.UTC)
	seed := chatEraSeed{
		tutor: domain.UserID{0x10},
		students: map[domain.RelationshipState]domain.UserID{
			domain.RelationshipProvisional: {0x21}, domain.RelationshipActive: {0x22}, domain.RelationshipEnded: {0x23},
			domain.RelationshipReclaimPending: {0x24},
		},
		links: map[domain.RelationshipState]domain.RelationshipID{
			domain.RelationshipProvisional: {0x31}, domain.RelationshipActive: {0x32}, domain.RelationshipEnded: {0x33},
			domain.RelationshipReclaimPending: {0x34},
		},
		invited: domain.RelationshipID{0x35}, discarded: domain.RelationshipID{0x36},
		group: domain.GroupID{0x41}, archived: domain.GroupID{0x42},
		linkedAt: created.Add(time.Hour), endedAt: created.Add(2 * time.Hour), archivedAt: created.Add(3 * time.Hour), groupedFrom: created.Add(30 * time.Minute),
	}
	if _, err := database.Pool.Exec(ctx, insertInterimUser, seed.tutor, "tutor", "active", "tutor@example.org", &created, created); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Pool.Exec(ctx, insertInterimProfile, seed.tutor, created); err != nil {
		t.Fatal(err)
	}
	for state, student := range seed.students {
		accountState, verified := "active", &created
		if state == domain.RelationshipProvisional {
			accountState, verified = "unverified", nil
		}
		if _, err := database.Pool.Exec(ctx, insertInterimUser, student, "student", accountState, "student-"+student.String()+"@example.org", verified, created); err != nil {
			t.Fatal(err)
		}
	}
	by := "tutor"
	for state, link := range seed.links {
		student := seed.students[state]
		var endedAt *time.Time
		var endedBy *string
		if state == domain.RelationshipEnded {
			endedAt, endedBy = &seed.endedAt, &by
		}
		if _, err := database.Pool.Exec(ctx, insertChatEraLink, link, seed.tutor, student, string(state), "clay", seed.linkedAt, endedAt, endedBy, nil, nil, created); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := database.Pool.Exec(ctx, insertChatEraLink, seed.invited, seed.tutor, nil, "invited", nil, nil, nil, nil, nil, nil, created); err != nil {
		t.Fatal(err)
	}
	removed := "removed"
	if _, err := database.Pool.Exec(ctx, insertChatEraLink, seed.discarded, seed.tutor, seed.students[domain.RelationshipProvisional], "discarded", "clay",
		seed.linkedAt, nil, nil, &removed, &seed.endedAt, created); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Pool.Exec(ctx, insertChatEraGroup, seed.group, seed.tutor, nil, seed.groupedFrom); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Pool.Exec(ctx, insertChatEraGroup, seed.archived, seed.tutor, &seed.archivedAt, seed.groupedFrom); err != nil {
		t.Fatal(err)
	}
	for _, group := range []domain.GroupID{seed.group, seed.archived} {
		if _, err := database.Pool.Exec(ctx, insertChatEraMember, group, seed.links[domain.RelationshipActive], seed.tutor, seed.groupedFrom); err != nil {
			t.Fatal(err)
		}
	}
	return seed
}

func migrateChatEra(t *testing.T, database *testdb.Database) {
	t.Helper()
	migrated := runBinary(t, "api", []string{"OVC_DATABASE_URL=" + database.URL.Reveal()}, "migrate")
	if migrated.exitCode != 0 {
		t.Fatalf("migrate: %d %s", migrated.exitCode, migrated.stderr)
	}
}

func backfilled(t *testing.T, database *testdb.Database, column string, audience [16]byte) (backfilledChannel, bool) {
	t.Helper()
	ctx := context.Background()
	var count int64
	if err := database.Pool.QueryRow(ctx, "SELECT count(id) FROM channels WHERE "+column+" = $1", audience).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		return backfilledChannel{}, false
	}
	var channel backfilledChannel
	var id [16]byte
	err := database.Pool.QueryRow(ctx, "SELECT id, kind, read_only_since, last_activity_at, provisional_through_seq FROM channels WHERE "+column+" = $1", audience).
		Scan(&id, &channel.kind, &channel.readOnlySince, &channel.lastActivityAt, &channel.provisionalThroughSeq)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := database.Pool.Query(ctx, "SELECT user_id, suspended_since IS NOT NULL FROM channel_members WHERE channel_id = $1", id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	channel.members = map[domain.UserID]bool{}
	for rows.Next() {
		var user domain.UserID
		var suspended bool
		if err := rows.Scan(&user, &suspended); err != nil {
			t.Fatal(err)
		}
		channel.members[user] = suspended
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return channel, true
}

func TestTheMigrationCreatesChannelsForExistingRelationshipsAndGroups(t *testing.T) {
	t.Parallel()
	holdProcessSlotAfterParallel(t)
	database := testdb.NewMigratedTo(t, lastMigrationBeforeChannels)
	seed := seedChatEra(t, database)

	migrateChatEra(t, database)

	for _, state := range []domain.RelationshipState{domain.RelationshipProvisional, domain.RelationshipActive, domain.RelationshipEnded} {
		channel, found := backfilled(t, database, "relationship_id", seed.links[state])
		_, tutorListed := channel.members[seed.tutor]
		_, studentListed := channel.members[seed.students[state]]
		if !found || channel.kind != "direct" || !channel.lastActivityAt.Equal(seed.linkedAt) || len(channel.members) != 2 ||
			!tutorListed || !studentListed || channel.members[seed.tutor] || channel.members[seed.students[state]] {
			t.Fatalf("the %s relationship's channel is %+v (found %t)", state, channel, found)
		}
		if readOnly := channel.readOnlySince != nil; readOnly != (state == domain.RelationshipEnded) {
			t.Fatalf("the %s relationship's channel is read-only since %v", state, channel.readOnlySince)
		}
	}
	if ended, _ := backfilled(t, database, "relationship_id", seed.links[domain.RelationshipEnded]); !ended.readOnlySince.Equal(seed.endedAt) {
		t.Fatalf("the ended channel is read-only since %v, not since the ending", ended.readOnlySince)
	}
	for name, link := range map[string]domain.RelationshipID{"invited": seed.invited, "discarded": seed.discarded} {
		if _, found := backfilled(t, database, "relationship_id", link); found {
			t.Fatalf("the %s card was given a channel", name)
		}
	}
	group, groupFound := backfilled(t, database, "group_id", seed.group)
	archived, archivedFound := backfilled(t, database, "group_id", seed.archived)
	member := seed.students[domain.RelationshipActive]
	for name, channel := range map[string]backfilledChannel{"active": group, "archived": archived} {
		if channel.kind != "group" || len(channel.members) != 2 || channel.members[seed.tutor] || channel.members[member] {
			t.Fatalf("the %s group's channel is %+v", name, channel)
		}
	}
	if !groupFound || !archivedFound || group.readOnlySince != nil || archived.readOnlySince == nil || !archived.readOnlySince.Equal(seed.archivedAt) {
		t.Fatalf("the group channels are %+v and %+v", group, archived)
	}
}

func TestTheBackfillSuspendsTheStudentOfAReclaimPendingChannel(t *testing.T) {
	t.Parallel()
	holdProcessSlotAfterParallel(t)
	database := testdb.NewMigratedTo(t, lastMigrationBeforeChannels)
	seed := seedChatEra(t, database)

	migrateChatEra(t, database)

	channel, found := backfilled(t, database, "relationship_id", seed.links[domain.RelationshipReclaimPending])
	student := seed.students[domain.RelationshipReclaimPending]
	suspended, listed := channel.members[student]
	tutorSuspended, tutorListed := channel.members[seed.tutor]
	if !found || !listed || !suspended || !tutorListed || tutorSuspended || len(channel.members) != 2 {
		t.Fatalf("the reclaim-pending channel holds %+v", channel.members)
	}
	if channel.readOnlySince != nil || channel.provisionalThroughSeq == nil || *channel.provisionalThroughSeq != 0 {
		t.Fatalf("the reclaim-pending channel is %+v", channel)
	}
}
