package tutoring

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/accounts"
)

const tutorPurgeBatchRows = 500

var errDeletionTransition = errors.New("a relationship cannot take the transition of an account's deletion")

type tutorDeletion struct {
	tutor        domain.UserID
	ended        []ending
	removed      []repositories.Relationship
	discarded    int
	revoked      int64
	expired      int64
	archived     []domain.GroupID
	emailsQueued int64
}

func (s *Service) OnAccountDeleted(
	ctx context.Context, repos repositories.Repositories, user domain.UserID, role domain.Role, now time.Time,
) (accounts.Announce, error) {
	if role == domain.RoleTutor {
		deletion, err := s.applyTutorDeletion(ctx, repos, user, now)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context) { s.announceTutorDeletion(ctx, deletion) }, nil
	}
	return s.applyStudentDeletion(ctx, repos, user, now)
}

func (s *Service) applyTutorDeletion(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, now time.Time) (tutorDeletion, error) {
	deletion := tutorDeletion{tutor: tutor}
	if err := repos.TutorProfiles.Lock(ctx, tutor); err != nil {
		return tutorDeletion{}, err
	}
	if err := repos.Relationships.LockOfTutor(ctx, tutor); err != nil {
		return tutorDeletion{}, err
	}
	discarded, err := repos.Relationships.DiscardForTutorDeletion(ctx, tutor, now)
	if err != nil {
		return tutorDeletion{}, err
	}
	for _, card := range discarded {
		transition, allowed := domain.NextRelationshipState(card.PreviousState, domain.CauseTutorDeleted)
		if !allowed || transition.To != domain.RelationshipDiscarded || transition.NewCard {
			return tutorDeletion{}, fmt.Errorf("discard relationship %s: %w", card.ID, errDeletionTransition)
		}
		deletion.discarded++
		if card.PreviousState != domain.RelationshipInvited {
			deletion.removed = append(deletion.removed, repositories.Relationship{ID: card.ID, TutorID: card.TutorID, StudentID: card.StudentID})
		}
	}
	ended, err := s.endForTutorDeletion(ctx, repos, tutor, now)
	if err != nil {
		return tutorDeletion{}, err
	}
	if deletion.revoked, err = repos.Invitations.RevokeAllOfTutor(ctx, tutor, now); err != nil {
		return tutorDeletion{}, err
	}
	if deletion.expired, err = repos.JoinRequests.ExpireAllOfTutor(ctx, tutor, now); err != nil {
		return tutorDeletion{}, err
	}
	memberships, err := repos.Relationships.DeleteMembershipsOfTutor(ctx, tutor)
	if err != nil {
		return tutorDeletion{}, err
	}
	removed := make(map[domain.RelationshipID]int64, len(memberships))
	for _, relationship := range memberships {
		removed[relationship]++
	}
	for _, link := range ended {
		card := repositories.Relationship{ID: link.ID, TutorID: link.TutorID, StudentID: link.StudentID}
		deletion.ended = append(deletion.ended, ending{link: card, by: domain.EndedByDeletion, memberships: removed[link.ID]})
	}
	if deletion.archived, err = repos.Groups.ArchiveAllOfTutor(ctx, tutor, now); err != nil {
		return tutorDeletion{}, err
	}
	if err := signalTutorDeletion(ctx, repos, deletion); err != nil {
		return tutorDeletion{}, err
	}
	if err := s.queueTutorDeletedEmails(ctx, repos, &deletion, now); err != nil {
		return tutorDeletion{}, err
	}
	return deletion, nil
}

func signalTutorDeletion(ctx context.Context, repos repositories.Repositories, deletion tutorDeletion) error {
	links := append([]repositories.Relationship{}, deletion.removed...)
	for _, ended := range deletion.ended {
		links = append(links, ended.link)
	}
	for _, link := range links {
		if link.StudentID == nil {
			continue
		}
		if err := repos.Signals.Entity(ctx, []domain.UserID{*link.StudentID}, domain.EntityRelationship, domain.EntityID(link.ID)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) endForTutorDeletion(
	ctx context.Context, repos repositories.Repositories, tutor domain.UserID, now time.Time,
) ([]repositories.EndedForTutorDeletion, error) {
	transition, allowed := domain.NextRelationshipState(domain.RelationshipActive, domain.CauseTutorDeleted)
	if !allowed || transition.To != domain.RelationshipEnded || transition.EndedBy != domain.EndedByDeletion {
		return nil, fmt.Errorf("end the relationships of tutor %s: %w", tutor, errDeletionTransition)
	}
	return repos.Relationships.EndForTutorDeletion(ctx, tutor, now)
}

func (s *Service) queueTutorDeletedEmails(ctx context.Context, repos repositories.Repositories, deletion *tutorDeletion, now time.Time) error {
	students := make([]domain.UserID, 0, len(deletion.ended))
	for _, ended := range deletion.ended {
		if ended.link.StudentID != nil {
			students = append(students, *ended.link.StudentID)
		}
	}
	if len(students) == 0 {
		return nil
	}
	confirmed, err := repos.Users.ConfirmedAmong(ctx, students)
	if err != nil {
		return err
	}
	if len(confirmed) == 0 {
		return nil
	}
	tutor := hex.EncodeToString(deletion.tutor[:])
	deletedAt := now.UnixMicro()
	rows := make([]repositories.QueuedOutboxRow, 0, len(confirmed))
	for _, student := range confirmed {
		recipient := hex.EncodeToString(student[:])
		payload, err := json.Marshal(domain.TutorDeletedPayload{Tutor: tutor, DeletedAtMicros: deletedAt, Recipient: recipient})
		if err != nil {
			return fmt.Errorf("encode the tutor deleted payload: %w", err)
		}
		id, err := core.NewID[domain.OutboxID](s.deps.Random)
		if err != nil {
			return err
		}
		rows = append(rows, repositories.QueuedOutboxRow{
			ID: id, Payload: payload, IdempotencyKey: fmt.Sprintf("%s:%s:%d:%s", domain.EventTutorDeleted, tutor, deletedAt, recipient),
		})
	}
	queued, err := repos.Outbox.InsertAll(ctx, repositories.NewOutboxRows{
		Lane: domain.LaneEmail, Priority: domain.PriorityNormal, Kind: domain.EventTutorDeleted, NextAttemptAt: now, CreatedAt: now, Rows: rows,
	})
	if err != nil {
		return err
	}
	deletion.emailsQueued = queued
	return nil
}

func (s *Service) announceTutorDeletion(ctx context.Context, deletion tutorDeletion) {
	for _, ended := range deletion.ended {
		s.logEnded(ctx, ended)
	}
	for _, link := range deletion.removed {
		s.logRemoved(ctx, link, removedByTutorDeletion)
	}
	for _, group := range deletion.archived {
		attributes := append(groupAttributes(deletion.tutor, group), slog.Int("members", 0))
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "group.archived", attributes...)
	}
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "tutor.deletion_applied", slog.String("tutor_id", deletion.tutor.String()),
		slog.Int("ended", len(deletion.ended)), slog.Int("discarded", deletion.discarded), slog.Int64("invitations_revoked", deletion.revoked),
		slog.Int64("requests_expired", deletion.expired), slog.Int("groups_archived", len(deletion.archived)), slog.Int64("emails_queued", deletion.emailsQueued))
}

func (s *Service) applyStudentDeletion(ctx context.Context, repos repositories.Repositories, student domain.UserID, now time.Time) (accounts.Announce, error) {
	open, err := repos.JoinRequests.OpenTutorsOf(ctx, student)
	if err != nil {
		return nil, err
	}
	waiting, links, err := s.lockStudentForDeletion(ctx, repos, student, open)
	if err != nil {
		return nil, err
	}
	decision, err := s.apply(ctx, repos, student, reclaimPlan{dropLinks: waiting.links, dropRequests: waiting.requests}, domain.CauseStudentDeleted, now)
	if err != nil {
		return nil, err
	}
	expiredFor, err := repos.JoinRequests.ExpireOpenOf(ctx, student, now)
	if err != nil {
		return nil, err
	}
	if _, err := repos.JoinRequests.CloseUnblockedOf(ctx, student, now); err != nil {
		return nil, err
	}
	freed, err := s.unsuppressEach(ctx, repos, expiredFor, now)
	if err != nil {
		return nil, err
	}
	var ended []ending
	for _, link := range links {
		if link.State != domain.RelationshipActive {
			continue
		}
		done, err := s.endLink(ctx, repos, link, domain.CauseStudentDeleted, now)
		if err != nil {
			return nil, err
		}
		ended = append(ended, done)
	}
	if _, err := repos.Users.ClearReclaimDeadline(ctx, student, now); err != nil {
		return nil, err
	}
	return func(ctx context.Context) {
		for _, link := range decision.removed {
			s.logRemoved(ctx, link, removedByStudent)
		}
		for _, done := range ended {
			s.logEnded(ctx, done)
		}
		s.logUnsuppressed(ctx, freed)
	}, nil
}

func (s *Service) lockStudentForDeletion(
	ctx context.Context, repos repositories.Repositories, student domain.UserID, also []domain.UserID,
) (waitingItems, []repositories.Relationship, error) {
	pending, err := repos.Relationships.LinkedTutorsOfStudent(ctx, student)
	if err != nil {
		return waitingItems{}, nil, err
	}
	requests, err := repos.JoinRequests.OwnerChoiceOf(ctx, student)
	if err != nil {
		return waitingItems{}, nil, err
	}
	tutors := append(append([]domain.UserID{}, pending...), also...)
	for _, request := range requests {
		tutors = append(tutors, request.TutorID)
	}
	if len(tutors) > 0 {
		if err := repos.TutorProfiles.LockAll(ctx, tutors); err != nil {
			return waitingItems{}, nil, err
		}
	}
	links, err := repos.Relationships.LockOfStudent(ctx, student)
	if err != nil {
		return waitingItems{}, nil, err
	}
	var waiting waitingItems
	for _, link := range links {
		if transition, allowed := domain.NextRelationshipState(link.State, domain.CauseStudentDeleted); allowed && transition.NewCard {
			waiting.links = append(waiting.links, link)
		}
	}
	waiting.requests, err = repos.JoinRequests.OwnerChoiceRowsOf(ctx, student)
	return waiting, links, err
}

func (s *Service) OnAccountPurged(
	ctx context.Context, repos repositories.Repositories, user domain.UserID, role domain.Role, now time.Time,
) (accounts.Announce, error) {
	if role != domain.RoleStudent {
		return accounts.Silent, nil
	}
	tutors, err := repos.Relationships.LinkedTutorsOfStudent(ctx, user)
	if err != nil {
		return nil, err
	}
	if len(tutors) > 0 {
		if err := repos.TutorProfiles.LockAll(ctx, tutors); err != nil {
			return nil, err
		}
	}
	links, err := repos.Relationships.LockOfStudent(ctx, user)
	if err != nil {
		return nil, err
	}
	var removed []repositories.Relationship
	var ended []ending
	for _, link := range links {
		if link.State == domain.RelationshipActive {
			done, err := s.endLink(ctx, repos, link, domain.CauseStudentDeleted, now)
			if err != nil {
				return nil, err
			}
			ended = append(ended, done)
			continue
		}
		transition, allowed := domain.NextRelationshipState(link.State, domain.CauseStudentPurged)
		if !allowed || !transition.NewCard {
			if err := cardChanged(ctx, repos, link.TutorID, link.ID); err != nil {
				return nil, err
			}
			continue
		}
		if _, err := s.removeLink(ctx, repos, link, now); err != nil {
			return nil, err
		}
		removed = append(removed, link)
	}
	if err := repos.Relationships.ForgetPurgedStudent(ctx, user, now); err != nil {
		return nil, err
	}
	return func(ctx context.Context) {
		for _, done := range ended {
			s.logEnded(ctx, done)
		}
		for _, link := range removed {
			s.logRemoved(ctx, link, removedByCleanup)
		}
	}, nil
}

func (s *Service) lockActingTutor(ctx context.Context, repos repositories.Repositories, tutor domain.UserID) error {
	err := repos.TutorProfiles.Lock(ctx, tutor)
	if errors.Is(err, repositories.ErrNotFound) {
		return accounts.ErrUnauthenticated
	}
	if err != nil {
		return err
	}
	active, err := s.tutorActive(ctx, repos, tutor)
	if err != nil {
		return err
	}
	if !active {
		return accounts.ErrUnauthenticated
	}
	return nil
}

func (s *Service) tutorActive(ctx context.Context, repos repositories.Repositories, tutor domain.UserID) (bool, error) {
	account, err := repos.Users.ByID(ctx, tutor)
	if err != nil {
		return false, err
	}
	return account.State == domain.AccountActive, nil
}

func (s *Service) PurgeTutorBatch(ctx context.Context, repos repositories.Repositories, tutor domain.UserID) (int64, error) {
	steps := []func(context.Context, domain.UserID, int32) (int64, error){
		repos.Groups.DeleteMembersBatchOfTutor,
		repos.Groups.DeleteBatchOfTutor,
		repos.Invitations.DeleteBatchOfTutor,
		repos.JoinRequests.DeleteBatchOfTutor,
		repos.Blocked.DeleteBatchOfTutor,
		repos.Relationships.DeleteBatchOfTutor,
		repos.Notifications.DeleteBatchNaming,
	}
	for _, step := range steps {
		deleted, err := step(ctx, tutor, tutorPurgeBatchRows)
		if err != nil || deleted > 0 {
			return deleted, err
		}
	}
	return 0, nil
}
