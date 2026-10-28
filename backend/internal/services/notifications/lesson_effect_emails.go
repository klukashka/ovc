package notifications

import (
	"context"
	"errors"
	"fmt"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/core/timezonecities"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

const schedulePagePath = "/schedule"

type cancelledList struct {
	heading string
	lines   []string
	more    string
}

func listCancelled(
	ctx context.Context, repos repositories.Repositories, cities timezonecities.Table, recipient repositories.User,
	query repositories.EffectCancellations,
) (cancelledList, error) {
	text, found := core.CancelledLessonsEmailText(recipient.Language)
	if !found {
		return cancelledList{}, domain.PermanentEffect(fmt.Errorf("%w: no cancelled lessons text in %s", errUnknownKind, recipient.Language))
	}
	query.Limit = domain.ChangeEmailLessonLimit
	lessons, err := repos.Lessons.CancelledByEffect(ctx, query)
	if err != nil || len(lessons) == 0 {
		return cancelledList{}, err
	}
	list := cancelledList{heading: text.Heading}
	for _, lesson := range lessons {
		when, err := lessonStart(cities, recipient, lesson.StartsAt, lesson.Timezone)
		if err != nil {
			return cancelledList{}, err
		}
		list.lines = append(list.lines, core.EmailSafeText(lesson.Title)+", "+when)
	}
	if left := int(lessons[0].Total) - len(lessons); left > 0 {
		list.more = text.More(left)
	}
	return list, nil
}

type groupArchivedEmail struct {
	emails *LessonEmails
}

type archivedNotice struct {
	group      repositories.ArchivedEmailGroup
	recipient  repositories.User
	archivedAt time.Time
}

func (b groupArchivedEmail) notice(ctx context.Context, repos repositories.Repositories, encoded []byte) (archivedNotice, bool, error) {
	var payload domain.GroupArchivedPayload
	if err := decodePayload(encoded, &payload); err != nil {
		return archivedNotice{}, false, err
	}
	groupID, err := decodeID[domain.GroupID](payload.Group)
	if err != nil {
		return archivedNotice{}, false, err
	}
	recipientID, err := decodeID[domain.UserID](payload.Recipient)
	if err != nil {
		return archivedNotice{}, false, err
	}
	group, err := repos.Groups.ForArchivedEmail(ctx, groupID)
	if errors.Is(err, repositories.ErrNotFound) {
		return archivedNotice{}, false, nil
	}
	if err != nil {
		return archivedNotice{}, false, err
	}
	recipient, found, err := existingUser(ctx, repos, recipientID)
	if err != nil || !found {
		return archivedNotice{}, false, err
	}
	link, err := repos.Relationships.OfTutorAndStudent(ctx, group.TutorID, recipient.ID)
	if errors.Is(err, repositories.ErrNotFound) {
		return archivedNotice{}, false, nil
	}
	if err != nil {
		return archivedNotice{}, false, err
	}
	if link.State != domain.RelationshipActive {
		return archivedNotice{}, false, nil
	}
	return archivedNotice{group: group, recipient: recipient, archivedAt: time.UnixMicro(payload.ArchivedAtMicros).UTC()}, true, nil
}

func (b groupArchivedEmail) Build(ctx context.Context, row repositories.ClaimedOutboxRow, now time.Time) (Composition, error) {
	repos := b.emails.store.Repositories()
	notice, found, err := b.notice(ctx, repos, row.Payload)
	if err != nil || !found {
		return Composition{Obsolete: err == nil}, err
	}
	recipient, group := notice.recipient, notice.group
	composition := changeComposition(recipient, group.TutorState)
	if composition.Drop != "" {
		return composition, nil
	}
	text, found := core.GroupArchivedEmailText(recipient.Language)
	if !found {
		return Composition{}, domain.PermanentEffect(fmt.Errorf("%w: no %s text in %s", errUnknownKind, row.Kind, recipient.Language))
	}
	member, err := repos.Lessons.IsCurrentMember(ctx, group.ID, recipient.ID)
	if err != nil {
		return Composition{}, err
	}
	label := text.GroupLesson
	if member {
		label = core.EmailSafeText(group.Name)
	}
	list, err := listCancelled(ctx, repos, b.emails.cities, recipient, repositories.EffectCancellations{
		Cause: domain.CancelGroupArchived, CancelledAt: notice.archivedAt, Tutor: group.TutorID, Group: &group.ID, OnRosterOf: &recipient.ID,
	})
	if err != nil {
		return Composition{}, err
	}
	view := lessonView{
		Language: recipient.Language, Subject: text.SubjectPrefix + ": " + label, Intro: text.Intro, Title: label,
		Details: []lessonDetail{{Label: text.Tutor, Value: core.EmailSafeText(group.TutorName)}},
		Lessons: list.lines, More: list.more, Action: text.Action, ActionURL: b.emails.publicURL + schedulePagePath, Note: text.Note,
		Unsubscribe: text.Unsubscribe, UnsubscribeNote: text.UnsubscribeNote,
	}
	composition.Email, err = b.emails.execute(view, recipient, composition.Optional.Group, now)
	if err != nil {
		return Composition{}, err
	}
	composition.Identity = group.ID.String() + ":" + recipient.ID.String()
	return composition, nil
}
