package tutoring

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/accounts"
)

const (
	reclaimExpireBatch  = 100
	reclaimExpireRounds = 10

	fieldKeepLinks    = "body.keep_links"
	fieldDropLinks    = "body.drop_links"
	fieldKeepRequests = "body.keep_requests"
	fieldDropRequests = "body.drop_requests"
)

var ErrReclaimItemNotFound = errors.New("the identifier names no item waiting for this account's choice")

var errReclaimTransition = errors.New("a waiting reclaim item cannot take this transition")

type ReclaimTutor struct {
	Name     string
	AvatarID *domain.AttachmentID
	Subjects []string
}

type ReclaimLink struct {
	RelationshipID domain.RelationshipID
	Tutor          ReclaimTutor
}

type ReclaimRequest struct {
	RequestID domain.JoinRequestID
	Tutor     ReclaimTutor
}

type ReclaimList struct {
	Links    []ReclaimLink
	Requests []ReclaimRequest
	ChooseBy *time.Time
}

type ReclaimChoice struct {
	KeepLinks    []string
	DropLinks    []string
	KeepRequests []string
	DropRequests []string
}

type decisionCause string

const (
	decidedByOwner   decisionCause = "owner"
	decidedByTimeout decisionCause = "timeout"
)

type releasedRequest struct {
	request    repositories.JoinRequest
	suppressed domain.Suppression
}

type reclaimDecision struct {
	kept     int
	dropped  int
	outcomes []domain.LinkOutcome
	removed  []repositories.Relationship
	released []releasedRequest
}

type waitingItems struct {
	links    []repositories.Relationship
	requests []repositories.JoinRequest
}

func (w waitingItems) empty() bool {
	return len(w.links) == 0 && len(w.requests) == 0
}

type reclaimPlan struct {
	keepLinks    []repositories.Relationship
	dropLinks    []repositories.Relationship
	keepRequests []repositories.JoinRequest
	dropRequests []repositories.JoinRequest
}

func (s *Service) reclaim(ctx context.Context, repos repositories.Repositories, user domain.UserID, now time.Time) (domain.LinkOutcome, accounts.Announce, error) {
	links, err := repos.Relationships.LockOfStudent(ctx, user)
	if err != nil {
		return "", nil, err
	}
	var moved []repositories.Relationship
	for _, link := range links {
		transition, allowed := domain.NextRelationshipState(link.State, domain.CauseReclaimed)
		if !allowed || transition.To != domain.RelationshipReclaimPending {
			continue
		}
		reclaimed, err := repos.Relationships.Reclaim(ctx, link.ID, now)
		if err != nil {
			return "", nil, err
		}
		if !reclaimed {
			return "", nil, fmt.Errorf("reclaim the locked relationship %s: %w", link.ID, repositories.ErrNotFound)
		}
		moved = append(moved, link)
	}
	held, err := repos.JoinRequests.MarkHeldForOwnerChoice(ctx, user)
	if err != nil {
		return "", nil, err
	}
	if len(moved) == 0 && held == 0 {
		return "", accounts.Silent, nil
	}
	if _, err := repos.Users.SetReclaimDeadline(ctx, user, now.Add(domain.ReclaimWindow), now); err != nil {
		return "", nil, err
	}
	return domain.LinkOutcomeChoiceRequired, func(ctx context.Context) {
		for _, link := range moved {
			attributes := append(cardAttributes(link.TutorID, link.ID), slog.String("user_id", user.String()))
			s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "relationship.reclaim_pending", attributes...)
		}
	}, nil
}

func (s *Service) ReclaimPending(ctx context.Context, repos repositories.Repositories, user domain.UserID) (bool, error) {
	return repos.Relationships.AwaitsReclaimChoice(ctx, user)
}

func (s *Service) ReclaimItems(ctx context.Context, principal accounts.Principal) (ReclaimList, error) {
	list := ReclaimList{Links: []ReclaimLink{}, Requests: []ReclaimRequest{}}
	err := s.deps.Store.WithReadTx(ctx, func(repos repositories.Repositories) error {
		account, err := repos.Users.ByID(ctx, principal.UserID)
		if errors.Is(err, repositories.ErrNotFound) {
			return accounts.ErrUnauthenticated
		}
		if err != nil {
			return err
		}
		links, err := repos.Relationships.ReclaimPendingOf(ctx, principal.UserID)
		if err != nil {
			return err
		}
		requests, err := repos.JoinRequests.OwnerChoiceOf(ctx, principal.UserID)
		if err != nil {
			return err
		}
		tutors := make([]domain.UserID, 0, len(links)+len(requests))
		for _, link := range links {
			tutors = append(tutors, link.TutorID)
		}
		for _, request := range requests {
			tutors = append(tutors, request.TutorID)
		}
		avatars, err := repos.Relationships.AvatarsOf(ctx, tutors)
		if err != nil {
			return err
		}
		tutor := func(id domain.UserID, name string, subjects []string) ReclaimTutor {
			shown := ReclaimTutor{Name: name, Subjects: subjects}
			if avatar, has := avatars[id]; has {
				shown.AvatarID = &avatar
			}
			return shown
		}
		for _, link := range links {
			list.Links = append(list.Links, ReclaimLink{RelationshipID: link.ID, Tutor: tutor(link.TutorID, link.Name, link.Subjects)})
		}
		for _, request := range requests {
			list.Requests = append(list.Requests, ReclaimRequest{RequestID: request.ID, Tutor: tutor(request.TutorID, request.Name, request.Subjects)})
		}
		if len(tutors) > 0 {
			list.ChooseBy = account.ReclaimUntil
		}
		return nil
	})
	return list, err
}

func (s *Service) DecideReclaim(ctx context.Context, principal accounts.Principal, choice ReclaimChoice) (*domain.LinkOutcome, error) {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return nil, err
	}
	var decision reclaimDecision
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		if err := accounts.LockActingAccount(ctx, repos, principal, now, principal.State); err != nil {
			return err
		}
		waiting, err := s.lockWaitingLinks(ctx, repos, principal.UserID)
		if err != nil || waiting.empty() {
			return err
		}
		plan, err := waiting.plan(choice)
		if err != nil {
			return err
		}
		decision, err = s.apply(ctx, repos, principal.UserID, plan, domain.CauseOwnerDropped, now)
		if err != nil {
			return err
		}
		_, err = repos.Users.ClearReclaimDeadline(ctx, principal.UserID, now)
		return err
	})
	if err != nil {
		return nil, err
	}
	if decision.kept+decision.dropped == 0 {
		return nil, nil
	}
	s.announceDecision(ctx, principal.UserID, decision, decidedByOwner)
	outcome, set := domain.CombineOutcomes(decision.outcomes...)
	if !set {
		return nil, nil
	}
	return &outcome, nil
}

func (s *Service) lockWaitingLinks(ctx context.Context, repos repositories.Repositories, student domain.UserID) (waitingItems, error) {
	links, err := repos.Relationships.ReclaimPendingOf(ctx, student)
	if err != nil {
		return waitingItems{}, err
	}
	requests, err := repos.JoinRequests.OwnerChoiceOf(ctx, student)
	if err != nil {
		return waitingItems{}, err
	}
	tutors := make([]domain.UserID, 0, len(links)+len(requests))
	for _, link := range links {
		tutors = append(tutors, link.TutorID)
	}
	for _, request := range requests {
		tutors = append(tutors, request.TutorID)
	}
	if len(tutors) == 0 {
		return waitingItems{}, nil
	}
	if err := repos.TutorProfiles.LockAll(ctx, tutors); err != nil {
		return waitingItems{}, err
	}
	var waiting waitingItems
	waiting.links, err = repos.Relationships.LockReclaimPendingOf(ctx, student)
	if err != nil {
		return waitingItems{}, err
	}
	waiting.requests, err = repos.JoinRequests.OwnerChoiceRowsOf(ctx, student)
	return waiting, err
}

type namedItems[ID comparable, Item any] struct {
	waiting map[ID]Item
	named   map[ID]bool
	order   []ID
}

func waitingByID[ID comparable, Item any](items []Item, id func(Item) ID) namedItems[ID, Item] {
	named := namedItems[ID, Item]{waiting: make(map[ID]Item, len(items)), named: map[ID]bool{}}
	for _, item := range items {
		named.waiting[id(item)] = item
		named.order = append(named.order, id(item))
	}
	return named
}

func (n namedItems[ID, Item]) known(texts []string, parse func(string) (ID, bool)) bool {
	for _, text := range texts {
		id, valid := parse(text)
		if _, waiting := n.waiting[id]; !valid || !waiting {
			return false
		}
	}
	return true
}

func (n namedItems[ID, Item]) take(texts []string, field string, parse func(string) (ID, bool), problems *[]FieldProblem) []Item {
	var taken []Item
	repeated := false
	for _, text := range texts {
		id, _ := parse(text)
		if n.named[id] {
			repeated = true
			continue
		}
		n.named[id] = true
		taken = append(taken, n.waiting[id])
	}
	if repeated {
		*problems = append(*problems, FieldProblem{Path: field, Code: domain.FieldInvalid})
	}
	return taken
}

func (n namedItems[ID, Item]) allNamed() bool {
	for _, id := range n.order {
		if !n.named[id] {
			return false
		}
	}
	return true
}

func (w waitingItems) plan(choice ReclaimChoice) (reclaimPlan, error) {
	links := waitingByID(w.links, func(link repositories.Relationship) domain.RelationshipID { return link.ID })
	requests := waitingByID(w.requests, func(request repositories.JoinRequest) domain.JoinRequestID { return request.ID })
	parseLink, parseRequest := domain.ParseUUID[domain.RelationshipID], domain.ParseUUID[domain.JoinRequestID]
	if !links.known(choice.KeepLinks, parseLink) || !links.known(choice.DropLinks, parseLink) ||
		!requests.known(choice.KeepRequests, parseRequest) || !requests.known(choice.DropRequests, parseRequest) {
		return reclaimPlan{}, ErrReclaimItemNotFound
	}
	var problems []FieldProblem
	plan := reclaimPlan{
		keepLinks:    links.take(choice.KeepLinks, fieldKeepLinks, parseLink, &problems),
		dropLinks:    links.take(choice.DropLinks, fieldDropLinks, parseLink, &problems),
		keepRequests: requests.take(choice.KeepRequests, fieldKeepRequests, parseRequest, &problems),
		dropRequests: requests.take(choice.DropRequests, fieldDropRequests, parseRequest, &problems),
	}
	if !links.allNamed() {
		problems = append(problems, FieldProblem{Path: fieldKeepLinks, Code: domain.FieldRequired})
	}
	if !requests.allNamed() {
		problems = append(problems, FieldProblem{Path: fieldKeepRequests, Code: domain.FieldRequired})
	}
	if len(problems) > 0 {
		return reclaimPlan{}, &ValidationError{Fields: problems}
	}
	return plan, nil
}

func (s *Service) apply(
	ctx context.Context, repos repositories.Repositories, student domain.UserID, plan reclaimPlan, dropCause domain.RelationshipCause, now time.Time,
) (reclaimDecision, error) {
	var decision reclaimDecision
	var keptTutors []domain.UserID
	for _, link := range plan.keepLinks {
		kept, err := s.keepLink(ctx, repos, link, now, &decision)
		if err != nil {
			return reclaimDecision{}, err
		}
		if kept {
			keptTutors = append(keptTutors, link.TutorID)
		}
	}
	for _, link := range plan.dropLinks {
		if err := s.dropLink(ctx, repos, link, dropCause, now, &decision); err != nil {
			return reclaimDecision{}, err
		}
	}
	locked, err := lockPlannedRequests(ctx, repos, student, plan)
	if err != nil {
		return reclaimDecision{}, err
	}
	for _, request := range plan.keepRequests {
		if current, waiting := locked[request.ID]; waiting {
			if err := s.keepRequest(ctx, repos, current, now, &decision); err != nil {
				return reclaimDecision{}, err
			}
		}
	}
	for _, request := range plan.dropRequests {
		if _, waiting := locked[request.ID]; !waiting {
			continue
		}
		expired, err := repos.JoinRequests.ExpireHeld(ctx, request.ID, now)
		if err != nil {
			return reclaimDecision{}, err
		}
		if expired {
			decision.dropped++
		}
	}
	for _, tutor := range keptTutors {
		if _, err := repos.JoinRequests.AcceptWaiting(ctx, tutor, student, now); err != nil {
			return reclaimDecision{}, err
		}
	}
	return decision, nil
}

func lockPlannedRequests(
	ctx context.Context, repos repositories.Repositories, student domain.UserID, plan reclaimPlan,
) (map[domain.JoinRequestID]repositories.JoinRequest, error) {
	if len(plan.keepRequests)+len(plan.dropRequests) == 0 {
		return nil, nil
	}
	rows, err := repos.JoinRequests.LockOwnerChoiceOf(ctx, student)
	if err != nil {
		return nil, err
	}
	locked := make(map[domain.JoinRequestID]repositories.JoinRequest, len(rows))
	for _, row := range rows {
		locked[row.ID] = row
	}
	return locked, nil
}

func (s *Service) keepLink(
	ctx context.Context, repos repositories.Repositories, link repositories.Relationship, now time.Time, decision *reclaimDecision,
) (bool, error) {
	tutor, err := repos.Users.ByID(ctx, link.TutorID)
	if err != nil {
		return false, err
	}
	if tutor.State != domain.AccountActive {
		return false, s.dropLink(ctx, repos, link, domain.CauseOwnerKeptInactiveTutor, now, decision)
	}
	transition, allowed := domain.NextRelationshipState(link.State, domain.CauseOwnerKept)
	if !allowed || transition.To != domain.RelationshipActive {
		return false, fmt.Errorf("keep relationship %s: %w", link.ID, errReclaimTransition)
	}
	activated, err := repos.Relationships.ActivateLinked(ctx, link, now)
	if err != nil {
		return false, err
	}
	if !activated {
		return false, fmt.Errorf("keep the locked relationship %s: %w", link.ID, repositories.ErrNotFound)
	}
	if _, err := repos.Invitations.RevokeOpen(ctx, link.TutorID, link.ID, domain.RevokeLinked, now); err != nil {
		return false, err
	}
	decision.kept++
	decision.outcomes = append(decision.outcomes, domain.LinkOutcomeLinked)
	return true, nil
}

func (s *Service) dropLink(
	ctx context.Context, repos repositories.Repositories, link repositories.Relationship, cause domain.RelationshipCause, now time.Time, decision *reclaimDecision,
) error {
	transition, allowed := domain.NextRelationshipState(link.State, cause)
	if !allowed || !transition.NewCard {
		return fmt.Errorf("drop relationship %s: %w", link.ID, errReclaimTransition)
	}
	if _, err := s.removeLink(ctx, repos, link, now); err != nil {
		return err
	}
	decision.dropped++
	decision.removed = append(decision.removed, link)
	return nil
}

func (s *Service) keepRequest(ctx context.Context, repos repositories.Repositories, request repositories.JoinRequest, now time.Time, decision *reclaimDecision) error {
	if request.State != domain.JoinRequestHeld {
		return nil
	}
	decision.kept++
	tutor, err := repos.Users.ByID(ctx, request.TutorID)
	if err != nil {
		return err
	}
	if tutor.State != domain.AccountActive || !request.ExpiresAt.After(now) {
		decision.outcomes = append(decision.outcomes, domain.LinkOutcomeCodeInvalid)
		_, err := repos.JoinRequests.ExpireHeld(ctx, request.ID, now)
		return err
	}
	suppressed, err := releaseVisibility(ctx, repos, request)
	if err != nil {
		return err
	}
	released, err := repos.JoinRequests.ReleaseHeld(ctx, request.ID, suppressed)
	if err != nil {
		return err
	}
	if !released {
		return fmt.Errorf("release the locked join request %s: %w", request.ID, repositories.ErrNotFound)
	}
	decision.outcomes = append(decision.outcomes, domain.LinkOutcomeRequestSent)
	decision.released = append(decision.released, releasedRequest{request: request, suppressed: suppressed})
	return nil
}

func releaseVisibility(ctx context.Context, repos repositories.Repositories, request repositories.JoinRequest) (domain.Suppression, error) {
	blocked, err := repos.JoinRequests.Blocked(ctx, request.TutorID, request.StudentID)
	if err != nil {
		return "", err
	}
	if blocked {
		return domain.SuppressionBlocked, nil
	}
	visible, err := repos.JoinRequests.VisiblePendingCount(ctx, request.TutorID)
	if err != nil {
		return "", err
	}
	if visible >= domain.VisiblePendingRequestCap {
		return domain.SuppressionLimit, nil
	}
	return domain.SuppressionNone, nil
}

func clearSettledDeadline(ctx context.Context, repos repositories.Repositories, student domain.UserID, now time.Time) error {
	waiting, err := repos.Relationships.AwaitsReclaimChoice(ctx, student)
	if err != nil || waiting {
		return err
	}
	_, err = repos.Users.ClearReclaimDeadline(ctx, student, now)
	return err
}

func (s *Service) announceDecision(ctx context.Context, student domain.UserID, decision reclaimDecision, cause decisionCause) {
	s.logReclaimDecided(ctx, student, decision, cause)
	for _, link := range decision.removed {
		s.logRemoved(ctx, link, removedByReclaim)
	}
	for _, released := range decision.released {
		visibility := "visible"
		if released.suppressed != domain.SuppressionNone {
			visibility = "suppressed"
		}
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "join_request.released",
			slog.String("tutor_id", released.request.TutorID.String()), slog.String("user_id", student.String()), slog.String("visibility", visibility))
	}
}

func (s *Service) logReclaimDecided(ctx context.Context, student domain.UserID, decision reclaimDecision, cause decisionCause) {
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "relationship.reclaim_decided", slog.String("user_id", student.String()),
		slog.Int("kept", decision.kept), slog.Int("dropped", decision.dropped), slog.String("cause", string(cause)))
}

func (s *Service) ExpireReclaims(ctx context.Context, now time.Time) (int64, error) {
	var expired int64
	var failures []error
	var after *repositories.ReclaimDue
	for range reclaimExpireRounds {
		due, err := s.deps.Store.Repositories().Users.ReclaimsDue(ctx, now, after, reclaimExpireBatch)
		if err != nil {
			return expired, errors.Join(append(failures, err)...)
		}
		for _, account := range due {
			cleared, decision, err := s.expireReclaim(ctx, account.ID, now)
			if err != nil {
				failures = append(failures, err)
				s.deps.Logger.LogAttrs(ctx, slog.LevelError, "reclaim.expire_failed", slog.String("user_id", account.ID.String()), slog.String("kind", core.ErrorKind(err)))
				continue
			}
			if !cleared {
				continue
			}
			expired++
			if decision.dropped > 0 {
				s.announceDecision(ctx, account.ID, decision, decidedByTimeout)
			}
		}
		if len(due) < reclaimExpireBatch {
			break
		}
		after = &due[len(due)-1]
	}
	return expired, errors.Join(failures...)
}

func (s *Service) expireReclaim(ctx context.Context, student domain.UserID, now time.Time) (bool, reclaimDecision, error) {
	cleared := false
	var decision reclaimDecision
	err := s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		locked, err := repos.Users.LockReclaim(ctx, student)
		if errors.Is(err, repositories.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if locked.ReclaimUntil == nil || locked.ReclaimUntil.After(now) {
			return nil
		}
		waiting, err := s.lockWaitingLinks(ctx, repos, student)
		if err != nil {
			return err
		}
		decision, err = s.apply(ctx, repos, student, reclaimPlan{dropLinks: waiting.links, dropRequests: waiting.requests}, domain.CauseReclaimExpired, now)
		if err != nil {
			return err
		}
		cleared = true
		_, err = repos.Users.ClearReclaimDeadline(ctx, student, now)
		return err
	})
	return cleared, decision, err
}

func (s *Service) OnAccountDeleted(
	ctx context.Context, repos repositories.Repositories, user domain.UserID, role domain.Role, now time.Time,
) (accounts.Announce, error) {
	if role == domain.RoleTutor {
		return s.discardForTutorDeletion(ctx, repos, user, now)
	}
	waiting, err := s.lockWaitingLinks(ctx, repos, user)
	if err != nil {
		return nil, err
	}
	decision, err := s.apply(ctx, repos, user, reclaimPlan{dropLinks: waiting.links, dropRequests: waiting.requests}, domain.CauseStudentDeleted, now)
	if err != nil {
		return nil, err
	}
	if _, err := repos.Users.ClearReclaimDeadline(ctx, user, now); err != nil {
		return nil, err
	}
	return func(ctx context.Context) {
		for _, link := range decision.removed {
			s.logRemoved(ctx, link, removedByStudent)
		}
	}, nil
}

func (s *Service) discardForTutorDeletion(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, now time.Time) (accounts.Announce, error) {
	if err := repos.TutorProfiles.Lock(ctx, tutor); err != nil {
		return nil, err
	}
	links, err := repos.Relationships.LockReclaimPendingOfTutor(ctx, tutor)
	if err != nil {
		return nil, err
	}
	discarded := make([]repositories.Relationship, 0, len(links))
	for _, link := range links {
		transition, allowed := domain.NextRelationshipState(link.State, domain.CauseTutorDeleted)
		if !allowed || transition.NewCard || transition.To != domain.RelationshipDiscarded {
			return nil, fmt.Errorf("discard relationship %s: %w", link.ID, errReclaimTransition)
		}
		applied, err := repos.Relationships.DiscardForTutorDeletion(ctx, link, now)
		if err != nil {
			return nil, err
		}
		if !applied {
			return nil, fmt.Errorf("discard the locked relationship %s: %w", link.ID, repositories.ErrNotFound)
		}
		if _, err := repos.Invitations.RevokeOpen(ctx, tutor, link.ID, domain.RevokeTutorDeleted, now); err != nil {
			return nil, err
		}
		discarded = append(discarded, link)
	}
	requests, err := repos.JoinRequests.LockOwnerChoiceOfTutor(ctx, tutor)
	if err != nil {
		return nil, err
	}
	for _, request := range requests {
		if _, err := repos.JoinRequests.ExpireHeld(ctx, request.ID, now); err != nil {
			return nil, err
		}
	}
	return func(ctx context.Context) {
		for _, link := range discarded {
			s.logRemoved(ctx, link, removedByTutorDeletion)
		}
	}, nil
}
