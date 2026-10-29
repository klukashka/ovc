package tutoring

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/accounts"
)

const (
	purposeCodeEntry domain.RateLimitPurpose = "join_request_code_hour"

	expireJoinRequestBatch  = 200
	expireJoinRequestRounds = 5

	TutorUnavailableRetry = time.Hour
)

var codeEntryLimit = rateLimit{purpose: purposeCodeEntry, window: time.Hour, limit: 10}

var (
	ErrAlreadyLinked               = errors.New("the student already has a link with this tutor")
	ErrTutorUnavailable            = errors.New("the tutor has the most visible pending requests allowed")
	ErrJoinRequestNotFound         = errors.New("the join request is not in the tutor's scope")
	ErrBlockedRequesterNotFound    = errors.New("the student is not blocked by this tutor")
	errJoinRequestLinkNotAvailable = errors.New("the student's relationship with the tutor cannot take a join request")
)

type SentJoinRequest struct {
	ID        domain.JoinRequestID
	State     domain.SentRequestState
	CreatedAt time.Time
	DecidedAt *time.Time
}

type SentJoinRequestPage struct {
	Items      []SentJoinRequest
	NextCursor *string
}

type RequestingStudent struct {
	Name     string
	Email    string
	AvatarID *domain.AttachmentID
}

type TutorJoinRequest struct {
	ID        domain.JoinRequestID
	Student   RequestingStudent
	CreatedAt time.Time
	ExpiresAt time.Time
}

type TutorJoinRequestPage struct {
	Items      []TutorJoinRequest
	NextCursor *string
}

type JoinRequestDecision struct {
	ID    domain.JoinRequestID
	State domain.JoinRequestState
}

type BlockedStudent struct {
	StudentID domain.UserID
	Name      *string
	AvatarID  *domain.AttachmentID
	BlockedAt time.Time
}

type BlockedStudentPage struct {
	Items      []BlockedStudent
	NextCursor *string
}

type PageQuery struct {
	Cursor string
	Limit  int32
}

type requestVisibility string

const (
	visibilityVisible    requestVisibility = "visible"
	visibilityHeld       requestVisibility = "held"
	visibilitySuppressed requestVisibility = "suppressed"
)

type unsuppressedRequest struct {
	tutor    domain.UserID
	request  repositories.UnsuppressedRequest
	notified *createdNotification
}

type codeEntry struct {
	request      SentJoinRequest
	created      *requestVisibility
	reopened     *requestVisibility
	kept         *releasedRequest
	unsuppressed []unsuppressedRequest
	notified     *createdNotification
}

func (s *Service) PrecheckCode(ctx context.Context, text string) (accounts.CodeCheck, error) {
	return s.activeCodeOwner(ctx, s.deps.Store.Repositories(), text)
}

func (s *Service) activeCodeOwner(ctx context.Context, repos repositories.Repositories, text string) (accounts.CodeCheck, error) {
	code, valid := domain.NormaliseTutorCode(text)
	if !valid {
		return accounts.CodeCheck{}, accounts.ErrTutorCodeInvalid
	}
	owner, err := repos.TutorProfiles.ByCode(ctx, code)
	if errors.Is(err, repositories.ErrNotFound) {
		return accounts.CodeCheck{}, accounts.ErrTutorCodeInvalid
	}
	if err != nil {
		return accounts.CodeCheck{}, err
	}
	if owner.State != domain.AccountActive {
		return accounts.CodeCheck{}, accounts.ErrTutorCodeInvalid
	}
	return accounts.CodeCheck{Tutor: owner.UserID, Code: code}, nil
}

type codeRefusal string

const (
	codeRefusedTutorInactive   codeRefusal = "tutor_inactive"
	codeRefusedCodeRegenerated codeRefusal = "code_regenerated"
)

type codeRefusedError struct {
	reason codeRefusal
}

func (e *codeRefusedError) Error() string {
	return "the tutor code stopped being valid under the profile lock: " + string(e.reason)
}

func (e *codeRefusedError) Unwrap() error {
	return accounts.ErrTutorCodeInvalid
}

func (s *Service) HoldRequestAtSignUp(
	ctx context.Context, repos repositories.Repositories, user domain.UserID, check accounts.CodeCheck, now time.Time,
) (accounts.Announce, error) {
	err := s.lockActiveTutor(ctx, repos, check)
	var refused *codeRefusedError
	if errors.As(err, &refused) {
		return func(ctx context.Context) {
			s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "join_request.not_held", slog.String("tutor_id", check.Tutor.String()),
				slog.String("user_id", user.String()), slog.String("reason", string(refused.reason)))
		}, nil
	}
	if err != nil {
		return nil, err
	}
	request, err := s.createRequest(ctx, repos, check.Tutor, user, domain.JoinRequestHeld, domain.SuppressionNone, now)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context) {
		s.logCreated(ctx, check.Tutor, user, request.ID, visibilityHeld)
	}, nil
}

func (s *Service) EnterCode(ctx context.Context, principal accounts.Principal, text string) (SentJoinRequest, error) {
	if principal.Role != domain.RoleStudent {
		return SentJoinRequest{}, ErrRoleMismatch
	}
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return SentJoinRequest{}, err
	}
	if err := s.reserveFor(ctx, s.deps.Store.Repositories(), codeEntryLimit, principal.UserID.String(), now); err != nil {
		var throttled *ThrottledError
		if errors.As(err, &throttled) {
			s.deps.Logger.LogAttrs(ctx, slog.LevelWarn, "join_request.throttled", slog.String("user_id", principal.UserID.String()))
		}
		return SentJoinRequest{}, err
	}
	check, err := s.activeCodeOwner(ctx, s.deps.Store.Repositories(), text)
	if err != nil {
		return SentJoinRequest{}, err
	}
	var entry codeEntry
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		if err := accounts.LockActingAccount(ctx, repos, principal, now, principal.State); err != nil {
			return err
		}
		if err := s.lockActiveTutor(ctx, repos, check); err != nil {
			return err
		}
		entry, err = s.enter(ctx, repos, principal, check.Tutor, now)
		if err != nil {
			return err
		}
		return requestChanged(ctx, repos, entry.request.ID, principal.UserID)
	})
	if err != nil {
		return SentJoinRequest{}, err
	}
	s.announceEntry(ctx, principal.UserID, check.Tutor, entry)
	return entry.request, nil
}

func (s *Service) lockActiveTutor(ctx context.Context, repos repositories.Repositories, check accounts.CodeCheck) error {
	current, err := repos.TutorProfiles.LockCode(ctx, check.Tutor)
	if errors.Is(err, repositories.ErrNotFound) {
		return &codeRefusedError{reason: codeRefusedTutorInactive}
	}
	if err != nil {
		return err
	}
	owner, err := repos.Users.ByID(ctx, check.Tutor)
	if err != nil {
		return err
	}
	if owner.State != domain.AccountActive {
		return &codeRefusedError{reason: codeRefusedTutorInactive}
	}
	if current != check.Code {
		return &codeRefusedError{reason: codeRefusedCodeRegenerated}
	}
	return nil
}

func (s *Service) enter(ctx context.Context, repos repositories.Repositories, principal accounts.Principal, tutor domain.UserID, now time.Time) (codeEntry, error) {
	student := principal.UserID
	link, err := repos.Relationships.OfTutorAndStudent(ctx, tutor, student)
	switch {
	case errors.Is(err, repositories.ErrNotFound):
	case err != nil:
		return codeEntry{}, err
	case link.State != domain.RelationshipEnded:
		return codeEntry{}, ErrAlreadyLinked
	}
	var entry codeEntry
	existing, err := repos.JoinRequests.LockOpen(ctx, tutor, student)
	switch {
	case errors.Is(err, repositories.ErrNotFound):
		closed, err := repos.JoinRequests.LockClosedByUnblock(ctx, tutor, student, now)
		if err == nil {
			return s.reopen(ctx, repos, closed, now)
		}
		if !errors.Is(err, repositories.ErrNotFound) {
			return codeEntry{}, err
		}
	case err != nil:
		return codeEntry{}, err
	case existing.ExpiresAt.After(now):
		return s.enterExisting(ctx, repos, existing, now)
	default:
		entry.unsuppressed, err = s.expireOverdue(ctx, repos, existing, now)
		if err != nil {
			return codeEntry{}, err
		}
	}
	state, suppressed, visibility := domain.JoinRequestHeld, domain.SuppressionNone, visibilityHeld
	if principal.State == domain.AccountActive {
		state, visibility = domain.JoinRequestPending, visibilityVisible
		suppressed, err = s.entryVisibility(ctx, repos, tutor, student, now)
		if err != nil {
			return codeEntry{}, err
		}
		if suppressed != domain.SuppressionNone {
			visibility = visibilitySuppressed
		}
	}
	request, err := s.createRequest(ctx, repos, tutor, student, state, suppressed, now)
	if err != nil {
		return codeEntry{}, err
	}
	entry.request, entry.created = sentView(request, now), &visibility
	if visibility == visibilityVisible {
		entry.notified, err = s.requestVisible(ctx, repos, request.ID, tutor, student, now)
	}
	return entry, err
}

func (s *Service) reopen(ctx context.Context, repos repositories.Repositories, closed repositories.JoinRequest, now time.Time) (codeEntry, error) {
	suppressed, err := releaseVisibility(ctx, repos, closed, now)
	if err != nil {
		return codeEntry{}, err
	}
	reopened, err := repos.JoinRequests.Reopen(ctx, closed.ID, suppressed, now)
	if err != nil {
		return codeEntry{}, err
	}
	if !reopened {
		return codeEntry{}, fmt.Errorf("reopen the locked join request %s: %w", closed.ID, repositories.ErrNotFound)
	}
	current := closed
	current.State, current.Suppressed, current.DecidedAt = domain.JoinRequestPending, suppressed, nil
	entry := codeEntry{request: sentView(current, now)}
	visibility := visibilityVisible
	if suppressed != domain.SuppressionNone {
		visibility = visibilitySuppressed
	} else {
		entry.notified, err = s.requestVisible(ctx, repos, closed.ID, closed.TutorID, closed.StudentID, now)
	}
	entry.reopened = &visibility
	return entry, err
}

func (s *Service) expireOverdue(ctx context.Context, repos repositories.Repositories, overdue repositories.JoinRequest, now time.Time) ([]unsuppressedRequest, error) {
	expired, err := repos.JoinRequests.ExpireDue(ctx, overdue.ID, now)
	if err != nil {
		return nil, err
	}
	if !expired {
		return nil, fmt.Errorf("expire the locked overdue join request %s: %w", overdue.ID, repositories.ErrNotFound)
	}
	if overdue.OwnerChoicePending {
		if err := clearSettledDeadline(ctx, repos, overdue.StudentID, now); err != nil {
			return nil, err
		}
	}
	return s.unsuppress(ctx, repos, overdue.TutorID, now)
}

func (s *Service) entryVisibility(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, student domain.UserID, now time.Time) (domain.Suppression, error) {
	visible, err := repos.JoinRequests.VisiblePendingCount(ctx, tutor, now)
	if err != nil {
		return "", err
	}
	if visible >= domain.VisiblePendingRequestCap {
		return "", ErrTutorUnavailable
	}
	blocked, err := repos.Blocked.Contains(ctx, tutor, student)
	if err != nil {
		return "", err
	}
	if blocked {
		return domain.SuppressionBlocked, nil
	}
	return domain.SuppressionNone, nil
}

func (s *Service) enterExisting(ctx context.Context, repos repositories.Repositories, existing repositories.JoinRequest, now time.Time) (codeEntry, error) {
	if existing.State != domain.JoinRequestHeld || !existing.OwnerChoicePending {
		return codeEntry{request: sentView(existing, now)}, nil
	}
	outcome, released, err := s.releaseHeld(ctx, repos, existing, now)
	if err != nil {
		return codeEntry{}, err
	}
	if err := clearSettledDeadline(ctx, repos, existing.StudentID, now); err != nil {
		return codeEntry{}, err
	}
	current := existing
	current.OwnerChoicePending = false
	current.State = domain.JoinRequestPending
	if outcome == domain.LinkOutcomeCodeInvalid {
		current.State = domain.JoinRequestExpired
		current.DecidedAt = &now
	}
	return codeEntry{request: sentView(current, now), kept: released}, nil
}

func (s *Service) createRequest(
	ctx context.Context, repos repositories.Repositories, tutor domain.UserID, student domain.UserID,
	state domain.JoinRequestState, suppressed domain.Suppression, now time.Time,
) (repositories.JoinRequest, error) {
	id, err := core.NewID[domain.JoinRequestID](s.deps.Random)
	if err != nil {
		return repositories.JoinRequest{}, err
	}
	request := repositories.NewJoinRequest{
		ID: id, TutorID: tutor, StudentID: student, State: state, Suppressed: suppressed,
		ExpiresAt: now.Add(domain.JoinRequestLifetime), CreatedAt: now,
	}
	if err := repos.JoinRequests.Create(ctx, request); err != nil {
		return repositories.JoinRequest{}, err
	}
	return repositories.JoinRequest{
		ID: id, TutorID: tutor, StudentID: student, State: state, Suppressed: suppressed, ExpiresAt: request.ExpiresAt, CreatedAt: now,
	}, nil
}

func sentView(request repositories.JoinRequest, now time.Time) SentJoinRequest {
	if request.State == domain.JoinRequestExpired && request.DecidedAt != nil && request.DecidedAt.After(now) {
		return SentJoinRequest{ID: request.ID, State: domain.SentRequestPending, CreatedAt: request.CreatedAt}
	}
	if request.State.Open() && !request.ExpiresAt.After(now) {
		expiredAt := request.ExpiresAt
		return SentJoinRequest{ID: request.ID, State: domain.SentRequestExpired, CreatedAt: request.CreatedAt, DecidedAt: &expiredAt}
	}
	return SentJoinRequest{ID: request.ID, State: request.State.Sent(), CreatedAt: request.CreatedAt, DecidedAt: request.DecidedAt}
}

func (s *Service) announceEntry(ctx context.Context, student domain.UserID, tutor domain.UserID, entry codeEntry) {
	if entry.created != nil {
		s.logCreated(ctx, tutor, student, entry.request.ID, *entry.created)
	}
	if entry.reopened != nil {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "join_request.reopened", slog.String("tutor_id", tutor.String()),
			slog.String("user_id", student.String()), slog.String("join_request_id", entry.request.ID.String()),
			slog.String("visibility", string(*entry.reopened)))
	}
	if entry.kept != nil {
		s.logReclaimDecided(ctx, student, reclaimDecision{kept: 1}, decidedByOwner)
		s.logReleased(ctx, student, *entry.kept)
	}
	s.logUnsuppressed(ctx, entry.unsuppressed)
	s.logNotified(ctx, entry.notified)
}

func (s *Service) logCreated(ctx context.Context, tutor domain.UserID, student domain.UserID, id domain.JoinRequestID, visibility requestVisibility) {
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "join_request.created", slog.String("tutor_id", tutor.String()),
		slog.String("user_id", student.String()), slog.String("join_request_id", id.String()), slog.String("visibility", string(visibility)))
}

func (s *Service) logReleased(ctx context.Context, student domain.UserID, released releasedRequest) {
	visibility := visibilityVisible
	if released.suppressed != domain.SuppressionNone {
		visibility = visibilitySuppressed
	}
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "join_request.released", slog.String("tutor_id", released.request.TutorID.String()),
		slog.String("user_id", student.String()), slog.String("visibility", string(visibility)))
	s.logNotified(ctx, released.notified)
}

func (s *Service) logUnsuppressed(ctx context.Context, requests []unsuppressedRequest) {
	for _, freed := range requests {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "join_request.unsuppressed", slog.String("tutor_id", freed.tutor.String()),
			slog.String("user_id", freed.request.StudentID.String()), slog.String("join_request_id", freed.request.ID.String()))
		s.logNotified(ctx, freed.notified)
	}
}

func (s *Service) releaseHeld(
	ctx context.Context, repos repositories.Repositories, request repositories.JoinRequest, now time.Time,
) (domain.LinkOutcome, *releasedRequest, error) {
	tutor, err := repos.Users.ByID(ctx, request.TutorID)
	if err != nil {
		return "", nil, err
	}
	if tutor.State != domain.AccountActive || !request.ExpiresAt.After(now) {
		_, err := repos.JoinRequests.ExpireHeld(ctx, request.ID, now)
		return domain.LinkOutcomeCodeInvalid, nil, err
	}
	suppressed, err := releaseVisibility(ctx, repos, request, now)
	if err != nil {
		return "", nil, err
	}
	released, err := repos.JoinRequests.ReleaseHeld(ctx, request.ID, suppressed)
	if err != nil {
		return "", nil, err
	}
	if !released {
		return "", nil, fmt.Errorf("release the locked join request %s: %w", request.ID, repositories.ErrNotFound)
	}
	kept := &releasedRequest{request: request, suppressed: suppressed}
	if suppressed == domain.SuppressionNone {
		kept.notified, err = s.requestVisible(ctx, repos, request.ID, request.TutorID, request.StudentID, now)
	}
	return domain.LinkOutcomeRequestSent, kept, err
}

func (s *Service) releaseAtConfirmation(ctx context.Context, repos repositories.Repositories, student domain.UserID, now time.Time) ([]domain.LinkOutcome, []releasedRequest, error) {
	held, err := repos.JoinRequests.LockHeldOf(ctx, student)
	if err != nil {
		return nil, nil, err
	}
	var outcomes []domain.LinkOutcome
	var released []releasedRequest
	for _, request := range held {
		outcome, kept, err := s.releaseHeld(ctx, repos, request, now)
		if err != nil {
			return nil, nil, err
		}
		outcomes = append(outcomes, outcome)
		if kept != nil {
			released = append(released, *kept)
		}
	}
	return outcomes, released, nil
}

func (s *Service) unsuppress(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, now time.Time) ([]unsuppressedRequest, error) {
	visible, err := repos.JoinRequests.VisiblePendingCount(ctx, tutor, now)
	if err != nil {
		return nil, err
	}
	free := int64(domain.VisiblePendingRequestCap) - visible
	if free <= 0 {
		return nil, nil
	}
	rows, err := repos.JoinRequests.Unsuppress(ctx, tutor, free, now)
	if err != nil {
		return nil, err
	}
	freed := make([]unsuppressedRequest, 0, len(rows))
	for _, row := range rows {
		notified, err := s.requestVisible(ctx, repos, row.ID, tutor, row.StudentID, now)
		if err != nil {
			return nil, err
		}
		freed = append(freed, unsuppressedRequest{tutor: tutor, request: row, notified: notified})
	}
	return freed, nil
}

func (s *Service) unsuppressEach(ctx context.Context, repos repositories.Repositories, tutors []domain.UserID, now time.Time) ([]unsuppressedRequest, error) {
	unique := slices.Clone(tutors)
	slices.SortFunc(unique, compareUsers)
	unique = slices.Compact(unique)
	var freed []unsuppressedRequest
	for _, tutor := range unique {
		rows, err := s.unsuppress(ctx, repos, tutor, now)
		if err != nil {
			return nil, err
		}
		freed = append(freed, rows...)
	}
	return freed, nil
}

func compareUsers(first domain.UserID, second domain.UserID) int {
	for index := range first {
		if first[index] != second[index] {
			return int(first[index]) - int(second[index])
		}
	}
	return 0
}

type tutorDecision struct {
	student      domain.UserID
	relationship *repositories.Relationship
	effect       acceptanceEffect
	unsuppressed []unsuppressedRequest
	notified     *createdNotification
	decided      bool
	blocked      bool
}

func (s *Service) AcceptRequest(ctx context.Context, tutor domain.UserID, id domain.JoinRequestID) (Card, error) {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return Card{}, err
	}
	var card Card
	var decision tutorDecision
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		request, err := s.lockRequestForDecision(ctx, repos, tutor, id, now)
		if err != nil {
			return err
		}
		decision.student = request.StudentID
		account, err := repos.Users.ByID(ctx, request.StudentID)
		if err != nil {
			return err
		}
		rows, err := repos.Relationships.LockForJoinRequest(ctx, tutor, request.StudentID, account.Email)
		if err != nil {
			return err
		}
		request, err = repos.JoinRequests.OfTutor(ctx, tutor, id)
		if err != nil {
			return err
		}
		var linked repositories.Relationship
		switch {
		case request.State == domain.JoinRequestAccepted:
			linked, err = repos.Relationships.OfTutorAndStudent(ctx, tutor, request.StudentID)
			if errors.Is(err, repositories.ErrNotFound) {
				return ErrNotEditable
			}
			if err != nil {
				return err
			}
		case visibleToTutor(request):
			linked, decision.effect, err = s.linkFromRequest(ctx, repos, tutor, account, rows, now)
			if err != nil {
				return err
			}
			if _, err := repos.JoinRequests.AcceptWaiting(ctx, tutor, request.StudentID, now); err != nil {
				return err
			}
			decision.decided = true
			decision.relationship = &linked
			decision.unsuppressed, err = s.unsuppress(ctx, repos, tutor, now)
			if err != nil {
				return err
			}
			if err := s.mirror(ctx, repos, domain.PathJoinAcceptance, directAudience(linked.ID), now); err != nil {
				return err
			}
			if err := relationshipChanged(ctx, repos, linked, true); err != nil {
				return err
			}
			if err := requestChanged(ctx, repos, id, tutor, request.StudentID); err != nil {
				return err
			}
			relationship := linked.ID
			decision.notified, err = s.notify(ctx, repos, notificationRow{
				event: domain.NotificationJoinRequestAccepted, recipient: request.StudentID, tutor: tutor, student: request.StudentID,
				subject: domain.EntityID(id), relationship: &relationship, key: domain.JoinRequestAcceptedKey(id, request.StudentID),
			}, now)
			if err != nil {
				return err
			}
		default:
			return decisionRefusal(request)
		}
		card, err = s.fullCard(ctx, repos, tutor, linked, now)
		return err
	})
	if err != nil {
		return Card{}, err
	}
	s.announceTutorDecision(ctx, tutor, id, decision, "accepted")
	return card, nil
}

func visibleToTutor(request repositories.JoinRequest) bool {
	return request.State == domain.JoinRequestPending && request.Suppressed == domain.SuppressionNone
}

func decisionRefusal(request repositories.JoinRequest) error {
	if request.State.Open() {
		return ErrJoinRequestNotFound
	}
	return ErrNotEditable
}

func (s *Service) lockRequestForDecision(
	ctx context.Context, repos repositories.Repositories, tutor domain.UserID, id domain.JoinRequestID, now time.Time,
) (repositories.JoinRequest, error) {
	seen, err := repos.JoinRequests.ForTutor(ctx, tutor, id, now)
	if errors.Is(err, repositories.ErrNotFound) {
		return repositories.JoinRequest{}, ErrJoinRequestNotFound
	}
	if err != nil {
		return repositories.JoinRequest{}, err
	}
	student, err := repos.Users.LockCredentials(ctx, seen.StudentID)
	if errors.Is(err, repositories.ErrNotFound) {
		return repositories.JoinRequest{}, ErrJoinRequestNotFound
	}
	if err != nil {
		return repositories.JoinRequest{}, err
	}
	if student.State == domain.AccountBlocked {
		return repositories.JoinRequest{}, ErrJoinRequestNotFound
	}
	if err := s.lockActingTutor(ctx, repos, tutor); err != nil {
		return repositories.JoinRequest{}, err
	}
	return seen, nil
}

func (s *Service) linkFromRequest(
	ctx context.Context, repos repositories.Repositories, tutor domain.UserID, account repositories.User, rows []repositories.Relationship, now time.Time,
) (repositories.Relationship, acceptanceEffect, error) {
	var invited *repositories.Relationship
	for index := range rows {
		row := rows[index]
		if row.StudentID != nil && *row.StudentID == account.ID {
			return s.reactivateFromRequest(ctx, repos, row, now)
		}
		if row.State == domain.RelationshipInvited && invited == nil {
			invited = &rows[index]
		}
	}
	taken, err := repos.Relationships.ColoursOfStudent(ctx, account.ID)
	if err != nil {
		return repositories.Relationship{}, "", err
	}
	colour := domain.ChooseColour(taken)
	if invited != nil {
		return s.linkInvitedFromRequest(ctx, repos, *invited, account.ID, colour, now)
	}
	id, err := core.NewID[domain.RelationshipID](s.deps.Random)
	if err != nil {
		return repositories.Relationship{}, "", err
	}
	err = repos.Relationships.Create(ctx, repositories.NewRelationship{
		ID: id, TutorID: tutor, StudentID: &account.ID, State: domain.RelationshipActive, DisplayName: account.Name,
		Colour: &colour, LinkedAt: &now, CreatedAt: now,
	})
	if err != nil {
		return repositories.Relationship{}, "", err
	}
	created, err := repos.Relationships.ForTutor(ctx, tutor, id)
	return created, effectCreated, err
}

func (s *Service) reactivateFromRequest(
	ctx context.Context, repos repositories.Repositories, row repositories.Relationship, now time.Time,
) (repositories.Relationship, acceptanceEffect, error) {
	if row.State == domain.RelationshipActive {
		return row, effectLinked, nil
	}
	transition, allowed := domain.NextRelationshipState(row.State, domain.CauseJoinRequestAccepted)
	if !allowed || transition.To != domain.RelationshipActive {
		return repositories.Relationship{}, "", fmt.Errorf("relationship %s in state %s: %w", row.ID, row.State, errJoinRequestLinkNotAvailable)
	}
	activated, err := repos.Relationships.ActivateLinked(ctx, row, now)
	if err != nil {
		return repositories.Relationship{}, "", err
	}
	if !activated {
		return repositories.Relationship{}, "", fmt.Errorf("reactivate the locked relationship %s: %w", row.ID, repositories.ErrNotFound)
	}
	if _, err := repos.Invitations.RevokeOpen(ctx, row.TutorID, row.ID, domain.RevokeLinked, now); err != nil {
		return repositories.Relationship{}, "", err
	}
	current, err := repos.Relationships.ForTutor(ctx, row.TutorID, row.ID)
	return current, effectReactivated, err
}

func (s *Service) linkInvitedFromRequest(
	ctx context.Context, repos repositories.Repositories, card repositories.Relationship, student domain.UserID, colour domain.Colour, now time.Time,
) (repositories.Relationship, acceptanceEffect, error) {
	transition, allowed := domain.NextRelationshipState(card.State, domain.CauseJoinRequestAccepted)
	if !allowed {
		return repositories.Relationship{}, "", fmt.Errorf("card %s in state %s: %w", card.ID, card.State, errJoinRequestLinkNotAvailable)
	}
	linked, err := repos.Relationships.LinkInvited(ctx, repositories.Link{ID: card.ID, State: transition.To, Student: student, Colour: colour, Now: now})
	if err != nil {
		return repositories.Relationship{}, "", err
	}
	if !linked {
		return repositories.Relationship{}, "", fmt.Errorf("link the locked card %s: %w", card.ID, repositories.ErrNotFound)
	}
	if _, err := repos.Invitations.RevokeOpen(ctx, card.TutorID, card.ID, domain.RevokeLinked, now); err != nil {
		return repositories.Relationship{}, "", err
	}
	current, err := repos.Relationships.ForTutor(ctx, card.TutorID, card.ID)
	return current, effectLinked, err
}

func (s *Service) DeclineRequest(ctx context.Context, tutor domain.UserID, id domain.JoinRequestID, block bool) (JoinRequestDecision, error) {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return JoinRequestDecision{}, err
	}
	var decision tutorDecision
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		request, err := s.lockRequestForDecision(ctx, repos, tutor, id, now)
		if err != nil {
			return err
		}
		decision.student = request.StudentID
		request, err = repos.JoinRequests.LockForTutor(ctx, tutor, id)
		if err != nil {
			return err
		}
		switch {
		case request.State == domain.JoinRequestDeclined:
		case visibleToTutor(request):
			declined, err := repos.JoinRequests.Decline(ctx, tutor, id, now)
			if err != nil {
				return err
			}
			if !declined {
				return fmt.Errorf("decline the locked join request %s: %w", id, repositories.ErrNotFound)
			}
			decision.decided = true
			decision.unsuppressed, err = s.unsuppress(ctx, repos, tutor, now)
			if err != nil {
				return err
			}
			if err := requestChanged(ctx, repos, id, tutor, request.StudentID); err != nil {
				return err
			}
			decision.notified, err = s.notify(ctx, repos, notificationRow{
				event: domain.NotificationJoinRequestDeclined, recipient: request.StudentID, tutor: tutor, student: request.StudentID,
				subject: domain.EntityID(id), key: domain.JoinRequestDeclinedKey(id, request.StudentID),
			}, now)
			if err != nil {
				return err
			}
		default:
			return decisionRefusal(request)
		}
		if block {
			decision.blocked, err = repos.Blocked.Block(ctx, tutor, request.StudentID, now)
		}
		return err
	})
	if err != nil {
		return JoinRequestDecision{}, err
	}
	outcome := "declined"
	if block {
		outcome = "blocked"
	}
	if decision.decided || decision.blocked {
		decision.decided = true
		s.announceTutorDecision(ctx, tutor, id, decision, outcome)
	}
	return JoinRequestDecision{ID: id, State: domain.JoinRequestDeclined}, nil
}

const effectCreated acceptanceEffect = "created"

func (s *Service) announceTutorDecision(ctx context.Context, tutor domain.UserID, id domain.JoinRequestID, decision tutorDecision, outcome string) {
	if !decision.decided {
		return
	}
	attributes := []slog.Attr{
		slog.String("tutor_id", tutor.String()), slog.String("user_id", decision.student.String()), slog.String("join_request_id", id.String()),
		slog.String("decision", outcome),
	}
	if decision.relationship != nil {
		attributes = append(attributes, slog.String("relationship_id", decision.relationship.ID.String()), slog.String("effect", string(decision.effect)))
	}
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "join_request.decided", attributes...)
	if decision.relationship != nil {
		link := append(cardAttributes(tutor, decision.relationship.ID), slog.String("user_id", decision.student.String()))
		switch decision.effect {
		case effectReactivated:
			s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "relationship.reactivated", link...)
		case effectCreated:
			s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "card.created", cardAttributes(tutor, decision.relationship.ID)...)
		case effectLinked, effectProvisional, effectMerged:
		}
	}
	s.logUnsuppressed(ctx, decision.unsuppressed)
	s.logNotified(ctx, decision.notified)
}

func (s *Service) Unblock(ctx context.Context, tutor domain.UserID, student domain.UserID) error {
	var expired int64
	err := s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		blocked, err := repos.Blocked.Contains(ctx, tutor, student)
		if err != nil {
			return err
		}
		if !blocked {
			return ErrBlockedRequesterNotFound
		}
		if _, err := repos.Users.LockCredentials(ctx, student); errors.Is(err, repositories.ErrNotFound) {
			return ErrBlockedRequesterNotFound
		} else if err != nil {
			return err
		}
		if err := s.lockActingTutor(ctx, repos, tutor); err != nil {
			return err
		}
		removed, err := repos.Blocked.Unblock(ctx, tutor, student)
		if err != nil {
			return err
		}
		if !removed {
			return ErrBlockedRequesterNotFound
		}
		expired, err = repos.JoinRequests.CloseBlocked(ctx, tutor, student)
		return err
	})
	if err != nil {
		return err
	}
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "blocked_requester.removed", slog.String("tutor_id", tutor.String()),
		slog.String("user_id", student.String()), slog.Int64("closed", expired))
	return nil
}

func (s *Service) TutorRequests(ctx context.Context, tutor domain.UserID, query PageQuery) (TutorJoinRequestPage, error) {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return TutorJoinRequestPage{}, err
	}
	after, err := parseRequestCursor(query.Cursor)
	if err != nil {
		return TutorJoinRequestPage{}, err
	}
	page := TutorJoinRequestPage{Items: []TutorJoinRequest{}}
	err = s.deps.Store.WithReadTx(ctx, func(repos repositories.Repositories) error {
		rows, err := repos.JoinRequests.VisibleOf(ctx, tutor, after, query.Limit+1, now)
		if err != nil {
			return err
		}
		if len(rows) > int(query.Limit) {
			rows = rows[:query.Limit]
			last := rows[len(rows)-1]
			next := encodeCursor(last.CreatedAt, last.ID)
			page.NextCursor = &next
		}
		students := make([]domain.UserID, 0, len(rows))
		for _, row := range rows {
			students = append(students, row.StudentID)
		}
		avatars, err := repos.Relationships.AvatarsOf(ctx, students)
		if err != nil {
			return err
		}
		for _, row := range rows {
			item := TutorJoinRequest{ID: row.ID, Student: RequestingStudent{Name: row.Name, Email: row.Email}, CreatedAt: row.CreatedAt, ExpiresAt: row.ExpiresAt}
			if avatar, has := avatars[row.StudentID]; has {
				item.Student.AvatarID = &avatar
			}
			page.Items = append(page.Items, item)
		}
		return nil
	})
	return page, err
}

func (s *Service) SentRequests(ctx context.Context, principal accounts.Principal, query PageQuery) (SentJoinRequestPage, error) {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return SentJoinRequestPage{}, err
	}
	after, err := parseRequestCursor(query.Cursor)
	if err != nil {
		return SentJoinRequestPage{}, err
	}
	rows, err := s.deps.Store.Repositories().JoinRequests.SentBy(ctx, principal.UserID, after, query.Limit+1)
	if err != nil {
		return SentJoinRequestPage{}, err
	}
	page := SentJoinRequestPage{Items: make([]SentJoinRequest, 0, len(rows))}
	if len(rows) > int(query.Limit) {
		rows = rows[:query.Limit]
		last := rows[len(rows)-1]
		next := encodeCursor(last.CreatedAt, last.ID)
		page.NextCursor = &next
	}
	for _, row := range rows {
		page.Items = append(page.Items, sentView(row, now))
	}
	return page, nil
}

func (s *Service) BlockedStudents(ctx context.Context, tutor domain.UserID, query PageQuery) (BlockedStudentPage, error) {
	var after *repositories.BlockedCursor
	if query.Cursor != "" {
		instant, id, valid := decodeCursor[domain.UserID](query.Cursor)
		if !valid {
			return BlockedStudentPage{}, invalidCursor()
		}
		after = &repositories.BlockedCursor{CreatedAt: instant, ID: id}
	}
	page := BlockedStudentPage{Items: []BlockedStudent{}}
	err := s.deps.Store.WithReadTx(ctx, func(repos repositories.Repositories) error {
		rows, err := repos.Blocked.Of(ctx, tutor, after, query.Limit+1)
		if err != nil {
			return err
		}
		if len(rows) > int(query.Limit) {
			rows = rows[:query.Limit]
			last := rows[len(rows)-1]
			next := encodeCursor(last.CreatedAt, last.StudentID)
			page.NextCursor = &next
		}
		students := make([]domain.UserID, 0, len(rows))
		for _, row := range rows {
			if row.State != domain.AccountDeleted {
				students = append(students, row.StudentID)
			}
		}
		avatars, err := repos.Relationships.AvatarsOf(ctx, students)
		if err != nil {
			return err
		}
		for _, row := range rows {
			page.Items = append(page.Items, blockedStudent(row, avatars))
		}
		return nil
	})
	return page, err
}

func blockedStudent(row repositories.BlockedRequester, avatars map[domain.UserID]domain.AttachmentID) BlockedStudent {
	item := BlockedStudent{StudentID: row.StudentID, BlockedAt: row.CreatedAt}
	if row.State == domain.AccountDeleted {
		return item
	}
	item.Name = &row.Name
	if avatar, has := avatars[row.StudentID]; has {
		item.AvatarID = &avatar
	}
	return item
}

func parseRequestCursor(text string) (*repositories.JoinRequestCursor, error) {
	if text == "" {
		return nil, nil
	}
	instant, id, valid := decodeCursor[domain.JoinRequestID](text)
	if !valid {
		return nil, invalidCursor()
	}
	return &repositories.JoinRequestCursor{CreatedAt: instant, ID: id}, nil
}

func invalidCursor() error {
	return &ValidationError{Fields: []FieldProblem{{Path: fieldCursor, Code: domain.FieldInvalid}}}
}

type expiryResume struct {
	mutex sync.Mutex
	after *repositories.DueCursor
}

func (r *expiryResume) take() *repositories.DueCursor {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.after
}

func (r *expiryResume) keep(after *repositories.DueCursor) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.after = after
}

func (s *Service) ExpireJoinRequests(ctx context.Context, now time.Time) (int64, error) {
	var expired int64
	var failures []error
	after := s.expiry.take()
	for round := range expireJoinRequestRounds {
		due, err := s.deps.Store.Repositories().JoinRequests.Due(ctx, now, after, expireJoinRequestBatch)
		if err != nil {
			return expired, errors.Join(append(failures, err)...)
		}
		for _, request := range due {
			applied, freed, err := s.expireJoinRequest(ctx, request, now)
			if err != nil {
				failures = append(failures, err)
				s.deps.Logger.LogAttrs(ctx, slog.LevelError, "join_requests.expire_failed",
					slog.String("join_request_id", request.ID.String()), slog.String("kind", core.ErrorKind(err)))
				continue
			}
			if applied {
				expired++
			}
			s.logUnsuppressed(ctx, freed)
		}
		if len(due) < expireJoinRequestBatch {
			s.expiry.keep(nil)
			break
		}
		last := due[len(due)-1]
		after = &repositories.DueCursor{ExpiresAt: last.ExpiresAt, ID: last.ID}
		if round == expireJoinRequestRounds-1 {
			s.expiry.keep(after)
		}
	}
	return expired, errors.Join(failures...)
}

func (s *Service) expireJoinRequest(ctx context.Context, request repositories.DueJoinRequest, now time.Time) (bool, []unsuppressedRequest, error) {
	applied := false
	var freed []unsuppressedRequest
	err := s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		if _, err := repos.Users.LockCredentials(ctx, request.StudentID); errors.Is(err, repositories.ErrNotFound) {
			return nil
		} else if err != nil {
			return err
		}
		if err := repos.TutorProfiles.Lock(ctx, request.TutorID); errors.Is(err, repositories.ErrNotFound) {
			return nil
		} else if err != nil {
			return err
		}
		expired, err := repos.JoinRequests.ExpireDue(ctx, request.ID, now)
		if err != nil || !expired {
			return err
		}
		applied = true
		freed, err = s.unsuppress(ctx, repos, request.TutorID, now)
		return err
	})
	return applied, freed, err
}
