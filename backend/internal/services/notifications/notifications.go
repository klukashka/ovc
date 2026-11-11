package notifications

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"ovc/backend/internal/adapters/mail"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

const releaseTimeout = 5 * time.Second

var (
	errUnknownKind         = errors.New("no email is defined for this kind")
	errRecipientUnreported = errors.New("the email builder reported no recipient")
	errDuplicateKind       = errors.New("two email builders share one kind")
)

type Email struct {
	To          string
	Subject     string
	Text        string
	HTML        string
	Unsubscribe string
}

type Optional struct {
	Recipient domain.UserID
	Group     domain.EmailGroup
	Capped    bool
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
	Drop      domain.EmailDropReason
	Recipient Recipient
	Optional  *Optional
}

type Builder interface {
	Build(ctx context.Context, row repositories.ClaimedOutboxRow, now time.Time) (Composition, error)
}

type Dispatch struct {
	Store  *repositories.Store
	Keys   core.Keys
	Logger *slog.Logger
}

type Service struct {
	mailer   mail.Mailer
	host     string
	builders map[domain.OutboxKind]Builder
	dispatch Dispatch
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

func New(mailer mail.Mailer, publicURL string, builders map[domain.OutboxKind]Builder, dispatch Dispatch) (*Service, error) {
	parsed, err := url.Parse(publicURL)
	if err != nil || parsed.Hostname() == "" {
		return nil, fmt.Errorf("the public URL %q has no host for message identifiers", publicURL)
	}
	return &Service{mailer: mailer, host: parsed.Hostname(), builders: builders, dispatch: dispatch}, nil
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
	if composition.Drop != "" {
		return s.dropped(ctx, row.Kind, composition, composition.Drop), nil
	}
	enabled, err := s.preferenceOn(ctx, composition.Optional)
	if err != nil {
		return "", err
	}
	if !enabled {
		return s.dropped(ctx, row.Kind, composition, domain.DropPreference), nil
	}
	slot, reserved, err := s.reserve(ctx, composition.Optional, now)
	if err != nil {
		return "", err
	}
	if !reserved {
		return s.dropped(ctx, row.Kind, composition, domain.DropDailyCap), nil
	}
	err = s.mailer.Send(ctx, mail.Message{
		To:          composition.Email.To,
		Subject:     composition.Email.Subject,
		Text:        composition.Email.Text,
		HTML:        composition.Email.HTML,
		MessageID:   s.messageID(row.IdempotencyKey, composition.Identity),
		Date:        now,
		Unsubscribe: composition.Email.Unsubscribe,
	})
	if err != nil {
		err = errors.Join(err, s.release(ctx, slot))
	}
	if errors.Is(err, mail.ErrInvalidMessage) {
		return "", domain.PermanentEffect(err)
	}
	if err != nil {
		return "", err
	}
	return domain.DoneSent, nil
}

func (s *Service) dropped(ctx context.Context, kind domain.OutboxKind, composition Composition, reason domain.EmailDropReason) domain.OutboxDoneReason {
	group := ""
	if composition.Optional != nil {
		group = string(composition.Optional.Group)
	}
	s.dispatch.Logger.LogAttrs(ctx, slog.LevelDebug, "email.dropped", slog.String("event", string(kind)), slog.String("group", group),
		slog.String("reason", string(reason)))
	return domain.DoneObsolete
}

func (s *Service) preferenceOn(ctx context.Context, optional *Optional) (bool, error) {
	if optional == nil {
		return true, nil
	}
	preferences, err := s.dispatch.Store.Repositories().Preferences.Of(ctx, optional.Recipient)
	if err != nil {
		return false, err
	}
	for _, preference := range preferences {
		if preference.EmailGroup == optional.Group {
			return preference.Enabled, nil
		}
	}
	return true, nil
}

func (s *Service) reserve(ctx context.Context, optional *Optional, now time.Time) (*repositories.RateLimitSlot, bool, error) {
	if optional == nil || !optional.Capped {
		return nil, true, nil
	}
	window, _ := domain.FixedWindow(now, domain.LessonEmailWindow)
	purpose, limit := domain.DailyEmailCap(optional.Group)
	slot := repositories.RateLimitSlot{
		Purpose: purpose, WindowStart: window,
		Subject: s.dispatch.Keys.ThrottleSubject(string(purpose), optional.Recipient.String()),
	}
	reserved, err := s.dispatch.Store.Repositories().RateLimits.ReserveBelow(ctx, repositories.DailySlot{
		Purpose: slot.Purpose, Subject: slot.Subject, WindowStart: slot.WindowStart, Below: limit,
	})
	if err != nil || !reserved {
		return nil, false, err
	}
	return &slot, true, nil
}

func (s *Service) release(ctx context.Context, slot *repositories.RateLimitSlot) error {
	if slot == nil {
		return nil
	}
	releasing, cancel := context.WithTimeout(context.WithoutCancel(ctx), releaseTimeout)
	defer cancel()
	_, err := s.dispatch.Store.Repositories().RateLimits.Release(releasing, *slot)
	return err
}

func (s *Service) messageID(idempotencyKey string, identity string) string {
	sum := sha256.Sum256([]byte(idempotencyKey + "\x00" + identity))
	return hex.EncodeToString(sum[:16]) + "@" + s.host
}
