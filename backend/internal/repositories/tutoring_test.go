package repositories_test

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/testdb"
)

const (
	uniqueViolation = "23505"
	checkViolation  = "23514"

	foreignKeyViolation = "23503"
)

type tutoringFixture struct {
	pool  *pgxpool.Pool
	store *repositories.Store
	repos repositories.Repositories
}

func newTutoringFixture(t *testing.T) tutoringFixture {
	t.Helper()
	pool := testdb.New(t).Pool
	store := repositories.NewStore(pool)
	return tutoringFixture{pool: pool, store: store, repos: store.Repositories()}
}

func (f tutoringFixture) account(t *testing.T, id byte, role domain.Role) domain.UserID {
	t.Helper()
	user := domain.UserID{id}
	err := f.repos.Users.Create(context.Background(), repositories.NewUser{
		ID: user, Role: role, State: domain.AccountActive, Email: string(rune('a'+id%26)) + string(rune('a'+id/26)) + "@example.org",
		EmailVerifiedAt: &instant, PasswordHash: fixturePasswordHash(), Name: "An account", Language: domain.LanguageEnglish,
		Timezone: "Europe/Moscow", Theme: domain.ThemeSystem, ConsentVersion: "2026-10", ConsentAt: instant, CreatedAt: instant,
	})
	if err != nil {
		t.Fatal(err)
	}
	return user
}

func (f tutoringFixture) tutor(t *testing.T, id byte) domain.UserID {
	t.Helper()
	user := f.account(t, id, domain.RoleTutor)
	profile := repositories.NewTutorProfile{UserID: user, FileQuotaBytes: 1, RecordingQuotaBytes: 1, CreatedAt: instant}
	if err := f.repos.TutorProfiles.Create(context.Background(), profile, bytes.NewReader([]byte{id, id, id, id, id, id, id, id})); err != nil {
		t.Fatal(err)
	}
	return user
}

func (f tutoringFixture) student(t *testing.T, id byte) domain.UserID {
	t.Helper()
	return f.account(t, id, domain.RoleStudent)
}

func invitedCard(id byte, tutor domain.UserID) repositories.NewRelationship {
	return repositories.NewRelationship{
		ID: domain.RelationshipID{id}, TutorID: tutor, State: domain.RelationshipInvited, DisplayName: "A student", CreatedAt: instant,
	}
}

func linkedCard(id byte, tutor domain.UserID, student domain.UserID, state domain.RelationshipState) repositories.NewRelationship {
	card := invitedCard(id, tutor)
	colour := domain.ColourClay
	card.StudentID, card.State, card.Colour, card.LinkedAt = &student, state, &colour, &instant
	if state == domain.RelationshipEnded {
		by := domain.EndedByTutor
		card.EndedAt, card.EndedBy = &instant, &by
	}
	return card
}

func (f tutoringFixture) card(t *testing.T, card repositories.NewRelationship) domain.RelationshipID {
	t.Helper()
	if err := f.repos.Relationships.Create(context.Background(), card); err != nil {
		t.Fatal(err)
	}
	return card.ID
}

func openInvitation(id byte, card domain.RelationshipID) repositories.NewInvitation {
	return repositories.NewInvitation{
		ID: domain.InvitationID{id}, RelationshipID: card, TokenHash: hash(string(rune('a' + id))), TokenEncrypted: []byte("ciphertext"),
		ExpiresAt: instant.Add(14 * 24 * time.Hour), CreatedAt: instant,
	}
}

func violation(err error) (string, string) {
	var refused *pgconn.PgError
	if errors.As(err, &refused) {
		return refused.Code, refused.ConstraintName
	}
	return "", ""
}

func (f tutoringFixture) invitationState(t *testing.T, id domain.InvitationID) (bool, bool, string, int16) {
	t.Helper()
	var readable, revoked bool
	var reason *string
	var mismatches int16
	err := f.pool.QueryRow(context.Background(),
		"SELECT token_encrypted IS NOT NULL, revoked_at IS NOT NULL, revoke_reason, mismatch_count FROM invitations WHERE id = $1",
		id).Scan(&readable, &revoked, &reason, &mismatches)
	if err != nil {
		t.Fatal(err)
	}
	if reason == nil {
		return readable, revoked, "", mismatches
	}
	return readable, revoked, *reason, mismatches
}

func TestInvitationConsumptionIsSingleUse(t *testing.T) {
	t.Parallel()
	f := newTutoringFixture(t)
	tutor := f.tutor(t, 1)
	card := f.card(t, invitedCard(10, tutor))
	invitation := openInvitation(20, card)
	if err := f.repos.Invitations.Create(context.Background(), invitation); err != nil {
		t.Fatal(err)
	}
	const contenders = 8
	students := make([]domain.UserID, contenders)
	for index := range students {
		students[index] = f.student(t, byte(30+index))
	}

	results := make(chan error, contenders)
	var wait sync.WaitGroup
	for _, student := range students {
		wait.Go(func() {
			_, err := f.repos.Invitations.Consume(context.Background(), invitation.TokenHash, student, instant.Add(time.Hour))
			results <- err
		})
	}
	wait.Wait()
	close(results)

	consumed := 0
	for err := range results {
		switch {
		case err == nil:
			consumed++
		case !errors.Is(err, repositories.ErrNotFound):
			t.Fatal(err)
		}
	}
	if consumed != 1 {
		t.Fatalf("%d consumptions succeeded, want exactly one", consumed)
	}
	if readable, _, _, _ := f.invitationState(t, invitation.ID); readable {
		t.Fatal("a used invitation kept its ciphertext")
	}
}

func TestExpiredOrBoundInvitationIsConsumedOnlyByItsAccount(t *testing.T) {
	t.Parallel()
	f := newTutoringFixture(t)
	tutor := f.tutor(t, 1)
	bound, stranger := f.student(t, 30), f.student(t, 31)
	expired := openInvitation(20, f.card(t, invitedCard(10, tutor)))
	boundLink := openInvitation(21, f.card(t, linkedCard(11, tutor, bound, domain.RelationshipEnded)))
	boundLink.BoundStudentID = &bound
	for _, invitation := range []repositories.NewInvitation{expired, boundLink} {
		if err := f.repos.Invitations.Create(context.Background(), invitation); err != nil {
			t.Fatal(err)
		}
	}

	_, atExpiry := f.repos.Invitations.Consume(context.Background(), expired.TokenHash, stranger, expired.ExpiresAt)
	_, byStranger := f.repos.Invitations.Consume(context.Background(), boundLink.TokenHash, stranger, instant)
	byOwner, ownerErr := f.repos.Invitations.Consume(context.Background(), boundLink.TokenHash, bound, instant)

	if !errors.Is(atExpiry, repositories.ErrNotFound) {
		t.Fatalf("an invitation was consumed at its expiry: %v", atExpiry)
	}
	if !errors.Is(byStranger, repositories.ErrNotFound) {
		t.Fatalf("another account consumed a bound link: %v", byStranger)
	}
	if ownerErr != nil || byOwner.ID != boundLink.ID {
		t.Fatalf("the bound account could not consume its link after a stranger tried: %+v %v", byOwner, ownerErr)
	}
}

func TestOneOpenInvitationPerCard(t *testing.T) {
	t.Parallel()
	f := newTutoringFixture(t)
	tutor := f.tutor(t, 1)
	card := f.card(t, invitedCard(10, tutor))
	first, second := openInvitation(20, card), openInvitation(21, card)
	if err := f.repos.Invitations.Create(context.Background(), first); err != nil {
		t.Fatal(err)
	}

	refused := f.repos.Invitations.Create(context.Background(), second)
	revoked, revokeErr := f.repos.Invitations.RevokeOpen(context.Background(), tutor, card, domain.RevokeRegenerated, instant)
	replaced := f.repos.Invitations.Create(context.Background(), second)

	if code, constraint := violation(refused); code != uniqueViolation || constraint != "invitations_one_open_idx" {
		t.Fatalf("a second open invitation: %v", refused)
	}
	if !revoked || revokeErr != nil || replaced != nil {
		t.Fatalf("revoke %v %v, replacement %v", revoked, revokeErr, replaced)
	}
	if readable, isRevoked, reason, _ := f.invitationState(t, first.ID); readable || !isRevoked || reason != string(domain.RevokeRegenerated) {
		t.Fatalf("the revoked invitation: readable %v, revoked %v, reason %s", readable, isRevoked, reason)
	}
}

func TestAnotherTutorCannotRevokeAnInvitation(t *testing.T) {
	t.Parallel()
	f := newTutoringFixture(t)
	tutor, other := f.tutor(t, 1), f.tutor(t, 2)
	invitation := openInvitation(20, f.card(t, invitedCard(10, tutor)))
	if err := f.repos.Invitations.Create(context.Background(), invitation); err != nil {
		t.Fatal(err)
	}

	revoked, err := f.repos.Invitations.RevokeOpen(context.Background(), other, invitation.RelationshipID, domain.RevokeWithdrawn, instant)

	if revoked || err != nil {
		t.Fatalf("another tutor revoked the invitation: %v %v", revoked, err)
	}
	if readable, isRevoked, _, _ := f.invitationState(t, invitation.ID); !readable || isRevoked {
		t.Fatalf("the invitation changed: readable %v, revoked %v", readable, isRevoked)
	}
}

func TestMismatchIsCountedOnlyForTheUsableInvitationOfThePresentedToken(t *testing.T) {
	t.Parallel()
	f := newTutoringFixture(t)
	tutor, other := f.tutor(t, 1), f.tutor(t, 2)
	presented := openInvitation(20, f.card(t, invitedCard(10, tutor)))
	otherTutors := openInvitation(21, f.card(t, invitedCard(11, other)))
	for _, invitation := range []repositories.NewInvitation{presented, otherTutors} {
		if err := f.repos.Invitations.Create(context.Background(), invitation); err != nil {
			t.Fatal(err)
		}
	}

	_, unknown := f.repos.Invitations.CountMismatch(context.Background(), hash("an unknown token"), instant)
	_, expired := f.repos.Invitations.CountMismatch(context.Background(), presented.TokenHash, presented.ExpiresAt)
	counted, countErr := f.repos.Invitations.CountMismatch(context.Background(), presented.TokenHash, instant)

	if !errors.Is(unknown, repositories.ErrNotFound) || !errors.Is(expired, repositories.ErrNotFound) {
		t.Fatalf("unknown token %v, expired invitation %v", unknown, expired)
	}
	if counted != 1 || countErr != nil {
		t.Fatalf("presented token counted %d, %v", counted, countErr)
	}
	if _, _, _, mismatches := f.invitationState(t, otherTutors.ID); mismatches != 0 {
		t.Fatalf("another tutor's invitation counted %d mismatches", mismatches)
	}
}

func TestOneProvisionalRelationshipPerStudent(t *testing.T) {
	t.Parallel()
	f := newTutoringFixture(t)
	first, second := f.tutor(t, 1), f.tutor(t, 2)
	student := f.student(t, 3)
	f.card(t, linkedCard(10, first, student, domain.RelationshipProvisional))

	refused := f.repos.Relationships.Create(context.Background(), linkedCard(11, second, student, domain.RelationshipProvisional))
	active := f.repos.Relationships.Create(context.Background(), linkedCard(12, second, student, domain.RelationshipActive))
	duplicate := f.repos.Relationships.Create(context.Background(), linkedCard(13, second, student, domain.RelationshipEnded))

	if code, constraint := violation(refused); code != uniqueViolation || constraint != "relationships_one_provisional_idx" {
		t.Fatalf("a second provisional link: %v", refused)
	}
	if active != nil {
		t.Fatalf("an active link beside the provisional one: %v", active)
	}
	if code, constraint := violation(duplicate); code != uniqueViolation || constraint != "relationships_tutor_student_idx" {
		t.Fatalf("a second relationship with the same tutor: %v", duplicate)
	}
}

func joinRequest(id byte, tutor domain.UserID, student domain.UserID, state domain.JoinRequestState) repositories.NewJoinRequest {
	return repositories.NewJoinRequest{
		ID: domain.JoinRequestID{id}, TutorID: tutor, StudentID: student, State: state, Suppressed: domain.SuppressionNone,
		ExpiresAt: instant.Add(14 * 24 * time.Hour), CreatedAt: instant,
	}
}

func TestOnePendingRequestPerTutorAndStudent(t *testing.T) {
	t.Parallel()
	f := newTutoringFixture(t)
	tutor, other := f.tutor(t, 1), f.tutor(t, 2)
	student := f.student(t, 3)
	pending := joinRequest(10, tutor, student, domain.JoinRequestPending)
	if err := f.repos.JoinRequests.Create(context.Background(), pending); err != nil {
		t.Fatal(err)
	}

	refused := f.repos.JoinRequests.Create(context.Background(), joinRequest(11, tutor, student, domain.JoinRequestHeld))
	elsewhere := f.repos.JoinRequests.Create(context.Background(), joinRequest(12, other, student, domain.JoinRequestHeld))
	outOfScope, outOfScopeErr := f.repos.JoinRequests.Decline(context.Background(), other, pending.ID, instant)
	declined, declineErr := f.repos.JoinRequests.Decline(context.Background(), tutor, pending.ID, instant)
	again, againErr := f.repos.JoinRequests.Accept(context.Background(), tutor, pending.ID, instant)
	fresh := f.repos.JoinRequests.Create(context.Background(), joinRequest(13, tutor, student, domain.JoinRequestPending))

	if code, constraint := violation(refused); code != uniqueViolation || constraint != "join_requests_open_idx" {
		t.Fatalf("a second open request: %v", refused)
	}
	if elsewhere != nil || outOfScope || outOfScopeErr != nil {
		t.Fatalf("another tutor: create %v, decided %v %v", elsewhere, outOfScope, outOfScopeErr)
	}
	if !declined || declineErr != nil || again || againErr != nil {
		t.Fatalf("decline %v %v, accept after decline %v %v", declined, declineErr, again, againErr)
	}
	if fresh != nil {
		t.Fatalf("a new request after a decline: %v", fresh)
	}
}

func TestMismatchCounterRevokesAtFiveUnderConcurrency(t *testing.T) {
	t.Parallel()
	f := newTutoringFixture(t)
	invitation := openInvitation(20, f.card(t, invitedCard(10, f.tutor(t, 1))))
	if err := f.repos.Invitations.Create(context.Background(), invitation); err != nil {
		t.Fatal(err)
	}
	const attempts = 12

	counts := make(chan int16, attempts)
	var wait sync.WaitGroup
	for range attempts {
		wait.Go(func() {
			count, err := f.repos.Invitations.CountMismatch(context.Background(), invitation.TokenHash, instant)
			switch {
			case err == nil:
				counts <- count
			case !errors.Is(err, repositories.ErrNotFound):
				t.Error(err)
			}
		})
	}
	wait.Wait()
	close(counts)

	seen := slices.Sorted(func(yield func(int16) bool) {
		for count := range counts {
			if !yield(count) {
				return
			}
		}
	})
	if !slices.Equal(seen, []int16{1, 2, 3, 4, 5}) {
		t.Fatalf("counted mismatches %v, want 1 to 5 once each", seen)
	}
	readable, revoked, reason, mismatches := f.invitationState(t, invitation.ID)
	if readable || !revoked || reason != string(domain.RevokeMismatchLimit) || mismatches != domain.InvitationMismatchLimit {
		t.Fatalf("readable %v, revoked %v, reason %s, count %d", readable, revoked, reason, mismatches)
	}
}

func TestRemovalSucceedsOnlyWhileProvisional(t *testing.T) {
	t.Parallel()
	f := newTutoringFixture(t)
	tutor, other := f.tutor(t, 1), f.tutor(t, 2)
	email, notes := "student@example.org", "Prefers mornings"
	states := []domain.RelationshipState{
		domain.RelationshipInvited, domain.RelationshipProvisional, domain.RelationshipReclaimPending, domain.RelationshipActive, domain.RelationshipEnded,
	}
	cards := map[domain.RelationshipState]domain.RelationshipID{}
	for index, state := range states {
		card := linkedCard(byte(10+index), tutor, f.student(t, byte(30+index)), state)
		if state == domain.RelationshipInvited {
			card = invitedCard(byte(10+index), tutor)
		}
		if state == domain.RelationshipProvisional {
			card.Email, card.Notes = &email, notes
		}
		cards[state] = f.card(t, card)
	}

	_, outOfScope := f.repos.Relationships.Remove(context.Background(), other, cards[domain.RelationshipProvisional], instant)
	removed, removeErr := f.repos.Relationships.Remove(context.Background(), tutor, cards[domain.RelationshipProvisional], instant)
	_, repeated := f.repos.Relationships.Remove(context.Background(), tutor, cards[domain.RelationshipProvisional], instant)
	refusals := map[domain.RelationshipState]error{}
	for _, state := range []domain.RelationshipState{domain.RelationshipInvited, domain.RelationshipActive, domain.RelationshipEnded} {
		_, refusals[state] = f.repos.Relationships.Remove(context.Background(), tutor, cards[state], instant)
	}
	_, pendingErr := f.repos.Relationships.Remove(context.Background(), tutor, cards[domain.RelationshipReclaimPending], instant)
	newCard := invitedCard(40, tutor)
	newCard.Email, newCard.Notes = removed.Email, removed.Notes
	newCardErr := f.repos.Relationships.Create(context.Background(), newCard)

	if !errors.Is(outOfScope, repositories.ErrNotFound) || !errors.Is(repeated, repositories.ErrNotFound) {
		t.Fatalf("another tutor's removal %v, repeated removal %v", outOfScope, repeated)
	}
	if removeErr != nil || removed.Email == nil || *removed.Email != email || removed.Notes != notes || removed.DisplayName != "A student" {
		t.Fatalf("removed %+v, %v", removed, removeErr)
	}
	for state, err := range refusals {
		if !errors.Is(err, repositories.ErrNotFound) {
			t.Fatalf("removal of an %s card: %v", state, err)
		}
	}
	if pendingErr != nil {
		t.Fatalf("removal of a reclaim-pending link: %v", pendingErr)
	}
	var state, reason, keptNotes string
	var keptEmail *string
	err := f.pool.QueryRow(context.Background(), "SELECT state, discard_reason, notes, email FROM relationships WHERE id = $1",
		cards[domain.RelationshipProvisional]).Scan(&state, &reason, &keptNotes, &keptEmail)
	if err != nil || state != "discarded" || reason != "removed" || keptNotes != "" || keptEmail != nil {
		t.Fatalf("discarded row %s %s %q %v, %v", state, reason, keptNotes, keptEmail, err)
	}
	if newCardErr != nil {
		t.Fatalf("the new card with the removed card's email: %v", newCardErr)
	}
}

func cardsAt(f tutoringFixture, t *testing.T, tutor domain.UserID, count int, firstID byte) []repositories.NewRelationship {
	t.Helper()
	created := make([]repositories.NewRelationship, 0, count)
	for index := range count {
		card := invitedCard(firstID, tutor)
		card.ID = domain.RelationshipID{byte(index*37%251 + 1), firstID}
		card.CreatedAt = instant.Add(time.Duration(index/3) * time.Minute)
		f.card(t, card)
		created = append(created, card)
	}
	return created
}

func TestStudentsKeysetPagesWithoutGapsOrRepeats(t *testing.T) {
	t.Parallel()
	f := newTutoringFixture(t)
	tutor, other := f.tutor(t, 1), f.tutor(t, 2)
	listed := cardsAt(f, t, tutor, 23, 1)
	for index, state := range []domain.RelationshipState{domain.RelationshipActive, domain.RelationshipProvisional, domain.RelationshipEnded} {
		card := linkedCard(byte(60+index), tutor, f.student(t, byte(30+index)), state)
		card.CreatedAt = instant.Add(5 * time.Minute)
		f.card(t, card)
		listed = append(listed, card)
	}
	withdrawn := invitedCard(70, tutor)
	withdrawn.State, withdrawn.DiscardedAt = domain.RelationshipDiscarded, &instant
	reason := domain.DiscardWithdrawn
	withdrawn.DiscardReason = &reason
	f.card(t, withdrawn)
	cardsAt(f, t, other, 4, 2)
	slices.SortFunc(listed, func(left, right repositories.NewRelationship) int {
		if order := right.CreatedAt.Compare(left.CreatedAt); order != 0 {
			return order
		}
		return bytes.Compare(right.ID[:], left.ID[:])
	})
	every := []domain.RelationshipState{
		domain.RelationshipInvited, domain.RelationshipProvisional, domain.RelationshipReclaimPending, domain.RelationshipActive, domain.RelationshipEnded,
	}

	var pages [][]domain.RelationshipID
	var seen []domain.RelationshipID
	query := repositories.StudentListQuery{Tutor: tutor, States: every, Limit: 5}
	for len(pages) <= len(listed) {
		page, err := f.repos.Relationships.ListForTutor(context.Background(), query)
		if err != nil {
			t.Fatal(err)
		}
		if len(page) == 0 {
			break
		}
		ids := make([]domain.RelationshipID, 0, len(page))
		for _, card := range page {
			ids = append(ids, card.ID)
		}
		pages, seen = append(pages, ids), append(seen, ids...)
		last := page[len(page)-1]
		query.After = &repositories.RelationshipCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	active, activeErr := f.repos.Relationships.ListForTutor(context.Background(), repositories.StudentListQuery{
		Tutor: tutor, States: []domain.RelationshipState{domain.RelationshipProvisional, domain.RelationshipReclaimPending, domain.RelationshipActive}, Limit: 30,
	})
	_, refused := f.repos.Relationships.ListForTutor(context.Background(), repositories.StudentListQuery{
		Tutor: tutor, States: []domain.RelationshipState{domain.RelationshipDiscarded}, Limit: 30,
	})

	want := make([]domain.RelationshipID, 0, len(listed))
	for _, card := range listed {
		want = append(want, card.ID)
	}
	if !slices.Equal(seen, want) || len(pages) != 6 {
		t.Fatalf("pages %v, want %v in pages of five", pages, want)
	}
	if activeErr != nil || len(active) != 2 {
		t.Fatalf("active filter: %d rows, %v", len(active), activeErr)
	}
	if !errors.Is(refused, repositories.ErrDiscardedStateListed) {
		t.Fatalf("listing discarded cards: %v", refused)
	}
}

func TestStudentListSearchesLiterallyAndFiltersByGroup(t *testing.T) {
	t.Parallel()
	f := newTutoringFixture(t)
	tutor := f.tutor(t, 1)
	names := map[byte]string{10: "100% Anna", 11: "Boris_K", 12: `C\D`, 13: "Dmitry", 14: "Anna Petrova"}
	for id, name := range names {
		card := invitedCard(id, tutor)
		card.DisplayName = name
		f.card(t, card)
	}
	email := "mail_box@example.org"
	withEmail := invitedCard(15, tutor)
	withEmail.Email = &email
	f.card(t, withEmail)
	group := domain.GroupID{50}
	_, err := f.pool.Exec(context.Background(),
		"INSERT INTO groups (id, tutor_id, name, colour, created_at, updated_at) VALUES ($1, $2, 'Evening', 'moss', $3, $3)",
		group, tutor, instant)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(context.Background(), "INSERT INTO group_members (group_id, relationship_id, tutor_id, added_at) VALUES ($1, $2, $3, $4)",
		group, domain.RelationshipID{13}, tutor, instant); err != nil {
		t.Fatal(err)
	}
	search := func(text *string, inGroup *domain.GroupID) []domain.RelationshipID {
		rows, err := f.repos.Relationships.ListForTutor(context.Background(), repositories.StudentListQuery{
			Tutor: tutor, States: []domain.RelationshipState{domain.RelationshipInvited}, Search: text, Group: inGroup, Limit: 30,
		})
		if err != nil {
			t.Fatal(err)
		}
		ids := make([]domain.RelationshipID, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.ID)
		}
		slices.SortFunc(ids, func(left, right domain.RelationshipID) int { return bytes.Compare(left[:], right[:]) })
		return ids
	}
	text := func(value string) *string { return &value }

	percent, underscore, backslash := search(text("%"), nil), search(text("_"), nil), search(text(`\`), nil)
	anna, byEmail, grouped := search(text("aNNa"), nil), search(text("BOX@EXAMPLE"), nil), search(nil, &group)

	cases := map[string]struct {
		got, want []domain.RelationshipID
	}{
		"percent":    {percent, []domain.RelationshipID{{10}}},
		"underscore": {underscore, []domain.RelationshipID{{11}, {15}}},
		"backslash":  {backslash, []domain.RelationshipID{{12}}},
		"case":       {anna, []domain.RelationshipID{{10}, {14}}},
		"email":      {byEmail, []domain.RelationshipID{{15}}},
		"group":      {grouped, []domain.RelationshipID{{13}}},
	}
	for name, testCase := range cases {
		if !slices.Equal(testCase.got, testCase.want) {
			t.Errorf("%s: got %v, want %v", name, testCase.got, testCase.want)
		}
	}
}

func TestRelationshipStateChecksRefuseInconsistentRows(t *testing.T) {
	t.Parallel()
	f := newTutoringFixture(t)
	tutor := f.tutor(t, 1)
	student := f.student(t, 2)
	removed, purged, withdrawn := domain.DiscardRemoved, domain.DiscardStudentPurged, domain.DiscardWithdrawn
	byTutor := domain.EndedByTutor
	unknownColour := domain.Colour("violet")
	change := func(base repositories.NewRelationship, edit func(*repositories.NewRelationship)) repositories.NewRelationship {
		edit(&base)
		return base
	}
	invited := invitedCard(10, tutor)
	active := linkedCard(10, tutor, student, domain.RelationshipActive)
	ended := linkedCard(10, tutor, student, domain.RelationshipEnded)
	refused := map[string]struct {
		row        repositories.NewRelationship
		constraint string
	}{
		"invited with a student":   {change(invited, func(r *repositories.NewRelationship) { r.StudentID = &student }), "relationships_invited_unlinked"},
		"invited with a colour":    {change(active, func(r *repositories.NewRelationship) { r.State = domain.RelationshipInvited; r.StudentID = nil }), "relationships_invited_unlinked"},
		"active without a student": {change(active, func(r *repositories.NewRelationship) { r.StudentID = nil }), "relationships_linked_states_linked"},
		"provisional without a colour": {change(active, func(r *repositories.NewRelationship) {
			r.State, r.Colour = domain.RelationshipProvisional, nil
		}), "relationships_linked_states_linked"},
		"reclaim pending without a link time": {change(active, func(r *repositories.NewRelationship) {
			r.State, r.LinkedAt = domain.RelationshipReclaimPending, nil
		}), "relationships_linked_states_linked"},
		"discarded without a reason": {change(invited, func(r *repositories.NewRelationship) {
			r.State, r.DiscardedAt = domain.RelationshipDiscarded, &instant
		}), "relationships_discard_reason_when_discarded"},
		"a reason without discarding": {change(invited, func(r *repositories.NewRelationship) { r.DiscardReason = &withdrawn }), "relationships_discard_reason_when_discarded"},
		"discarded without a time": {change(invited, func(r *repositories.NewRelationship) {
			r.State, r.DiscardReason = domain.RelationshipDiscarded, &withdrawn
		}), "relationships_discarded_at_when_discarded"},
		"ended without its fields":  {change(ended, func(r *repositories.NewRelationship) { r.EndedAt, r.EndedBy = nil, nil }), "relationships_ending_fields_when_ended"},
		"ended by without a time":   {change(ended, func(r *repositories.NewRelationship) { r.EndedAt = nil }), "relationships_ending_fields_together"},
		"active with ending fields": {change(ended, func(r *repositories.NewRelationship) { r.State = domain.RelationshipActive }), "relationships_ending_fields_when_ended"},
		"withdrawn with ending fields": {change(ended, func(r *repositories.NewRelationship) {
			r.State, r.DiscardReason, r.DiscardedAt = domain.RelationshipDiscarded, &withdrawn, &instant
		}), "relationships_ending_fields_when_ended"},
		"unknown state":        {change(invited, func(r *repositories.NewRelationship) { r.State = "paused" }), "relationships_state_check"},
		"unknown colour":       {change(active, func(r *repositories.NewRelationship) { r.Colour = &unknownColour }), "relationships_colour_check"},
		"empty display name":   {change(invited, func(r *repositories.NewRelationship) { r.DisplayName = "" }), "relationships_display_name_check"},
		"unknown ending party": {change(ended, func(r *repositories.NewRelationship) { by := domain.EndedBy("operator"); r.EndedBy = &by }), "relationships_ended_by_check"},
	}
	accepted := map[string]repositories.NewRelationship{
		"purged history keeps the ending": change(ended, func(r *repositories.NewRelationship) {
			r.ID, r.State, r.StudentID, r.DiscardReason, r.DiscardedAt = domain.RelationshipID{11}, domain.RelationshipDiscarded, nil, &purged, &instant
		}),
		"removed link keeps its student": change(active, func(r *repositories.NewRelationship) {
			r.ID, r.State, r.DiscardReason, r.DiscardedAt = domain.RelationshipID{12}, domain.RelationshipDiscarded, &removed, &instant
		}),
		"ended by the tutor": change(ended, func(r *repositories.NewRelationship) { r.ID, r.EndedBy = domain.RelationshipID{13}, &byTutor }),
	}

	for name, testCase := range refused {
		err := f.repos.Relationships.Create(context.Background(), testCase.row)

		if code, constraint := violation(err); code != checkViolation || constraint != testCase.constraint {
			t.Errorf("%s: got %s %s (%v), want %s", name, code, constraint, err, testCase.constraint)
		}
	}
	for name, row := range accepted {
		if err := f.repos.Relationships.Create(context.Background(), row); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestEveryCascadingForeignKeyIsIndexed(t *testing.T) {
	t.Parallel()
	f := newTutoringFixture(t)
	const statement = `
SELECT constraint_row.conrelid::regclass::text || '.' || constraint_row.conname,
    EXISTS (
        SELECT 1 FROM pg_index AS index_row
        WHERE index_row.indrelid = constraint_row.conrelid
            AND index_row.indpred IS NULL
            AND index_row.indisvalid
            AND index_row.indkey[0] = constraint_row.conkey[1]
    )
FROM pg_constraint AS constraint_row
JOIN pg_namespace AS namespace ON namespace.oid = constraint_row.connamespace
WHERE constraint_row.contype = 'f' AND constraint_row.confdeltype IN ('c', 'r', 'n', 'd') AND namespace.nspname = 'public'
ORDER BY 1`

	rows, err := f.pool.Query(context.Background(), statement)
	if err != nil {
		t.Fatal(err)
	}
	indexed := map[string]bool{}
	for rows.Next() {
		var name string
		var covered bool
		if err := rows.Scan(&name, &covered); err != nil {
			t.Fatal(err)
		}
		indexed[name] = covered
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	for _, expected := range []string{
		"relationships.relationships_tutor_id_fkey", "relationships.relationships_student_id_fkey",
		"invitations.invitations_relationship_id_fkey", "invitations.invitations_bound_student_id_fkey", "invitations.invitations_used_by_fkey",
		"join_requests.join_requests_tutor_id_fkey", "join_requests.join_requests_student_id_fkey",
		"blocked_requesters.blocked_requesters_tutor_id_fkey", "blocked_requesters.blocked_requesters_student_id_fkey",
		"groups.groups_tutor_id_fkey", "group_members.group_members_group_fkey", "group_members.group_members_relationship_fkey",
		"attachments.attachments_uploader_id_fkey", "sessions.sessions_user_id_fkey",
	} {
		if _, found := indexed[expected]; !found {
			t.Errorf("foreign key %s was not read from the catalogue", expected)
		}
	}
	for name, covered := range indexed {
		if !covered {
			t.Errorf("foreign key %s has no full index leading with its column", name)
		}
	}
}

func TestTutorCodeIsMintedAgainOnACollision(t *testing.T) {
	t.Parallel()
	f := newTutoringFixture(t)
	first, second, third := f.account(t, 1, domain.RoleTutor), f.account(t, 2, domain.RoleTutor), f.account(t, 3, domain.RoleTutor)
	profile := func(user domain.UserID) repositories.NewTutorProfile {
		return repositories.NewTutorProfile{UserID: user, FileQuotaBytes: 1, RecordingQuotaBytes: 1, CreatedAt: instant}
	}
	twos := bytes.Repeat([]byte{0}, domain.TutorCodeLength)
	biasedThenThrees := append([]byte{255, 248}, bytes.Repeat([]byte{1}, domain.TutorCodeLength)...)

	firstErr := f.repos.TutorProfiles.Create(context.Background(), profile(first), bytes.NewReader(twos))
	secondErr := f.repos.TutorProfiles.Create(context.Background(), profile(second), bytes.NewReader(append(slices.Clone(twos), biasedThenThrees...)))
	thirdErr := f.repos.TutorProfiles.Create(context.Background(), profile(third), bytes.NewReader(bytes.Repeat(twos, 5)))

	if firstErr != nil || secondErr != nil {
		t.Fatalf("first %v, second %v", firstErr, secondErr)
	}
	for user, want := range map[domain.UserID]domain.TutorCode{first: "22222222", second: "33333333"} {
		stored, err := f.repos.TutorProfiles.ByUser(context.Background(), user)
		if err != nil || stored.Code != want {
			t.Fatalf("code %s, %v, want %s", stored.Code, err, want)
		}
	}
	if !errors.Is(thirdErr, repositories.ErrTutorCodesExhausted) {
		t.Fatalf("five collisions: %v", thirdErr)
	}
}

func TestGroupMembershipCannotCrossTutors(t *testing.T) {
	t.Parallel()
	f := newTutoringFixture(t)
	tutor, other := f.tutor(t, 1), f.tutor(t, 2)
	ownCard := f.card(t, linkedCard(10, tutor, f.student(t, 30), domain.RelationshipActive))
	otherCard := f.card(t, linkedCard(11, other, f.student(t, 31), domain.RelationshipActive))
	group := domain.GroupID{50}
	_, err := f.pool.Exec(context.Background(),
		"INSERT INTO groups (id, tutor_id, name, colour, created_at, updated_at) VALUES ($1, $2, 'Evening', 'moss', $3, $3)", group, tutor, instant)
	if err != nil {
		t.Fatal(err)
	}
	const insertMember = "INSERT INTO group_members (group_id, relationship_id, tutor_id, added_at) VALUES ($1, $2, $3, $4)"

	_, asOwnTutor := f.pool.Exec(context.Background(), insertMember, group, otherCard, tutor, instant)
	_, asOtherTutor := f.pool.Exec(context.Background(), insertMember, group, otherCard, other, instant)
	_, ownMember := f.pool.Exec(context.Background(), insertMember, group, ownCard, tutor, instant)

	if code, constraint := violation(asOwnTutor); code != foreignKeyViolation || constraint != "group_members_relationship_fkey" {
		t.Errorf("another tutor's card under the group's tutor: %s %s %v", code, constraint, asOwnTutor)
	}
	if code, constraint := violation(asOtherTutor); code != foreignKeyViolation || constraint != "group_members_group_fkey" {
		t.Errorf("another tutor's card under its own tutor: %s %s %v", code, constraint, asOtherTutor)
	}
	if ownMember != nil {
		t.Errorf("the tutor's own card: %v", ownMember)
	}
}

func TestInvitationAndJoinRequestChecksRefuseInconsistentRows(t *testing.T) {
	t.Parallel()
	f := newTutoringFixture(t)
	tutor := f.tutor(t, 1)
	student := f.student(t, 2)
	card := f.card(t, invitedCard(10, tutor))
	withdrawn := domain.RevokeWithdrawn
	request := func(id byte, edit func(*repositories.NewJoinRequest)) repositories.NewJoinRequest {
		row := joinRequest(id, tutor, student, domain.JoinRequestPending)
		edit(&row)
		return row
	}
	const insertInvitation = `INSERT INTO invitations (id, relationship_id, token_hash, expires_at, used_at, used_by, revoked_at, revoke_reason, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $4)`
	const insertJoinRequest = `INSERT INTO join_requests (id, tutor_id, student_id, state, suppressed, owner_choice_pending, expires_at, decided_at, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $7)`
	type usage struct {
		usedAt, revokedAt *time.Time
		usedBy            *domain.UserID
		reason            *domain.RevokeReason
	}
	type decision struct {
		decidedAt *time.Time
	}
	invitationCases := map[string]struct {
		row        repositories.NewInvitation
		usage      usage
		constraint string
	}{
		"used and revoked":        {openInvitation(20, card), usage{usedAt: &instant, usedBy: &student, revokedAt: &instant, reason: &withdrawn}, "invitations_used_or_revoked"},
		"consumer without use":    {openInvitation(21, card), usage{usedBy: &student}, "invitations_used_by_only_when_used"},
		"revoked without reason":  {openInvitation(22, card), usage{revokedAt: &instant}, "invitations_revoke_reason_when_revoked"},
		"reason without revoking": {openInvitation(23, card), usage{reason: &withdrawn}, "invitations_revoke_reason_when_revoked"},
	}
	requestCases := map[string]struct {
		row        repositories.NewJoinRequest
		decision   decision
		constraint string
	}{
		"decided while pending":            {request(30, func(*repositories.NewJoinRequest) {}), decision{decidedAt: &instant}, "join_requests_decided_at_when_closed"},
		"declined without a decision time": {request(31, func(r *repositories.NewJoinRequest) { r.State = domain.JoinRequestDeclined }), decision{}, "join_requests_decided_at_when_closed"},
		"owner choice while pending":       {request(32, func(r *repositories.NewJoinRequest) { r.OwnerChoicePending = true }), decision{}, "join_requests_owner_choice_only_while_held"},
		"suppressed while held": {request(33, func(r *repositories.NewJoinRequest) {
			r.State, r.Suppressed = domain.JoinRequestHeld, domain.SuppressionBlocked
		}), decision{}, "join_requests_suppressed_only_while_pending"},
	}

	invitationErrors := map[string]error{}
	for name, testCase := range invitationCases {
		row, use := testCase.row, testCase.usage
		_, invitationErrors[name] = f.pool.Exec(context.Background(), insertInvitation,
			row.ID, row.RelationshipID, row.TokenHash, row.ExpiresAt, use.usedAt, use.usedBy, use.revokedAt, use.reason)
	}
	requestErrors := map[string]error{}
	for name, testCase := range requestCases {
		row := testCase.row
		_, requestErrors[name] = f.pool.Exec(context.Background(), insertJoinRequest,
			row.ID, row.TutorID, row.StudentID, row.State, row.Suppressed, row.OwnerChoicePending, row.ExpiresAt, testCase.decision.decidedAt)
	}

	for name, testCase := range invitationCases {
		if code, constraint := violation(invitationErrors[name]); code != checkViolation || constraint != testCase.constraint {
			t.Errorf("%s: got %s %s (%v), want %s", name, code, constraint, invitationErrors[name], testCase.constraint)
		}
	}
	for name, testCase := range requestCases {
		if code, constraint := violation(requestErrors[name]); code != checkViolation || constraint != testCase.constraint {
			t.Errorf("%s: got %s %s (%v), want %s", name, code, constraint, requestErrors[name], testCase.constraint)
		}
	}
}
