package homework

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/library"
)

var errReplayed = errors.New("the create key names an existing issue")

const membershipAttempts = 2

type lockedAudience struct {
	relationship *domain.RelationshipID
	group        *domain.GroupID
	recipients   []repositories.NewSubmission
	refusal      error
}

func (s *Service) CreateIssue(ctx context.Context, actor domain.TutorActor, request NewIssue) (IssueItem, error) {
	tutor := actor.UserID
	if request.CreateKey != nil {
		replayed, found, err := s.replayCreate(ctx, s.deps.Store.Repositories(), tutor, *request.CreateKey)
		if err != nil || found {
			return replayed, err
		}
	}
	err := s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		now, err := s.lockTutor(ctx, repos, actor)
		if err != nil {
			return err
		}
		if request.CreateKey != nil {
			_, found, err := s.replayCreate(ctx, repos, tutor, *request.CreateKey)
			if err != nil {
				return err
			}
			if found {
				return errReplayed
			}
		}
		return s.reserve(ctx, repos, tutor, s.issueSlot(), now)
	})
	if errors.Is(err, errReplayed) {
		replayed, _, replayErr := s.replayCreate(ctx, s.deps.Store.Repositories(), tutor, *request.CreateKey)
		return replayed, replayErr
	}
	if err != nil {
		return IssueItem{}, err
	}
	var created IssueItem
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		now, err := s.lockTutor(ctx, repos, actor)
		if err != nil {
			return err
		}
		if request.CreateKey != nil {
			replayed, found, err := s.replayCreate(ctx, repos, tutor, *request.CreateKey)
			if err != nil || found {
				created = replayed
				return err
			}
		}
		created, err = s.issue(ctx, repos, tutor, request, now)
		return err
	})
	return created, err
}

func (s *Service) replayCreate(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, key string) (IssueItem, bool, error) {
	existing, err := repos.Homework.IssueByCreateKey(ctx, tutor, key)
	if errors.Is(err, repositories.ErrNotFound) {
		return IssueItem{}, false, nil
	}
	if err != nil {
		return IssueItem{}, false, err
	}
	if existing.ErasePendingSince != nil {
		return IssueItem{}, true, ErrNotFound
	}
	item, err := s.itemOf(ctx, repos, tutor, existing.ID)
	return item, true, err
}

func (s *Service) issue(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, request NewIssue, now time.Time) (IssueItem, error) {
	template, err := s.deps.Templates.TemplateForIssue(ctx, repos, tutor, request.Template)
	if errors.Is(err, library.ErrNotFound) {
		return IssueItem{}, ErrNotFound
	}
	if err != nil {
		return IssueItem{}, err
	}
	audience, err := s.lockAudience(ctx, repos, tutor, request.Audience)
	if err != nil {
		return IssueItem{}, err
	}
	if request.Lesson != nil {
		if err := s.lockLesson(ctx, repos, tutor, *request.Lesson, audience.relationship, audience.group); err != nil {
			return IssueItem{}, err
		}
	}
	if !template.Ready {
		return IssueItem{}, ErrNotEditable
	}
	if audience.refusal != nil {
		return IssueItem{}, audience.refusal
	}
	if len(audience.recipients) == 0 {
		return IssueItem{}, ErrAudienceNotActive
	}
	if request.Deadline != nil && !request.Deadline.After(now) {
		return IssueItem{}, invalid(fieldDeadline, domain.FieldInvalid)
	}
	content, err := s.snapshotOf(template)
	if err != nil {
		return IssueItem{}, err
	}
	referenced, err := repos.Library.TemplateReferences(ctx, template.ID)
	if err != nil {
		return IssueItem{}, err
	}
	if err := requireReferenced(content.attachments, referenced); err != nil {
		return IssueItem{}, err
	}
	if err := s.refuseBeyondBounds(ctx, repos, tutor, int64(len(content.encoded)), request.Lesson); err != nil {
		return IssueItem{}, err
	}
	id, err := core.NewID[domain.IssueID](s.deps.Random)
	if err != nil {
		return IssueItem{}, err
	}
	if err := repos.Homework.InsertIssue(ctx, repositories.NewIssue{
		ID: id, TutorID: tutor, TemplateID: template.ID, RelationshipID: audience.relationship, GroupID: audience.group, LessonID: request.Lesson,
		Title: content.title, Snapshot: content.encoded, SnapshotBytes: int64(len(content.encoded)), MaxPoints: content.maxPoints,
		HasManualQuestions: content.manual, DeadlineAt: request.Deadline, AnswerPolicy: request.Policy, CreateKey: request.CreateKey, Now: now,
	}); err != nil {
		return IssueItem{}, err
	}
	if err := repos.Homework.AddSnapshotReferences(ctx, id, content.attachments, now); err != nil {
		return IssueItem{}, err
	}
	if err := repos.Homework.AddSnapshotBytes(ctx, tutor, int64(len(content.encoded))); err != nil {
		return IssueItem{}, err
	}
	for index := range audience.recipients {
		audience.recipients[index].ID, err = core.NewID[domain.SubmissionID](s.deps.Random)
		if err != nil {
			return IssueItem{}, err
		}
	}
	if err := repos.Homework.InsertSubmissions(ctx, id, tutor, request.Deadline, audience.recipients, now); err != nil {
		return IssueItem{}, err
	}
	if err := s.announceCreated(ctx, repos, tutor, id, audience.recipients, now); err != nil {
		return IssueItem{}, err
	}
	item, err := s.itemOf(ctx, repos, tutor, id)
	if err != nil {
		return IssueItem{}, err
	}
	kind := domain.IssueAudienceRelationship
	if audience.group != nil {
		kind = domain.IssueAudienceGroup
	}
	repos.AfterCommit(ctx, func(ctx context.Context) {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "homework.issue.created", tutorAttribute(tutor), issueAttribute(id),
			slog.String("audience", string(kind)), slog.Int("submission_count", len(audience.recipients)),
			slog.Bool("has_manual_questions", content.manual))
		for _, attachment := range content.attachments {
			s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "file.reference.added", slog.String(recordAttached, attachment.String()),
				slog.String(recordRefKind, string(domain.ReferenceIssueSnapshot)))
		}
	})
	return item, nil
}

func (s *Service) lockAudience(
	ctx context.Context, repos repositories.Repositories, tutor domain.UserID, request IssueAudienceRequest,
) (lockedAudience, error) {
	if request.Relationship != nil {
		return s.lockRelationshipAudience(ctx, repos, tutor, *request.Relationship)
	}
	if request.Group == nil {
		return lockedAudience{}, ErrNotFound
	}
	group, err := repos.Homework.Group(ctx, tutor, *request.Group)
	if errors.Is(err, repositories.ErrNotFound) {
		return lockedAudience{}, ErrNotFound
	}
	if err != nil {
		return lockedAudience{}, err
	}
	locked, settled, err := s.lockMembers(ctx, repos, tutor, group.ID)
	if err != nil {
		return lockedAudience{}, err
	}
	lockedGroup, err := repos.Homework.LockGroup(ctx, tutor, group.ID)
	if err != nil {
		return lockedAudience{}, err
	}
	audience := lockedAudience{group: &lockedGroup.ID}
	if lockedGroup.ArchivedAt != nil {
		audience.refusal = ErrAudienceNotActive
		return audience, nil
	}
	if !settled {
		audience.refusal = ErrConflict
		return audience, nil
	}
	for _, member := range locked {
		if member.State == domain.RelationshipActive && member.StudentID != nil {
			audience.recipients = append(audience.recipients, repositories.NewSubmission{Relationship: member.ID, Student: *member.StudentID})
		}
	}
	return audience, nil
}

func (s *Service) lockRelationshipAudience(
	ctx context.Context, repos repositories.Repositories, tutor domain.UserID, id domain.RelationshipID,
) (lockedAudience, error) {
	if _, err := repos.Homework.Relationship(ctx, tutor, id); errors.Is(err, repositories.ErrNotFound) {
		return lockedAudience{}, ErrNotFound
	} else if err != nil {
		return lockedAudience{}, err
	}
	locked, err := repos.Homework.LockRelationships(ctx, tutor, []domain.RelationshipID{id})
	if err != nil {
		return lockedAudience{}, err
	}
	if len(locked) != 1 || locked[0].State == domain.RelationshipDiscarded {
		return lockedAudience{}, ErrNotFound
	}
	link := locked[0]
	audience := lockedAudience{relationship: &link.ID}
	if !domain.IssuableRelationship(link.State) || link.StudentID == nil {
		audience.refusal = ErrRelationshipNotActive
		return audience, nil
	}
	audience.recipients = []repositories.NewSubmission{{Relationship: link.ID, Student: *link.StudentID}}
	return audience, nil
}

func (s *Service) lockMembers(
	ctx context.Context, repos repositories.Repositories, tutor domain.UserID, group domain.GroupID,
) ([]repositories.LockedIssueRelationship, bool, error) {
	members, err := repos.Homework.GroupMembers(ctx, tutor, group)
	if err != nil {
		return nil, false, err
	}
	for range membershipAttempts {
		locked, err := repos.Homework.LockRelationships(ctx, tutor, members)
		if err != nil {
			return nil, false, err
		}
		again, err := repos.Homework.GroupMembers(ctx, tutor, group)
		if err != nil {
			return nil, false, err
		}
		if slices.Equal(members, again) {
			return locked, true, nil
		}
		members = again
	}
	return nil, false, nil
}

func (s *Service) lockLesson(
	ctx context.Context, repos repositories.Repositories, tutor domain.UserID, lesson domain.LessonID,
	relationship *domain.RelationshipID, group *domain.GroupID,
) error {
	err := repos.Homework.LockLesson(ctx, tutor, lesson, relationship, group)
	if errors.Is(err, repositories.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

func (s *Service) refuseBeyondBounds(
	ctx context.Context, repos repositories.Repositories, tutor domain.UserID, snapshotBytes int64, lesson *domain.LessonID,
) error {
	if snapshotBytes > 0 {
		total, err := repos.Homework.SnapshotBytesTotal(ctx, tutor)
		if err != nil {
			return err
		}
		if total+snapshotBytes > s.deps.Caps.SnapshotBudgetBytes {
			return invalid(fieldBody, domain.FieldTooMany)
		}
	}
	return s.refuseLessonFull(ctx, repos, tutor, lesson)
}

func (s *Service) refuseLessonFull(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, lesson *domain.LessonID) error {
	if lesson == nil {
		return nil
	}
	count, err := repos.Homework.LessonIssueCount(ctx, tutor, *lesson)
	if err != nil {
		return err
	}
	if count >= s.deps.Caps.IssuesPerLesson {
		return invalid(fieldLesson, domain.FieldTooMany)
	}
	return nil
}
