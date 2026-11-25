package repositories_test

import (
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/testdb"
)

const (
	repliesAndPinMigration = 46
	seedPinnedMessage      = `INSERT INTO messages (id, channel_id, seq, sender_id, body, created_at, updated_at) VALUES ($1, $2, $3, $4, 'A body', $5, $5)`
)

type twoChannels struct {
	first, second                domain.ChannelID
	inFirst, alsoFirst, inSecond domain.MessageID
}

func seedTwoGroupChannels(t *testing.T, database *testdb.Database) twoChannels {
	t.Helper()
	tutor := domain.UserID{1}
	seeded := twoChannels{
		first: domain.ChannelID{0x71}, second: domain.ChannelID{0x72},
		inFirst: domain.MessageID{0x81}, alsoFirst: domain.MessageID{0x82}, inSecond: domain.MessageID{0x83},
	}
	ctx := t.Context()
	succeeded(t)(database.Pool.Exec(ctx, seedChannelAccount, tutor, "tutor", "tutor@example.org", instant))
	succeeded(t)(database.Pool.Exec(ctx, seedChannelProfile, tutor, instant))
	succeeded(t)(database.Pool.Exec(ctx, seedChannelGroup, domain.GroupID{0x51}, tutor, instant))
	succeeded(t)(database.Pool.Exec(ctx, seedChannelGroup, domain.GroupID{0x52}, tutor, instant))
	succeeded(t)(database.Pool.Exec(ctx, seedGroupChannel, seeded.first, tutor, domain.GroupID{0x51}, instant))
	succeeded(t)(database.Pool.Exec(ctx, seedGroupChannel, seeded.second, tutor, domain.GroupID{0x52}, instant))
	succeeded(t)(database.Pool.Exec(ctx, seedPinnedMessage, seeded.inFirst, seeded.first, 1, tutor, instant))
	succeeded(t)(database.Pool.Exec(ctx, seedPinnedMessage, seeded.alsoFirst, seeded.first, 2, tutor, instant))
	succeeded(t)(database.Pool.Exec(ctx, seedPinnedMessage, seeded.inSecond, seeded.second, 1, tutor, instant))
	return seeded
}

func succeeded(t *testing.T) func(pgconn.CommandTag, error) {
	return func(_ pgconn.CommandTag, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
}

func failed(_ pgconn.CommandTag, err error) bool {
	return err != nil
}

func TestRepliesAndThePinReferenceOnlyMessagesOfTheirOwnChannel(t *testing.T) {
	t.Parallel()
	database := testdb.NewMigratedTo(t, repliesAndPinMigration)
	seeded := seedTwoGroupChannels(t, database)
	ctx := t.Context()
	const reply = `UPDATE messages SET reply_to_message_id = $2 WHERE id = $1`
	const pin = `UPDATE channels SET pinned_message_id = $2 WHERE id = $1`

	crossReply := failed(database.Pool.Exec(ctx, reply, seeded.inFirst, seeded.inSecond))
	selfReply := failed(database.Pool.Exec(ctx, reply, seeded.inFirst, seeded.inFirst))
	crossPin := failed(database.Pool.Exec(ctx, pin, seeded.first, seeded.inSecond))
	sameReply := failed(database.Pool.Exec(ctx, reply, seeded.alsoFirst, seeded.inFirst))
	samePin := failed(database.Pool.Exec(ctx, pin, seeded.first, seeded.alsoFirst))
	pinnedDelete := failed(database.Pool.Exec(ctx, `DELETE FROM messages WHERE id = $1`, seeded.alsoFirst))
	quotedDelete := failed(database.Pool.Exec(ctx, `DELETE FROM messages WHERE id = $1`, seeded.inFirst))
	replyCleared := scalar(t, database, `SELECT reply_to_message_id IS NULL FROM messages WHERE id = '`+seeded.alsoFirst.String()+`'::uuid`)
	downErr := database.MigrateDownTo(t.Context(), repliesAndPinMigration-1)

	if !crossReply || !selfReply || !crossPin {
		t.Fatalf("a reference across channels or to itself was accepted: reply %t, self %t, pin %t", crossReply, selfReply, crossPin)
	}
	if sameReply || samePin {
		t.Fatalf("a reference within the channel was refused: reply %t, pin %t", sameReply, samePin)
	}
	if !pinnedDelete {
		t.Fatal("the pinned message was deleted while still pinned")
	}
	if quotedDelete || !replyCleared {
		t.Fatalf("deleting the quoted message was refused (%t) or left the reply's link (%t)", quotedDelete, !replyCleared)
	}
	if downErr != nil {
		t.Fatalf("the migration does not go down: %v", downErr)
	}
}
