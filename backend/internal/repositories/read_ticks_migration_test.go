package repositories_test

import (
	"testing"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/testdb"
)

const readTicksMigration = 47

type readTicksRow struct {
	name    string
	channel domain.ChannelID
	user    domain.UserID
}

func seedReadTicksChannels(t *testing.T, database *testdb.Database) []readTicksRow {
	t.Helper()
	ctx := t.Context()
	tutor, group := domain.UserID{1}, domain.GroupID{0x51}
	writable, suspended, readOnly, discarded, inGroup := domain.ChannelID{0x61}, domain.ChannelID{0x62}, domain.ChannelID{0x63}, domain.ChannelID{0x64},
		domain.ChannelID{0x65}
	students := []domain.UserID{{2}, {3}, {4}, {5}}
	links := []domain.RelationshipID{{0x41}, {0x42}, {0x43}, {0x44}}
	var none *time.Time
	at := instant
	succeeded(t)(database.Pool.Exec(ctx, seedChannelAccount, tutor, "tutor", "tutor@example.org", at))
	succeeded(t)(database.Pool.Exec(ctx, seedChannelProfile, tutor, at))
	succeeded(t)(database.Pool.Exec(ctx, seedChannelGroup, group, tutor, at))
	succeeded(t)(database.Pool.Exec(ctx, seedGroupChannel, inGroup, tutor, group, at))
	succeeded(t)(database.Pool.Exec(ctx, seedChannelMember, inGroup, tutor, none, at))
	for index, channel := range []domain.ChannelID{writable, suspended, readOnly, discarded} {
		succeeded(t)(database.Pool.Exec(ctx, seedChannelAccount, students[index], "student", students[index].String()+"@example.org", at))
		succeeded(t)(database.Pool.Exec(ctx, seedChannelLink, links[index], tutor, students[index], at))
		discardedAt := none
		if channel == discarded {
			discardedAt = &at
		}
		succeeded(t)(database.Pool.Exec(ctx, seedDirectChannel, channel, tutor, links[index], at, discardedAt, none))
		suspendedAt := none
		if channel == suspended {
			suspendedAt = &at
		}
		succeeded(t)(database.Pool.Exec(ctx, seedChannelMember, channel, tutor, none, at))
		succeeded(t)(database.Pool.Exec(ctx, seedChannelMember, channel, students[index], suspendedAt, at))
	}
	succeeded(t)(database.Pool.Exec(ctx, seedChannelMember, inGroup, students[0], none, at))
	succeeded(t)(database.Pool.Exec(ctx, `UPDATE channels SET read_only_since = $2 WHERE id = $1`, readOnly, at))
	succeeded(t)(database.Pool.Exec(ctx, `UPDATE channel_members SET last_read_seq = 1`))
	return []readTicksRow{
		{name: "the tutor in a writable direct channel", channel: writable, user: tutor},
		{name: "the student in a writable direct channel", channel: writable, user: students[0]},
		{name: "the tutor beside a suspended student", channel: suspended, user: tutor},
		{name: "a suspended student", channel: suspended, user: students[1]},
		{name: "the tutor in a read-only channel", channel: readOnly, user: tutor},
		{name: "the student in a read-only channel", channel: readOnly, user: students[2]},
		{name: "the tutor in a discarded channel", channel: discarded, user: tutor},
		{name: "the tutor in a group", channel: inGroup, user: tutor},
		{name: "a student in a group", channel: inGroup, user: students[0]},
	}
}

func TestReadTicksMigrationStartsEverySharedReadMarkerAtZero(t *testing.T) {
	t.Parallel()
	database := testdb.NewMigratedTo(t, readTicksMigration-1)
	rows := seedReadTicksChannels(t, database)
	ctx := t.Context()

	upErr := database.MigrateUpTo(ctx, readTicksMigration)
	startingMarkers := make([]int64, 0, len(rows))
	for _, row := range rows {
		var shared int64
		if err := database.Pool.QueryRow(ctx, `SELECT shared_read_seq FROM channel_members WHERE channel_id = $1 AND user_id = $2`, row.channel, row.user).
			Scan(&shared); err != nil {
			t.Fatal(err)
		}
		startingMarkers = append(startingMarkers, shared)
	}
	aheadOfRead := failed(database.Pool.Exec(ctx, `UPDATE channel_members SET shared_read_seq = last_read_seq + 1 WHERE channel_id = $1`, rows[0].channel))
	downErr := database.MigrateDownTo(ctx, readTicksMigration-1)
	columnGone := scalar(t, database, `SELECT NOT EXISTS (
    SELECT 1 FROM information_schema.columns WHERE table_name = 'channel_members' AND column_name = 'shared_read_seq'
)`)

	if upErr != nil || downErr != nil {
		t.Fatalf("the migration went up with %v and down with %v", upErr, downErr)
	}
	for index, row := range rows {
		if startingMarkers[index] != 0 {
			t.Errorf("%s starts with the exposed marker %d", row.name, startingMarkers[index])
		}
	}
	if !aheadOfRead {
		t.Fatal("a shared read marker ahead of the member's own marker was accepted")
	}
	if !columnGone {
		t.Fatal("the down migration kept the shared read marker")
	}
}
