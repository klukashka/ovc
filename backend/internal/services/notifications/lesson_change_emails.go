package notifications

import (
	"context"
	"errors"
	"fmt"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

func changeText(language domain.Language, kind domain.OutboxKind) (core.LessonChangeText, error) {
	text, found := core.LessonChangeEmailText(language, kind)
	if !found {
		return core.LessonChangeText{}, domain.PermanentEffect(fmt.Errorf("%w: no %s text in %s", errUnknownKind, kind, language))
	}
	return text, nil
}

func changeComposition(recipient repositories.User, tutorState domain.AccountState) Composition {
	composition := Composition{
		Recipient: AccountRecipient(recipient.State),
		Optional:  &Optional{Recipient: recipient.ID, Group: domain.EmailGroupLessonChanges, Capped: true},
	}
	if tutorState != domain.AccountActive {
		composition.Drop = domain.DropTutorInactive
	}
	return composition
}

type lessonMovedEmail struct {
	emails *LessonEmails
}

func (b lessonMovedEmail) Build(ctx context.Context, row repositories.ClaimedOutboxRow, now time.Time) (Composition, error) {
	var payload domain.LessonReminderPayload
	if err := decodePayload(row.Payload, &payload); err != nil {
		return Composition{}, err
	}
	lessonID, err := decodeID[domain.LessonID](payload.Lesson)
	if err != nil {
		return Composition{}, err
	}
	recipientID, err := decodeID[domain.UserID](payload.Recipient)
	if err != nil {
		return Composition{}, err
	}
	repos := b.emails.store.Repositories()
	lesson, err := repos.Lessons.ForEmail(ctx, lessonID)
	if errors.Is(err, repositories.ErrNotFound) {
		return Composition{Obsolete: true}, nil
	}
	if err != nil {
		return Composition{}, err
	}
	if lesson.Status != domain.LessonScheduled || lesson.ScheduleRevision != payload.Revision {
		return Composition{Obsolete: true}, nil
	}
	recipient, found, err := existingUser(ctx, repos, recipientID)
	if err != nil || !found {
		return Composition{Obsolete: !found}, err
	}
	inAudience, err := b.emails.inAudience(ctx, repos, lesson, recipient.ID)
	if err != nil || !inAudience {
		return Composition{Obsolete: !inAudience}, err
	}
	composition := changeComposition(recipient, lesson.TutorState)
	if composition.Drop != "" {
		return composition, nil
	}
	text, err := changeText(recipient.Language, row.Kind)
	if err != nil {
		return Composition{}, err
	}
	composition.Email, err = b.emails.render(lessonRender{
		lesson: lesson, recipient: recipient, text: text.LessonEmailText, group: domain.EmailGroupLessonChanges,
	}, now)
	return composition, err
}

type lessonCancelledEmail struct {
	emails *LessonEmails
}

func (b lessonCancelledEmail) Build(ctx context.Context, row repositories.ClaimedOutboxRow, now time.Time) (Composition, error) {
	var payload domain.LessonCancelledPayload
	if err := decodePayload(row.Payload, &payload); err != nil {
		return Composition{}, err
	}
	lessonID, err := decodeID[domain.LessonID](payload.Lesson)
	if err != nil {
		return Composition{}, err
	}
	recipientID, err := decodeID[domain.UserID](payload.Recipient)
	if err != nil {
		return Composition{}, err
	}
	repos := b.emails.store.Repositories()
	lesson, err := repos.Lessons.ForEmail(ctx, lessonID)
	if errors.Is(err, repositories.ErrNotFound) {
		return Composition{Obsolete: true}, nil
	}
	if err != nil {
		return Composition{}, err
	}
	recipient, found, err := existingUser(ctx, repos, recipientID)
	if err != nil || !found {
		return Composition{Obsolete: !found}, err
	}
	listed, err := repos.Lessons.OnRoster(ctx, lesson.ID, recipient.ID)
	if err != nil || !listed || lesson.Status != domain.LessonCancelled {
		return Composition{Obsolete: err == nil}, err
	}
	composition := changeComposition(recipient, lesson.TutorState)
	if composition.Drop != "" {
		return composition, nil
	}
	former, err := b.emails.formerMember(ctx, repos, lesson, recipient.ID)
	if err != nil {
		return Composition{}, err
	}
	text, err := changeText(recipient.Language, row.Kind)
	if err != nil {
		return Composition{}, err
	}
	composition.Email, err = b.emails.render(lessonRender{
		lesson: lesson, recipient: recipient, text: text.LessonEmailText, group: domain.EmailGroupLessonChanges, former: former,
	}, now)
	return composition, err
}

func (l *LessonEmails) formerMember(ctx context.Context, repos repositories.Repositories, lesson repositories.LessonForEmail, student domain.UserID) (bool, error) {
	if lesson.GroupID == nil {
		return false, nil
	}
	member, err := repos.Lessons.IsCurrentMember(ctx, *lesson.GroupID, student)
	return !member, err
}

type seriesChangedEmail struct {
	emails *LessonEmails
}

type seriesChange struct {
	recipient repositories.User
	tutor     domain.UserID
	lessons   []domain.LessonID
}

func decodeSeriesChange(ctx context.Context, repos repositories.Repositories, encoded []byte) (seriesChange, bool, error) {
	var payload domain.SeriesChangedPayload
	if err := decodePayload(encoded, &payload); err != nil {
		return seriesChange{}, false, err
	}
	tutor, err := decodeID[domain.UserID](payload.Tutor)
	if err != nil {
		return seriesChange{}, false, err
	}
	recipientID, err := decodeID[domain.UserID](payload.Recipient)
	if err != nil {
		return seriesChange{}, false, err
	}
	if _, err := decodeID[domain.OperationID](payload.Operation); err != nil {
		return seriesChange{}, false, err
	}
	change := seriesChange{tutor: tutor}
	for _, encodedLesson := range payload.Lessons {
		lesson, err := decodeID[domain.LessonID](encodedLesson)
		if err != nil {
			return seriesChange{}, false, err
		}
		change.lessons = append(change.lessons, lesson)
	}
	recipient, found, err := existingUser(ctx, repos, recipientID)
	change.recipient = recipient
	return change, found, err
}

func (b seriesChangedEmail) Build(ctx context.Context, row repositories.ClaimedOutboxRow, now time.Time) (Composition, error) {
	repos := b.emails.store.Repositories()
	change, found, err := decodeSeriesChange(ctx, repos, row.Payload)
	if err != nil || !found {
		return Composition{Obsolete: err == nil}, err
	}
	visible, err := repos.Lessons.StudentNotificationLessons(ctx, change.recipient.ID, change.lessons)
	if err != nil || len(visible) == 0 {
		return Composition{Obsolete: err == nil}, err
	}
	ids := make([]domain.LessonID, 0, len(visible))
	for _, lesson := range visible {
		ids = append(ids, lesson.ID)
	}
	listed, err := repos.Lessons.ForChangeEmail(ctx, ids)
	if err != nil || len(listed) == 0 {
		return Composition{Obsolete: err == nil}, err
	}
	first, err := repos.Lessons.ForEmail(ctx, listed[0].ID)
	if errors.Is(err, repositories.ErrNotFound) {
		return Composition{Obsolete: true}, nil
	}
	if err != nil {
		return Composition{}, err
	}
	if first.TutorID != change.tutor {
		return Composition{}, domain.PermanentEffect(errMalformedPayload)
	}
	composition := changeComposition(change.recipient, first.TutorState)
	if composition.Drop != "" {
		return composition, nil
	}
	text, err := changeText(change.recipient.Language, row.Kind)
	if err != nil {
		return Composition{}, err
	}
	former, err := b.emails.formerMember(ctx, repos, first, change.recipient.ID)
	if err != nil {
		return Composition{}, err
	}
	source := lessonRender{
		lesson: first, recipient: change.recipient, text: text.LessonEmailText, group: domain.EmailGroupLessonChanges, former: former,
	}
	for index, lesson := range listed {
		if index == domain.ChangeEmailLessonLimit {
			source.more = text.More(len(listed) - domain.ChangeEmailLessonLimit)
			break
		}
		when, err := b.emails.when(change.recipient, lesson.StartsAt, lesson.Timezone)
		if err != nil {
			return Composition{}, err
		}
		line := core.EmailSafeText(lesson.Title) + ", " + when
		if lesson.Status == domain.LessonCancelled {
			line += " (" + text.Cancelled + ")"
		}
		source.lines = append(source.lines, line)
	}
	composition.Email, err = b.emails.render(source, now)
	return composition, err
}
