package tutoring_test

import (
	"context"
	"crypto/rand"
	"log/slog"
	"slices"
	"testing"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/chat"
	"ovc/backend/internal/services/tutoring"
)

type purgedCounts struct {
	members, channels, relationships int64
}

func (f cardFixture) purgedCounts(t *testing.T) purgedCounts {
	t.Helper()
	var counts purgedCounts
	err := f.database.Pool.QueryRow(context.Background(), `SELECT
    (SELECT count(channel_members.user_id) FROM channel_members JOIN channels ON channels.id = channel_members.channel_id WHERE channels.tutor_id = $1),
    (SELECT count(id) FROM channels WHERE tutor_id = $1),
    (SELECT count(id) FROM relationships WHERE tutor_id = $1)`, f.tutor).Scan(&counts.members, &counts.channels, &counts.relationships)
	if err != nil {
		t.Fatal(err)
	}
	return counts
}

func (f cardFixture) channelsForEveryRelationship(t *testing.T) {
	t.Helper()
	rows, err := f.database.Pool.Query(context.Background(), "SELECT id FROM relationships WHERE tutor_id = $1 ORDER BY id", f.tutor)
	if err != nil {
		t.Fatal(err)
	}
	var links []domain.RelationshipID
	for rows.Next() {
		var link domain.RelationshipID
		if err := rows.Scan(&link); err != nil {
			t.Fatal(err)
		}
		links = append(links, link)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	mirror := chat.Mirror{Random: rand.Reader, Logger: slog.New(slog.DiscardHandler)}
	err = f.store.WithTx(context.Background(), func(repos repositories.Repositories) error {
		return mirror.Apply(context.Background(), repos, domain.PathLinking, chat.Audiences{Relationships: links}, subMicrosecond)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTutorPurgeDeletesChannelsInBatches(t *testing.T) {
	t.Parallel()
	f := newCardFixture(t, subMicrosecond)
	if _, err := f.database.Pool.Exec(context.Background(), insertActiveStudents, subMicrosecond, fixturePasswordHash(), 3); err != nil {
		t.Fatal(err)
	}
	if _, err := f.database.Pool.Exec(context.Background(), insertActiveRelationships, f.tutor, subMicrosecond); err != nil {
		t.Fatal(err)
	}
	f.channelsForEveryRelationship(t)
	service := tutoring.New(tutoring.Dependencies{Store: f.store, Random: rand.Reader, Logger: slog.New(slog.DiscardHandler), PurgeBatchRows: 4})
	seeded := f.purgedCounts(t)

	var after []purgedCounts
	for range 6 {
		err := f.store.WithTx(context.Background(), func(repos repositories.Repositories) error {
			_, err := service.PurgeTutorBatch(context.Background(), repos, f.tutor)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		after = append(after, f.purgedCounts(t))
	}

	want := []purgedCounts{{2, 3, 3}, {0, 3, 3}, {0, 0, 3}, {0, 0, 0}, {0, 0, 0}, {0, 0, 0}}
	if seeded != (purgedCounts{6, 3, 3}) || !slices.Equal(after, want) {
		t.Fatalf("seeded %+v, then after each batch %+v, want %+v", seeded, after, want)
	}
}
