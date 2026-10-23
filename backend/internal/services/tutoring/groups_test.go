package tutoring_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/tutoring"
)

const (
	seedGroupsOfTutor = `INSERT INTO groups (id, tutor_id, name, colour, created_at, updated_at)
SELECT gen_random_uuid(), $1, 'A seeded group', 'teal', $2, $2 FROM generate_series(1, $3::integer)`
	seedMembersOfEveryGroup = `INSERT INTO group_members (group_id, relationship_id, tutor_id, added_at)
SELECT groups.id, relationships.id, groups.tutor_id, $2 FROM groups
JOIN relationships ON relationships.tutor_id = groups.tutor_id AND relationships.state = 'active'
WHERE groups.tutor_id = $1
ON CONFLICT DO NOTHING`
	queryCompleted = `"msg":"db.query.completed"`
	groupMembers   = 6
)

func (f cardFixture) seedActiveStudents(t *testing.T, instant time.Time, count int) {
	t.Helper()
	repos := f.store.Repositories()
	for index := range count {
		student := domain.UserID{0x5e, byte(index)}
		err := repos.Users.Create(context.Background(), repositories.NewUser{
			ID: student, Role: domain.RoleStudent, State: domain.AccountActive, Email: fmt.Sprintf("member-%d@example.org", index),
			EmailVerifiedAt: &instant, PasswordHash: fixturePasswordHash(), Name: fmt.Sprintf("Member %d", index),
			Language: domain.LanguageEnglish, Timezone: "Europe/Moscow", Theme: domain.ThemeSystem, ConsentVersion: "2026-10",
			ConsentAt: instant, CreatedAt: instant,
		})
		if err != nil {
			t.Fatal(err)
		}
		colour := domain.ColourMoss
		err = repos.Relationships.Create(context.Background(), repositories.NewRelationship{
			ID: domain.RelationshipID{0x5e, byte(index)}, TutorID: f.tutor, StudentID: &student, State: domain.RelationshipActive,
			DisplayName: "A member", Colour: &colour, LinkedAt: &instant, CreatedAt: instant,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func (f cardFixture) seedGroupsWithMembers(t *testing.T, instant time.Time, count int) {
	t.Helper()
	if _, err := f.database.Pool.Exec(context.Background(), seedGroupsOfTutor, f.tutor, instant, count); err != nil {
		t.Fatal(err)
	}
	if _, err := f.database.Pool.Exec(context.Background(), seedMembersOfEveryGroup, f.tutor, instant); err != nil {
		t.Fatal(err)
	}
}

func tracedService(t *testing.T, f cardFixture, instant time.Time, logs *bytes.Buffer) *tutoring.Service {
	t.Helper()
	logger := slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	pool, err := core.OpenPool(context.Background(), core.Settings{DatabaseURL: f.database.URL, DBPoolSize: 2}, logger, clock.System{}.Now)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	fieldKey := sha256.Sum256([]byte("test-field-key"))
	cipher, err := core.NewFieldCipher(core.NewSecret(base64.StdEncoding.EncodeToString(fieldKey[:])), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := core.DeriveKeys(core.NewSecret("test-signing-material-0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	return tutoring.New(tutoring.Dependencies{
		Store: repositories.NewStore(pool), Clock: fixedClock{instant: instant}, Random: rand.Reader, Keys: keys, Cipher: cipher,
		PublicURL: "https://ovc.example", Logger: slog.New(slog.DiscardHandler),
	})
}

func listedWithQueries(t *testing.T, service *tutoring.Service, tutor domain.UserID, logs *bytes.Buffer) ([]tutoring.GroupSummary, int) {
	t.Helper()
	logs.Reset()
	groups, err := service.ListGroups(context.Background(), tutor)
	if err != nil {
		t.Fatal(err)
	}
	return groups, strings.Count(logs.String(), queryCompleted)
}

func TestGroupListUsesBatchedMemberQueries(t *testing.T) {
	f := newCardFixture(t, subMicrosecond)
	f.seedActiveStudents(t, subMicrosecond, groupMembers)
	logs := &bytes.Buffer{}
	service := tracedService(t, f, subMicrosecond, logs)
	f.seedGroupsWithMembers(t, subMicrosecond, 1)
	one, queriesForOne := listedWithQueries(t, service, f.tutor, logs)
	f.seedGroupsWithMembers(t, subMicrosecond, 39)

	many, queriesForMany := listedWithQueries(t, service, f.tutor, logs)

	if queriesForOne == 0 || queriesForMany != queriesForOne {
		t.Fatalf("listing 1 group took %d queries and 40 groups %d", queriesForOne, queriesForMany)
	}
	if len(one) != 1 || len(many) != 40 {
		t.Fatalf("listed %d and then %d groups", len(one), len(many))
	}
	for _, group := range many {
		if group.MemberCount != groupMembers || len(group.FirstMembers) != domain.GroupPreviewMembers || group.FirstMembers[0].Name != "Member 0" {
			t.Fatalf("a group lists %d members of %d: %+v", len(group.FirstMembers), group.MemberCount, group.FirstMembers)
		}
	}
}
