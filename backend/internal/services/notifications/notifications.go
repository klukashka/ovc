package notifications

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"time"

	"ovc/backend/internal/adapters/mail"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

var (
	errUnknownKind         = errors.New("no email is defined for this kind")
	errRecipientUnreported = errors.New("the email builder reported no recipient")
	errDuplicateKind       = errors.New("two email builders share one kind")
)

type Email struct {
	To      string
	Subject string
	Text    string
	HTML    string
}

type Recipient struct {
	reported bool
	account  bool
	state    domain.AccountState
}

func AccountRecipient(state domain.AccountState) Recipient {
	return Recipient{reported: true, account: true, state: state}
}

func NoAccountRecipient() Recipient {
	return Recipient{reported: true}
}

func (r Recipient) reaches(kind domain.OutboxKind) bool {
	return !r.account || domain.EmailReachesRecipient(kind, r.state)
}

type Composition struct {
	Email     Email
	Identity  string
	Obsolete  bool
	Recipient Recipient
}

type Builder interface {
	Build(ctx context.Context, row repositories.ClaimedOutboxRow, now time.Time) (Composition, error)
}

type Service struct {
	mailer   mail.Mailer
	host     string
	builders map[domain.OutboxKind]Builder
}

func Combine(sets ...map[domain.OutboxKind]Builder) (map[domain.OutboxKind]Builder, error) {
	combined := map[domain.OutboxKind]Builder{}
	for _, set := range sets {
		for kind, builder := range set {
			if _, taken := combined[kind]; taken {
				return nil, fmt.Errorf("%w: %s", errDuplicateKind, kind)
			}
			combined[kind] = builder
		}
	}
	return combined, nil
}

func New(mailer mail.Mailer, publicURL string, builders map[domain.OutboxKind]Builder) (*Service, error) {
	parsed, err := url.Parse(publicURL)
	if err != nil || parsed.Hostname() == "" {
		return nil, fmt.Errorf("the public URL %q has no host for message identifiers", publicURL)
	}
	return &Service{mailer: mailer, host: parsed.Hostname(), builders: builders}, nil
}

func (s *Service) Deliver(ctx context.Context, row repositories.ClaimedOutboxRow, now time.Time) (domain.OutboxDoneReason, error) {
	builder, found := s.builders[row.Kind]
	if !found {
		return "", domain.PermanentEffect(fmt.Errorf("%w: %s", errUnknownKind, row.Kind))
	}
	composition, err := builder.Build(ctx, row, now)
	if err != nil {
		return "", err
	}
	if composition.Obsolete {
		return domain.DoneObsolete, nil
	}
	if !composition.Recipient.reported {
		return "", domain.PermanentEffect(fmt.Errorf("%w: %s", errRecipientUnreported, row.Kind))
	}
	if !composition.Recipient.reaches(row.Kind) {
		return domain.DoneObsolete, nil
	}
	err = s.mailer.Send(ctx, mail.Message{
		To:        composition.Email.To,
		Subject:   composition.Email.Subject,
		Text:      composition.Email.Text,
		HTML:      composition.Email.HTML,
		MessageID: s.messageID(row.IdempotencyKey, composition.Identity),
		Date:      now,
	})
	if errors.Is(err, mail.ErrInvalidMessage) {
		return "", domain.PermanentEffect(err)
	}
	if err != nil {
		return "", err
	}
	return domain.DoneSent, nil
}

func (s *Service) messageID(idempotencyKey string, identity string) string {
	sum := sha256.Sum256([]byte(idempotencyKey + "\x00" + identity))
	return hex.EncodeToString(sum[:16]) + "@" + s.host
}
