package notifications_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/adapters/mail"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/notifications"
)

var now = time.Date(2030, time.March, 10, 9, 0, 0, 0, time.UTC)

const (
	greetingKind domain.OutboxKind = "test.greeting"
	staleKind    domain.OutboxKind = "test.stale"
	tokenKind    domain.OutboxKind = "test.token"
	silentKind   domain.OutboxKind = "test.silent"
)

type recordingMailer struct {
	sent []mail.Message
	err  error
}

func (m *recordingMailer) Send(_ context.Context, message mail.Message) error {
	if m.err != nil {
		return m.err
	}
	m.sent = append(m.sent, message)
	return nil
}

type fixedBuilder struct {
	composition notifications.Composition
}

func (b fixedBuilder) Build(context.Context, repositories.ClaimedOutboxRow, time.Time) (notifications.Composition, error) {
	return b.composition, nil
}

func newService(t *testing.T, mailer mail.Mailer) *notifications.Service {
	t.Helper()
	service, err := notifications.New(mailer, "https://ovc.example", map[domain.OutboxKind]notifications.Builder{
		greetingKind: fixedBuilder{composition: notifications.Composition{
			Email: notifications.Email{To: "tutor@example.org", Subject: "Hello", Text: "Hi", HTML: "<p>Hi</p>"}, Recipient: notifications.NoAccountRecipient(),
		}},
		silentKind: fixedBuilder{composition: notifications.Composition{Email: notifications.Email{To: "tutor@example.org", Subject: "Hello", Text: "Hi", HTML: "<p>Hi</p>"}}},
		staleKind:  fixedBuilder{composition: notifications.Composition{Obsolete: true}},
		tokenKind: fixedBuilder{composition: notifications.Composition{
			Email: notifications.Email{To: "tutor@example.org", Subject: "Hello", Text: "Hi", HTML: "<p>Hi</p>"}, Identity: "token-2",
			Recipient: notifications.NoAccountRecipient(),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func row(kind domain.OutboxKind, key string) repositories.ClaimedOutboxRow {
	return repositories.ClaimedOutboxRow{ID: domain.OutboxID{1}, Kind: kind, Payload: []byte(`{}`), IdempotencyKey: key, Attempts: 1}
}

func TestDeliverSendsWithAStableMessageIdentifier(t *testing.T) {
	mailer := &recordingMailer{}
	service := newService(t, mailer)

	first, firstErr := service.Deliver(context.Background(), row(greetingKind, "greeting:1"), now)
	second, secondErr := service.Deliver(context.Background(), row(greetingKind, "greeting:1"), now)
	other, otherErr := service.Deliver(context.Background(), row(greetingKind, "greeting:2"), now)

	if firstErr != nil || secondErr != nil || otherErr != nil || first != domain.DoneSent || second != domain.DoneSent || other != domain.DoneSent {
		t.Fatalf("results %s %s %s, errors %v %v %v", first, second, other, firstErr, secondErr, otherErr)
	}
	identifiers := []string{mailer.sent[0].MessageID, mailer.sent[1].MessageID, mailer.sent[2].MessageID}
	if identifiers[0] != identifiers[1] || identifiers[0] == identifiers[2] || !strings.HasSuffix(identifiers[0], "@ovc.example") || !mailer.sent[0].Date.Equal(now) {
		t.Fatalf("identifiers %v, date %v", identifiers, mailer.sent[0].Date)
	}
}

func TestDeliverMarksObsoleteEmailsWithoutSending(t *testing.T) {
	mailer := &recordingMailer{}
	service := newService(t, mailer)

	reason, err := service.Deliver(context.Background(), row(staleKind, "stale:1"), now)

	if err != nil || reason != domain.DoneObsolete || len(mailer.sent) != 0 {
		t.Fatalf("reason %s, error %v, sent %d", reason, err, len(mailer.sent))
	}
}

func TestDeliverFailuresThatCanNeverSucceedArePermanent(t *testing.T) {
	cases := map[string]struct {
		mailer *recordingMailer
		kind   domain.OutboxKind
	}{
		"unknown kind":         {mailer: &recordingMailer{}, kind: "test.unknown"},
		"malformed message":    {mailer: &recordingMailer{err: mail.ErrInvalidMessage}, kind: greetingKind},
		"unreported recipient": {mailer: &recordingMailer{}, kind: silentKind},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			service := newService(t, testCase.mailer)

			_, err := service.Deliver(context.Background(), row(testCase.kind, "key:1"), now)

			if !errors.Is(err, domain.ErrPermanentEffect) {
				t.Fatalf("expected a permanent failure, got %v", err)
			}
		})
	}
}

func TestDeliverTransientFailureIsRetryable(t *testing.T) {
	service := newService(t, &recordingMailer{err: errors.New("connection reset")})

	_, err := service.Deliver(context.Background(), row(greetingKind, "greeting:1"), now)

	if err == nil || errors.Is(err, domain.ErrPermanentEffect) {
		t.Fatalf("expected a retryable failure, got %v", err)
	}
}

func TestATokenBearingEmailGetsAnIdentifierOfItsOwnToken(t *testing.T) {
	mailer := &recordingMailer{}
	service := newService(t, mailer)

	_, plainErr := service.Deliver(context.Background(), row(greetingKind, "confirmation:1"), now)
	_, tokenErr := service.Deliver(context.Background(), row(tokenKind, "confirmation:1"), now)

	if plainErr != nil || tokenErr != nil || mailer.sent[0].MessageID == mailer.sent[1].MessageID {
		t.Fatalf("errors %v %v, identifiers %q %q", plainErr, tokenErr, mailer.sent[0].MessageID, mailer.sent[1].MessageID)
	}
}

func recipientService(t *testing.T, mailer mail.Mailer, kinds []domain.OutboxKind, state domain.AccountState) *notifications.Service {
	t.Helper()
	builders := map[domain.OutboxKind]notifications.Builder{}
	for _, kind := range kinds {
		builders[kind] = fixedBuilder{composition: notifications.Composition{
			Email: notifications.Email{To: "student@example.org", Subject: "Hello", Text: "Hi", HTML: "<p>Hi</p>"}, Recipient: notifications.AccountRecipient(state),
		}}
	}
	service, err := notifications.New(mailer, "https://ovc.example", builders)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestUnverifiedRecipientsGetOnlyAccountAndInvitationEmails(t *testing.T) {
	allowed := []domain.OutboxKind{domain.EventEmailConfirmation, domain.EventPasswordReset, domain.EventSignUpDuplicate, domain.EventInvitationCreated}
	other := []domain.OutboxKind{domain.EventRelationshipEnded, domain.EventTutorDeleted, greetingKind}
	kinds := append(append([]domain.OutboxKind{}, allowed...), other...)
	want := map[domain.AccountState]func(domain.OutboxKind) domain.OutboxDoneReason{
		domain.AccountUnverified: func(kind domain.OutboxKind) domain.OutboxDoneReason {
			if slices.Contains(allowed, kind) {
				return domain.DoneSent
			}
			return domain.DoneObsolete
		},
		domain.AccountActive:  func(domain.OutboxKind) domain.OutboxDoneReason { return domain.DoneSent },
		domain.AccountBlocked: func(domain.OutboxKind) domain.OutboxDoneReason { return domain.DoneObsolete },
		domain.AccountDeleted: func(domain.OutboxKind) domain.OutboxDoneReason { return domain.DoneObsolete },
	}

	for state, expected := range want {
		mailer := &recordingMailer{}
		service := recipientService(t, mailer, kinds, state)
		for _, kind := range kinds {
			reason, err := service.Deliver(context.Background(), row(kind, string(kind)+":1"), now)

			if err != nil || reason != expected(kind) {
				t.Errorf("%s for a %s recipient: %s, %v", kind, state, reason, err)
			}
		}
		sent := 0
		for _, kind := range kinds {
			if expected(kind) == domain.DoneSent {
				sent++
			}
		}
		if len(mailer.sent) != sent {
			t.Errorf("a %s recipient was sent %d emails instead of %d", state, len(mailer.sent), sent)
		}
	}
}

func TestCombineRefusesTwoBuildersForOneKind(t *testing.T) {
	first := map[domain.OutboxKind]notifications.Builder{greetingKind: fixedBuilder{}, tokenKind: fixedBuilder{}}
	second := map[domain.OutboxKind]notifications.Builder{staleKind: fixedBuilder{}}
	clashing := map[domain.OutboxKind]notifications.Builder{tokenKind: fixedBuilder{}}

	combined, combinedErr := notifications.Combine(first, second)
	_, clashErr := notifications.Combine(first, second, clashing)

	if combinedErr != nil || len(combined) != 3 {
		t.Fatalf("combining distinct kinds gave %d builders and %v", len(combined), combinedErr)
	}
	if clashErr == nil || !strings.Contains(clashErr.Error(), string(tokenKind)) {
		t.Fatalf("combining a repeated kind answered %v", clashErr)
	}
}
