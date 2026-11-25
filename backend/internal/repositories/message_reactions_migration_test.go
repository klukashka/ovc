package repositories_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/testdb"
)

const (
	messageReactionsMigration = 48
	seedReaction              = `INSERT INTO message_reactions (message_id, user_id, reaction, created_at) VALUES ($1, $2, $3, $4)`
	lockedElsewhere           = `SELECT id FROM channels WHERE id = $1 FOR UPDATE NOWAIT`
	lockNotAvailable          = "55P03"
)

func refusedForTheLock(_ pgconn.CommandTag, err error) bool {
	var postgresErr *pgconn.PgError
	return errors.As(err, &postgresErr) && postgresErr.Code == lockNotAvailable
}

type reactionWorld struct {
	tutor   domain.UserID
	student domain.UserID
	channel domain.ChannelID
	message domain.MessageID
}

func seedReactionWorld(t *testing.T, database *testdb.Database) reactionWorld {
	t.Helper()
	ctx := t.Context()
	world := reactionWorld{tutor: domain.UserID{1}, student: domain.UserID{2}, channel: domain.ChannelID{0x71}, message: domain.MessageID{0x81}}
	succeeded(t)(database.Pool.Exec(ctx, seedChannelAccount, world.tutor, "tutor", "tutor@example.org", instant))
	succeeded(t)(database.Pool.Exec(ctx, seedChannelProfile, world.tutor, instant))
	succeeded(t)(database.Pool.Exec(ctx, seedChannelAccount, world.student, "student", "student@example.org", instant))
	succeeded(t)(database.Pool.Exec(ctx, seedChannelGroup, domain.GroupID{0x51}, world.tutor, instant))
	succeeded(t)(database.Pool.Exec(ctx, seedGroupChannel, world.channel, world.tutor, domain.GroupID{0x51}, instant))
	succeeded(t)(database.Pool.Exec(ctx, seedPinnedMessage, world.message, world.channel, 1, world.tutor, instant))
	return world
}

func TestMessageReactionsMigrationChecksOneReactionPerMemberAndCascades(t *testing.T) {
	t.Parallel()
	database := testdb.NewMigratedTo(t, messageReactionsMigration)
	world := seedReactionWorld(t, database)
	ctx := t.Context()
	succeeded(t)(database.Pool.Exec(ctx, seedReaction, world.message, world.student, "heart", instant))

	unknown := failed(database.Pool.Exec(ctx, seedReaction, world.message, world.tutor, "rocket", instant))
	second := failed(database.Pool.Exec(ctx, seedReaction, world.message, world.student, "joy", instant))
	succeeded(t)(database.Pool.Exec(ctx, `DELETE FROM messages WHERE id = $1`, world.message))
	cascaded := scalar(t, database, `SELECT NOT EXISTS (SELECT 1 FROM message_reactions)`)
	downErr := database.MigrateDownTo(ctx, messageReactionsMigration-1)
	dropped := scalar(t, database, `SELECT to_regclass('message_reactions') IS NULL`)

	if !unknown || !second {
		t.Fatalf("an unknown value or a second reaction was accepted: unknown %t, second %t", unknown, second)
	}
	if !cascaded || downErr != nil || !dropped {
		t.Fatalf("cascade %t, down %v, dropped %t", cascaded, downErr, dropped)
	}
}

func TestStudentDeletionLocksTheChannelsAndTutorsOfTheAccountsReactions(t *testing.T) {
	t.Parallel()
	database := testdb.New(t)
	world := seedReactionWorld(t, database)
	ctx := t.Context()
	succeeded(t)(database.Pool.Exec(ctx, seedReaction, world.message, world.student, "check", instant))
	store := repositories.NewStore(database.Pool)
	locked := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan error, 1)

	go func() {
		finished <- store.WithTx(context.Background(), func(repos repositories.Repositories) error {
			if err := repos.Channels.LockForStudentDeletion(context.Background(), world.student, nil, nil); err != nil {
				return err
			}
			close(locked)
			<-release
			return nil
		})
	}()
	select {
	case <-locked:
	case err := <-finished:
		t.Fatal(err)
	}
	elsewhere := refusedForTheLock(database.Pool.Exec(ctx, lockedElsewhere, world.channel))
	close(release)
	lockErr := <-finished
	tutors, tutorsErr := store.Repositories().Channels.StudentDeletionTutors(ctx, world.student)

	if lockErr != nil || tutorsErr != nil {
		t.Fatal(errors.Join(lockErr, tutorsErr))
	}
	if !elsewhere {
		t.Fatal("the channel of the student's reaction was not locked for the deletion")
	}
	if !slices.Contains(tutors, world.tutor) {
		t.Fatalf("the deletion's tutors %v miss the reaction channel's tutor", tutors)
	}
}
