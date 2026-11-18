package tutoring

import (
	"context"
	"encoding/base64"
	"errors"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

const (
	displayNameMaxRunes    = 100
	expireInvitationBatch  = 500
	expireInvitationRounds = 10
	invitationPathPrefix   = "/invite/"
	cursorSeparator        = "_"

	fieldDisplayName = "body.display_name"
	fieldEmail       = "body.email"
	fieldNotes       = "body.notes"
	fieldCursor      = "query.cursor"
)

var (
	ErrCardNotFound          = errors.New("the card is not in the tutor's scope")
	ErrNotEditable           = errors.New("the card's email can be changed only while it is invited")
	ErrRelationshipNotActive = errors.New("the card is not in a state that allows this")
	ErrInvitationNotFound    = errors.New("the invitation is not usable")
)

type StudentExistsError struct {
	Existing domain.RelationshipID
}

func (e *StudentExistsError) Error() string {
	return "the tutor already has a card with this email"
}

type FieldProblem struct {
	Path string
	Code domain.FieldCode
}

type ValidationError struct {
	Fields []FieldProblem
}

func (e *ValidationError) Error() string {
	return "the request is invalid"
}

type CardAccount struct {
	Name           string
	Email          string
	AvatarID       *domain.AttachmentID
	EmailConfirmed bool
}

type CardGroup struct {
	ID     domain.GroupID
	Name   string
	Colour domain.Colour
}

type CardInvitation struct {
	URL       *string
	ExpiresAt time.Time
	Status    domain.InvitationStatus
}

type Card struct {
	ID          domain.RelationshipID
	State       domain.RelationshipState
	DisplayName string
	Email       *string
	Account     *CardAccount
	Groups      []CardGroup
	Notes       string
	Invitation  *CardInvitation
	LinkedAt    *time.Time
	EndedAt     *time.Time
	EndedBy     *domain.EndedBy
	CreatedAt   time.Time
	ChannelID   *domain.ChannelID
}

type CardQuery struct {
	Filter domain.CardFilter
	Group  *domain.GroupID
	Search string
	Cursor string
	Limit  int32
}

type CardPage struct {
	Items           []Card
	NextCursor      *string
	OpenAssignments map[domain.RelationshipID]int64
}

type NewCard struct {
	DisplayName string
	Email       *string
	CreateKey   *string
}

type EmailChange struct {
	Set   bool
	Value *string
}

type CardChange struct {
	DisplayName *string
	Email       EmailChange
	Notes       *string
}

type PublicTutor struct {
	Name     string
	AvatarID *domain.AttachmentID
	Subjects []string
}

func (s *Service) ListCards(ctx context.Context, tutor domain.UserID, query CardQuery) (CardPage, error) {
	states, known := query.Filter.States()
	if !known {
		return CardPage{}, &ValidationError{Fields: []FieldProblem{{Path: "query.state", Code: domain.FieldInvalid}}}
	}
	listing := repositories.StudentListQuery{Tutor: tutor, States: states, Group: query.Group, Limit: query.Limit + 1}
	if query.Search != "" {
		listing.Search = &query.Search
	}
	if query.Cursor != "" {
		after, valid := parseCursor(query.Cursor)
		if !valid {
			return CardPage{}, &ValidationError{Fields: []FieldProblem{{Path: fieldCursor, Code: domain.FieldInvalid}}}
		}
		listing.After = &after
	}
	page := CardPage{}
	err := s.deps.Store.WithReadTx(ctx, func(repos repositories.Repositories) error {
		if query.Group != nil {
			if _, err := repos.Groups.ForTutor(ctx, tutor, *query.Group); err != nil {
				return groupScope(err)
			}
		}
		rows, err := repos.Relationships.ListForTutor(ctx, listing)
		if err != nil {
			return err
		}
		if len(rows) > int(query.Limit) {
			rows = rows[:query.Limit]
			last := rows[len(rows)-1]
			next := formatCursor(repositories.RelationshipCursor{CreatedAt: last.CreatedAt, ID: last.ID})
			page.NextCursor = &next
		}
		page.Items, err = s.cardViews(ctx, repos, tutor, rows)
		if err != nil {
			return err
		}
		ids := make([]domain.RelationshipID, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.ID)
		}
		page.OpenAssignments, err = repos.Homework.OpenAssignments(ctx, tutor, ids)
		return err
	})
	return page, err
}

func formatCursor(cursor repositories.RelationshipCursor) string {
	return encodeCursor(cursor.CreatedAt, cursor.ID)
}

func parseCursor(text string) (repositories.RelationshipCursor, bool) {
	instant, id, valid := decodeCursor[domain.RelationshipID](text)
	return repositories.RelationshipCursor{CreatedAt: instant, ID: id}, valid
}

type cursorID interface {
	~[16]byte
	String() string
}

func encodeCursor[ID cursorID](createdAt time.Time, id ID) string {
	text := strconv.FormatInt(createdAt.UnixMicro(), 10) + cursorSeparator + id.String()
	return base64.RawURLEncoding.EncodeToString([]byte(text))
}

func decodeCursor[ID cursorID](text string) (time.Time, ID, bool) {
	var none ID
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(text)
	if err != nil {
		return time.Time{}, none, false
	}
	micros, idText, found := strings.Cut(string(decoded), cursorSeparator)
	if !found {
		return time.Time{}, none, false
	}
	instant, err := strconv.ParseInt(micros, 10, 64)
	if err != nil {
		return time.Time{}, none, false
	}
	id, valid := domain.ParseUUID[ID](idText)
	if !valid {
		return time.Time{}, none, false
	}
	return time.UnixMicro(instant).UTC(), id, true
}

func (s *Service) Card(ctx context.Context, tutor domain.UserID, id domain.RelationshipID) (Card, error) {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return Card{}, err
	}
	var card Card
	err = s.deps.Store.WithReadTx(ctx, func(repos repositories.Repositories) error {
		row, err := repos.Relationships.ForTutor(ctx, tutor, id)
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrCardNotFound
		}
		if err != nil {
			return err
		}
		card, err = s.fullCard(ctx, repos, tutor, row, now)
		return err
	})
	return card, err
}

func (s *Service) CreateCard(ctx context.Context, actor domain.TutorActor, request NewCard) (Card, error) {
	tutor := actor.UserID
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return Card{}, err
	}
	var card Card
	created := false
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		if err := s.lockActingTutor(ctx, repos, actor); err != nil {
			return err
		}
		if request.CreateKey != nil {
			replayed, found, err := s.replayedCard(ctx, repos, tutor, *request.CreateKey, now)
			if err != nil || found {
				card = replayed
				return err
			}
		}
		row, err := s.insertCard(ctx, repos, tutor, request, now)
		if err != nil {
			return err
		}
		if err := cardChanged(ctx, repos, tutor, row.ID); err != nil {
			return err
		}
		created = true
		card, err = s.fullCard(ctx, repos, tutor, row, now)
		return err
	})
	if err != nil {
		return Card{}, err
	}
	if created {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "card.created", cardAttributes(tutor, card.ID)...)
	}
	return card, nil
}

func (s *Service) replayedCard(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, key string, now time.Time) (Card, bool, error) {
	row, err := repos.Relationships.ByCreateKey(ctx, tutor, key)
	if errors.Is(err, repositories.ErrNotFound) {
		return Card{}, false, nil
	}
	if err != nil {
		return Card{}, false, err
	}
	if row.State == domain.RelationshipDiscarded {
		return Card{}, false, ErrCardNotFound
	}
	card, err := s.fullCard(ctx, repos, tutor, row, now)
	return card, true, err
}

func (s *Service) insertCard(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, request NewCard, now time.Time) (repositories.Relationship, error) {
	var email *string
	if request.Email != nil {
		normalized := domain.NormalizeEmail(*request.Email)
		email = &normalized
		if err := s.refuseDuplicateEmail(ctx, repos, tutor, normalized); err != nil {
			return repositories.Relationship{}, err
		}
	}
	name := strings.TrimSpace(request.DisplayName)
	if problems := cardProblems(name, email); len(problems) > 0 {
		return repositories.Relationship{}, &ValidationError{Fields: problems}
	}
	if err := s.reserve(ctx, repos, cardCreationLimit, tutor, now); err != nil {
		return repositories.Relationship{}, err
	}
	id, err := core.NewID[domain.RelationshipID](s.deps.Random)
	if err != nil {
		return repositories.Relationship{}, err
	}
	row := repositories.NewRelationship{
		ID: id, TutorID: tutor, State: domain.RelationshipInvited, DisplayName: name, Email: email, CreateKey: request.CreateKey, CreatedAt: now,
	}
	if err := repos.Relationships.Create(ctx, row); err != nil {
		return repositories.Relationship{}, err
	}
	created, err := repos.Relationships.ForTutor(ctx, tutor, id)
	if err != nil {
		return repositories.Relationship{}, err
	}
	return created, s.issueInvitation(ctx, repos, created, now)
}

func (s *Service) refuseDuplicateEmail(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, email string) error {
	existing, err := repos.Relationships.CardWithEmail(ctx, tutor, email)
	switch {
	case errors.Is(err, repositories.ErrNotFound):
		return nil
	case err != nil:
		return err
	}
	return &StudentExistsError{Existing: existing}
}

func cardProblems(name string, email *string) []FieldProblem {
	var problems []FieldProblem
	if !validDisplayName(name) {
		problems = append(problems, FieldProblem{Path: fieldDisplayName, Code: domain.FieldInvalid})
	}
	if email != nil && !domain.ValidEmail(*email) {
		problems = append(problems, FieldProblem{Path: fieldEmail, Code: domain.FieldInvalid})
	}
	return problems
}

func controlOtherThanLineFeedOrTab(character rune) bool {
	return unicode.IsControl(character) && character != '\n' && character != '\t'
}

func validDisplayName(name string) bool {
	return name != "" && utf8.RuneCountInString(name) <= displayNameMaxRunes && !strings.ContainsFunc(name, unicode.IsControl)
}

func (s *Service) UpdateCard(ctx context.Context, actor domain.TutorActor, id domain.RelationshipID, change CardChange) (Card, error) {
	tutor := actor.UserID
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return Card{}, err
	}
	var card Card
	var changed []string
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		if err := s.lockActingTutor(ctx, repos, actor); err != nil {
			return err
		}
		row, err := lockCard(ctx, repos, tutor, id)
		if err != nil {
			return err
		}
		updated, fields, err := s.applyChange(ctx, repos, row, change, now)
		if err != nil {
			return err
		}
		if len(fields) > 0 {
			if err := cardChanged(ctx, repos, tutor, id); err != nil {
				return err
			}
		}
		changed = fields
		card, err = s.fullCard(ctx, repos, tutor, updated, now)
		return err
	})
	if err != nil {
		return Card{}, err
	}
	if len(changed) > 0 {
		attributes := append(cardAttributes(tutor, id), slog.String("fields", strings.Join(changed, ",")))
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "card.updated", attributes...)
	}
	return card, nil
}

func lockCard(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, id domain.RelationshipID) (repositories.Relationship, error) {
	row, err := repos.Relationships.LockForTutor(ctx, tutor, id)
	if errors.Is(err, repositories.ErrNotFound) {
		return repositories.Relationship{}, ErrCardNotFound
	}
	return row, err
}

func (s *Service) applyChange(
	ctx context.Context, repos repositories.Repositories, row repositories.Relationship, change CardChange, now time.Time,
) (repositories.Relationship, []string, error) {
	if row.State == domain.RelationshipDiscarded {
		return row, nil, ErrCardNotFound
	}
	updated := row
	var changed []string
	emailChanged := false
	if change.Email.Set {
		var email *string
		if change.Email.Value != nil {
			normalized := domain.NormalizeEmail(*change.Email.Value)
			email = &normalized
		}
		emailChanged = !sameEmail(row.Email, email)
		updated.Email = email
	}
	if emailChanged {
		if row.State != domain.RelationshipInvited {
			return row, nil, ErrNotEditable
		}
		if updated.Email != nil {
			if err := s.refuseDuplicateEmail(ctx, repos, row.TutorID, *updated.Email); err != nil {
				return row, nil, err
			}
		}
		changed = append(changed, "email")
	}
	if change.DisplayName != nil {
		updated.DisplayName = strings.TrimSpace(*change.DisplayName)
		if updated.DisplayName != row.DisplayName {
			changed = append(changed, "display_name")
		}
	}
	if change.Notes != nil && *change.Notes != row.Notes {
		updated.Notes = *change.Notes
		changed = append(changed, "notes")
	}
	problems := cardProblems(updated.DisplayName, changedEmail(emailChanged, updated.Email))
	if change.Notes != nil && strings.ContainsFunc(*change.Notes, controlOtherThanLineFeedOrTab) {
		problems = append(problems, FieldProblem{Path: fieldNotes, Code: domain.FieldInvalid})
	}
	if len(problems) > 0 {
		return row, nil, &ValidationError{Fields: problems}
	}
	if len(changed) == 0 {
		return row, nil, nil
	}
	if emailChanged {
		if err := s.reserve(ctx, repos, cardEmailChangeLimit, row.TutorID, now); err != nil {
			return row, nil, err
		}
	}
	applied, err := repos.Relationships.UpdateText(ctx, repositories.CardText{
		DisplayName: updated.DisplayName, Email: updated.Email, Notes: updated.Notes, Now: now, ID: row.ID, TutorID: row.TutorID,
	})
	if err != nil {
		return row, nil, err
	}
	if !applied {
		return row, nil, ErrCardNotFound
	}
	if slices.Contains(changed, "display_name") {
		if err := s.channels().CardRenamed(ctx, repos, row.TutorID, row.ID); err != nil {
			return row, nil, err
		}
	}
	updated.UpdatedAt = now
	if emailChanged {
		if err := s.replaceInvitation(ctx, repos, updated, domain.RevokeEmailChanged, now); err != nil {
			return row, nil, err
		}
	}
	return updated, changed, nil
}

func sameEmail(current *string, next *string) bool {
	if current == nil || next == nil {
		return current == nil && next == nil
	}
	return domain.NormalizeEmail(*current) == *next
}

func changedEmail(changed bool, email *string) *string {
	if changed {
		return email
	}
	return nil
}

func (s *Service) WithdrawCard(ctx context.Context, actor domain.TutorActor, id domain.RelationshipID) error {
	tutor := actor.UserID
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return err
	}
	withdrawn := false
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		if err := s.lockActingTutor(ctx, repos, actor); err != nil {
			return err
		}
		row, err := lockCard(ctx, repos, tutor, id)
		if err != nil {
			return err
		}
		switch row.State {
		case domain.RelationshipDiscarded:
			if row.DiscardReason != nil && *row.DiscardReason == domain.DiscardWithdrawn {
				return nil
			}
			return ErrCardNotFound
		case domain.RelationshipProvisional, domain.RelationshipReclaimPending, domain.RelationshipActive, domain.RelationshipEnded:
			return ErrRelationshipNotActive
		case domain.RelationshipInvited:
		}
		applied, err := repos.Relationships.Withdraw(ctx, tutor, id, now)
		if err != nil {
			return err
		}
		if !applied {
			return ErrRelationshipNotActive
		}
		if _, err := repos.Invitations.RevokeOpen(ctx, tutor, id, domain.RevokeWithdrawn, now); err != nil {
			return err
		}
		withdrawn = true
		return cardChanged(ctx, repos, tutor, id)
	})
	if err != nil {
		return err
	}
	if withdrawn {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "card.withdrawn", cardAttributes(tutor, id)...)
	}
	return nil
}

func (s *Service) RegenerateInvitation(ctx context.Context, actor domain.TutorActor, id domain.RelationshipID) (CardInvitation, error) {
	tutor := actor.UserID
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return CardInvitation{}, err
	}
	var issued *CardInvitation
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		if err := s.lockActingTutor(ctx, repos, actor); err != nil {
			return err
		}
		row, err := lockCard(ctx, repos, tutor, id)
		if err != nil {
			return err
		}
		if row.State == domain.RelationshipDiscarded {
			return ErrCardNotFound
		}
		if !domain.CardTakesInvitations(row.State) {
			return ErrRelationshipNotActive
		}
		if err := s.reserve(ctx, repos, regenerationLimit, tutor, now); err != nil {
			return err
		}
		if err := s.replaceInvitation(ctx, repos, row, domain.RevokeRegenerated, now); err != nil {
			return err
		}
		if err := cardChanged(ctx, repos, tutor, id); err != nil {
			return err
		}
		issued, err = s.invitationView(ctx, repos, row, now)
		return err
	})
	if err != nil {
		return CardInvitation{}, err
	}
	if issued == nil || issued.URL == nil {
		return CardInvitation{}, errIssuedInvitationUnreadable
	}
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "invitation.regenerated", cardAttributes(tutor, id)...)
	return *issued, nil
}

var errIssuedInvitationUnreadable = errors.New("the invitation just issued cannot be shown")

func (s *Service) replaceInvitation(ctx context.Context, repos repositories.Repositories, card repositories.Relationship, reason domain.RevokeReason, now time.Time) error {
	if _, err := repos.Invitations.RevokeOpen(ctx, card.TutorID, card.ID, reason, now); err != nil {
		return err
	}
	return s.issueInvitation(ctx, repos, card, now)
}

func (s *Service) issueInvitation(ctx context.Context, repos repositories.Repositories, card repositories.Relationship, now time.Time) error {
	token, err := core.NewInvitationToken(s.deps.Random)
	if err != nil {
		return err
	}
	id, err := core.NewID[domain.InvitationID](s.deps.Random)
	if err != nil {
		return err
	}
	sealed, err := s.deps.Cipher.SealForInvitation(id, token.Raw)
	if err != nil {
		return err
	}
	var bound *domain.UserID
	if card.State == domain.RelationshipEnded {
		bound = card.StudentID
	}
	return repos.Invitations.Create(ctx, repositories.NewInvitation{
		ID: id, RelationshipID: card.ID, TokenHash: token.Hash, TokenEncrypted: sealed, BoundStudentID: bound,
		ExpiresAt: now.Add(domain.InvitationLifetime), CreatedAt: now,
	})
}

func (s *Service) fullCard(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, row repositories.Relationship, now time.Time) (Card, error) {
	cards, err := s.cardViews(ctx, repos, tutor, []repositories.Relationship{row})
	if err != nil {
		return Card{}, err
	}
	card := cards[0]
	card.Notes = row.Notes
	channels, err := repos.Channels.OfRelationshipsFor(ctx, tutor, []domain.RelationshipID{row.ID})
	if err != nil {
		return Card{}, err
	}
	if channel, has := channels[row.ID]; has {
		card.ChannelID = &channel
	}
	card.Invitation, err = s.invitationView(ctx, repos, row, now)
	return card, err
}

func (s *Service) cardViews(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, rows []repositories.Relationship) ([]Card, error) {
	ids := make([]domain.RelationshipID, 0, len(rows))
	var students []domain.UserID
	for _, row := range rows {
		ids = append(ids, row.ID)
		if row.StudentID != nil && domain.CardShowsAccount(row.State) {
			students = append(students, *row.StudentID)
		}
	}
	accounts, err := s.accounts(ctx, repos, students)
	if err != nil {
		return nil, err
	}
	groups, err := repos.Relationships.Groups(ctx, tutor, ids)
	if err != nil {
		return nil, err
	}
	groupsOf := make(map[domain.RelationshipID][]CardGroup, len(rows))
	for _, group := range groups {
		groupsOf[group.RelationshipID] = append(groupsOf[group.RelationshipID], CardGroup{ID: group.ID, Name: group.Name, Colour: group.Colour})
	}
	cards := make([]Card, 0, len(rows))
	for _, row := range rows {
		card := Card{
			ID: row.ID, State: row.State, DisplayName: row.DisplayName, Email: row.Email, Groups: groupsOf[row.ID],
			LinkedAt: row.LinkedAt, EndedAt: row.EndedAt, EndedBy: row.EndedBy, CreatedAt: row.CreatedAt,
		}
		if row.StudentID != nil && domain.CardShowsAccount(row.State) {
			if account, found := accounts[*row.StudentID]; found {
				card.Account = &account
			}
		}
		cards = append(cards, card)
	}
	return cards, nil
}

func (s *Service) accounts(ctx context.Context, repos repositories.Repositories, students []domain.UserID) (map[domain.UserID]CardAccount, error) {
	found := make(map[domain.UserID]CardAccount, len(students))
	if len(students) == 0 {
		return found, nil
	}
	rows, err := repos.Relationships.Accounts(ctx, students)
	if err != nil {
		return nil, err
	}
	avatars, err := repos.Relationships.AvatarsOf(ctx, students)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		account := CardAccount{Name: row.Name, Email: row.Email, EmailConfirmed: row.EmailVerifiedAt != nil}
		if avatar, has := avatars[row.ID]; has {
			account.AvatarID = &avatar
		}
		found[row.ID] = account
	}
	return found, nil
}

func (s *Service) invitationView(ctx context.Context, repos repositories.Repositories, card repositories.Relationship, now time.Time) (*CardInvitation, error) {
	if !domain.CardTakesInvitations(card.State) {
		return nil, nil
	}
	latest, err := repos.Invitations.LatestOf(ctx, card.ID)
	if errors.Is(err, repositories.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	open := latest.UsedAt == nil && latest.RevokedAt == nil
	switch {
	case open && latest.ExpiresAt.After(now):
		return &CardInvitation{URL: s.readableLink(ctx, card, latest), ExpiresAt: latest.ExpiresAt, Status: domain.InvitationOpen}, nil
	case open:
		return &CardInvitation{ExpiresAt: latest.ExpiresAt, Status: domain.InvitationExpired}, nil
	case latest.RevokeReason != nil && *latest.RevokeReason == domain.RevokeMismatchLimit:
		return &CardInvitation{ExpiresAt: latest.ExpiresAt, Status: domain.InvitationRevokedAfterMismatches}, nil
	}
	return nil, nil
}

func (s *Service) readableLink(ctx context.Context, card repositories.Relationship, invitation repositories.CardInvitation) *string {
	if invitation.TokenEncrypted == nil {
		return nil
	}
	raw, err := s.deps.Cipher.OpenForInvitation(invitation.ID, invitation.TokenEncrypted)
	if err != nil {
		s.unreadable(ctx, card, err)
		return nil
	}
	text, valid := core.InvitationTokenText(raw)
	if !valid {
		s.unreadable(ctx, card, core.ErrFieldCiphertextInvalid)
		return nil
	}
	link := s.deps.PublicURL + invitationPathPrefix + text
	return &link
}

func (s *Service) unreadable(ctx context.Context, card repositories.Relationship, err error) {
	attributes := append(cardAttributes(card.TutorID, card.ID), slog.String("kind", core.ErrorKind(err)))
	s.deps.Logger.LogAttrs(ctx, slog.LevelWarn, "invitation.unreadable", attributes...)
}

func (s *Service) InvitationPage(ctx context.Context, token string) (PublicTutor, error) {
	hash, valid := core.InvitationTokenHash(token)
	if !valid {
		return PublicTutor{}, ErrInvitationNotFound
	}
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return PublicTutor{}, err
	}
	repos := s.deps.Store.Repositories()
	tutor, err := repos.Invitations.UsableTutor(ctx, hash, now)
	if errors.Is(err, repositories.ErrNotFound) {
		return PublicTutor{}, ErrInvitationNotFound
	}
	if err != nil {
		return PublicTutor{}, err
	}
	avatars, err := repos.Relationships.AvatarsOf(ctx, []domain.UserID{tutor.ID})
	if err != nil {
		return PublicTutor{}, err
	}
	public := PublicTutor{Name: tutor.Name, Subjects: tutor.Subjects}
	if avatar, has := avatars[tutor.ID]; has {
		public.AvatarID = &avatar
	}
	return public, nil
}

func (s *Service) ExpireInvitations(ctx context.Context, now time.Time) (int64, error) {
	var cleared int64
	for range expireInvitationRounds {
		rows, err := s.deps.Store.Repositories().Invitations.ClearExpiredCiphertext(ctx, now, expireInvitationBatch)
		if err != nil {
			return cleared, err
		}
		cleared += rows
		if rows < expireInvitationBatch {
			break
		}
	}
	return cleared, nil
}

func cardAttributes(tutor domain.UserID, card domain.RelationshipID) []slog.Attr {
	return []slog.Attr{slog.String("tutor_id", tutor.String()), slog.String("relationship_id", card.String())}
}
