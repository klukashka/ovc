package schedule

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"time"
	"unicode"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/domain/schedule"
	"ovc/backend/internal/repositories"
)

var (
	ErrNotEditable  = errors.New("the lesson has left scheduled or started")
	ErrStale        = errors.New("the lesson changed since the client read it")
	errCoveredCount = errors.New("a counted series covers more occurrences than a series may count")
)

const fieldScope = "body.scope"

type LessonEdit struct {
	Scope           domain.EditScope
	Version         int32
	Date            *string
	Start           *string
	DurationMinutes *int16
	Title           *string
	DescriptionSet  bool
	Description     *string
	AllowOverlap    bool
}

type LessonCancellation struct {
	Scope   domain.EditScope
	Version int32
}

type lessonWrite struct {
	account        repositories.User
	target         repositories.EditedLessonTarget
	audience       jobAudience
	membersChanged bool
	series         *repositories.LockedSeriesForEdit
	selected       repositories.LockedLesson
	covered        []repositories.LockedLesson
}

func (w lessonWrite) occurrenceDate() domain.LocalDate {
	if w.selected.OccurrenceDate == nil {
		return domain.LocalDate{}
	}
	return *w.selected.OccurrenceDate
}

func (w lessonWrite) coveredIDs() []domain.LessonID {
	ids := make([]domain.LessonID, 0, len(w.covered))
	for _, lesson := range w.covered {
		ids = append(ids, lesson.ID)
	}
	return ids
}

type writeOutcome struct {
	result   Created
	changed  bool
	fields   []domain.LessonField
	split    *domain.SeriesID
	count    int
	rows     []queuedNotification
	original *domain.SeriesID
}

func editable(lesson repositories.LockedLesson, now time.Time) bool {
	return lesson.Status == domain.LessonScheduled && lesson.StartsAt.After(now)
}

func (s *Service) admitLessonWrite(ctx context.Context, tutor domain.UserID, id domain.LessonID) error {
	return s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		if _, err := s.lockActingTutor(ctx, repos, tutor); err != nil {
			return err
		}
		if _, err := repos.Lessons.TutorTarget(ctx, tutor, id); errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		now, err := s.deps.Clock.InstantWithin(ctx, repos.TestClock)
		if err != nil {
			return err
		}
		return s.reserveWrite(ctx, repos, tutor, now)
	})
}

func (s *Service) writeWithAudienceRetry(ctx context.Context, work func(repos repositories.Repositories) error) error {
	for attempt := 1; ; attempt++ {
		err := s.deps.Store.WithTx(ctx, work)
		if !errors.Is(err, errMembersChanged) {
			return err
		}
		if attempt == audienceAttempts {
			return ErrConflict
		}
	}
}

func (s *Service) lockWrite(
	ctx context.Context, repos repositories.Repositories, tutor domain.UserID, id domain.LessonID, scope domain.EditScope,
) (lessonWrite, error) {
	account, err := s.lockActingTutor(ctx, repos, tutor)
	if err != nil {
		return lessonWrite{}, err
	}
	target, err := repos.Lessons.TutorTarget(ctx, tutor, id)
	if errors.Is(err, repositories.ErrNotFound) {
		return lessonWrite{}, ErrNotFound
	}
	if err != nil {
		return lessonWrite{}, err
	}
	if scope == domain.ScopeFollowing && target.SeriesID == nil {
		return lessonWrite{}, invalid(fieldScope, domain.FieldNotAllowed)
	}
	audience, changed, err := lockAudienceOf(ctx, repos, tutor, target.RelationshipID, target.GroupID)
	if err != nil {
		return lessonWrite{}, err
	}
	write := lessonWrite{account: account, target: target, audience: audience, membersChanged: changed}
	var following *domain.SeriesID
	switch {
	case target.SeriesID == nil:
	case scope == domain.ScopeFollowing:
		series, err := repos.Lessons.LockSeriesForEdit(ctx, tutor, *target.SeriesID)
		if err != nil {
			return lessonWrite{}, err
		}
		write.series, following = &series, target.SeriesID
	default:
		if err := repos.Lessons.LockSeriesShared(ctx, *target.SeriesID); err != nil {
			return lessonWrite{}, err
		}
	}
	from := domain.LocalDate{}
	if target.OccurrenceDate.Valid {
		from = domain.DateOf(target.OccurrenceDate.Time)
	}
	rows, err := repos.Lessons.LockForEdit(ctx, tutor, id, following, from)
	if err != nil {
		return lessonWrite{}, err
	}
	found := slices.IndexFunc(rows, func(row repositories.LockedLesson) bool { return row.ID == id })
	if found < 0 {
		return lessonWrite{}, ErrNotFound
	}
	write.selected = rows[found]
	if scope == domain.ScopeFollowing && !sameSeries(write.selected.SeriesID, target.SeriesID) {
		return lessonWrite{}, ErrStale
	}
	write.covered = rows
	slices.SortFunc(write.covered, func(a, b repositories.LockedLesson) int { return a.StartsAt.Compare(b.StartsAt) })
	return write, nil
}

func sameSeries(a *domain.SeriesID, b *domain.SeriesID) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func (s *Service) checkState(write lessonWrite, version int32, now time.Time) error {
	if !editable(write.selected, now) {
		return ErrNotEditable
	}
	if write.selected.Version != version {
		return ErrStale
	}
	return nil
}

type editChanges struct {
	date           *domain.LocalDate
	start          *domain.LocalTime
	duration       *int16
	title          *string
	descriptionSet bool
	description    *string
}

func shapeProblem(edit LessonEdit) error {
	if edit.Date == nil && edit.Start == nil && edit.DurationMinutes == nil && edit.Title == nil && !edit.DescriptionSet {
		return invalid(fieldBody, domain.FieldRequired)
	}
	if edit.Scope == domain.ScopeFollowing && edit.Date != nil {
		return invalid(fieldDate, domain.FieldNotAllowed)
	}
	return nil
}

func parseEdit(edit LessonEdit) (editChanges, error) {
	changes := editChanges{duration: edit.DurationMinutes, title: edit.Title, descriptionSet: edit.DescriptionSet, description: normalisedDescription(edit.Description)}
	var problems []FieldProblem
	if edit.Date != nil {
		date, valid := domain.ParseLocalDate(*edit.Date)
		if !valid {
			problems = append(problems, FieldProblem{Path: fieldDate, Code: domain.FieldInvalid})
		}
		changes.date = &date
	}
	if edit.Start != nil {
		start, valid := domain.ParseLocalTime(*edit.Start)
		if !valid {
			problems = append(problems, FieldProblem{Path: fieldStart, Code: domain.FieldInvalid})
		}
		changes.start = &start
	}
	if edit.Title != nil && strings.ContainsFunc(*edit.Title, unicode.IsControl) {
		problems = append(problems, FieldProblem{Path: fieldTitle, Code: domain.FieldInvalid})
	}
	if edit.Description != nil && strings.ContainsFunc(*edit.Description, controlOtherThanLineBreak) {
		problems = append(problems, FieldProblem{Path: fieldDescription, Code: domain.FieldInvalid})
	}
	if len(problems) > 0 {
		return editChanges{}, &ValidationError{Fields: problems}
	}
	return changes, nil
}

type plannedEdit struct {
	values   repositories.EditedValues
	previous repositories.LockedLesson
	fields   []domain.LessonField
}

func (p plannedEdit) changed() bool {
	return len(p.fields) > 0
}

func samePointer(a *string, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

type editRule struct {
	changes   editChanges
	zone      *time.Location
	applyAll  bool
	overrides bool
	now       time.Time
}

func (r editRule) plan(lesson repositories.LockedLesson) plannedEdit {
	takes := func(field domain.LessonField) bool {
		return r.applyAll || !slices.Contains(lesson.OverriddenFields, field)
	}
	local := lesson.StartsAt.In(r.zone)
	date, start := domain.DateOf(local), domain.LocalTime{Hour: local.Hour(), Minute: local.Minute()}
	planned := plannedEdit{previous: lesson, values: repositories.EditedValues{
		ID: lesson.ID, StartsAt: lesson.StartsAt, DurationMinutes: lesson.DurationMinutes, Title: lesson.Title, Description: lesson.Description,
		OverriddenFields: lesson.OverriddenFields, Now: r.now,
	}}
	if r.changes.date != nil && takes(domain.LessonFieldDate) && *r.changes.date != date {
		date = *r.changes.date
		planned.fields = append(planned.fields, domain.LessonFieldDate)
	}
	if r.changes.start != nil && takes(domain.LessonFieldStart) && *r.changes.start != start {
		start = *r.changes.start
		planned.fields = append(planned.fields, domain.LessonFieldStart)
	}
	planned.values.StartsAt = schedule.StartInstant(date, start, r.zone)
	if !slices.Contains(planned.fields, domain.LessonFieldDate) && !slices.Contains(planned.fields, domain.LessonFieldStart) {
		planned.values.StartsAt = lesson.StartsAt
	}
	if r.changes.duration != nil && takes(domain.LessonFieldDuration) && *r.changes.duration != lesson.DurationMinutes {
		planned.values.DurationMinutes = *r.changes.duration
		planned.fields = append(planned.fields, domain.LessonFieldDuration)
	}
	if r.changes.title != nil && takes(domain.LessonFieldTitle) && *r.changes.title != lesson.Title {
		planned.values.Title = *r.changes.title
		planned.fields = append(planned.fields, domain.LessonFieldTitle)
	}
	if r.changes.descriptionSet && takes(domain.LessonFieldDescription) && !samePointer(r.changes.description, lesson.Description) {
		planned.values.Description = r.changes.description
		planned.fields = append(planned.fields, domain.LessonFieldDescription)
	}
	planned.values.Moved = !planned.values.StartsAt.Equal(lesson.StartsAt) || planned.values.DurationMinutes != lesson.DurationMinutes
	if r.overrides {
		overridden := slices.Clone(lesson.OverriddenFields)
		for _, field := range planned.fields {
			if !slices.Contains(overridden, field) {
				overridden = append(overridden, field)
			}
		}
		planned.values.OverriddenFields = overridden
	}
	return planned
}

func (s *Service) EditLesson(ctx context.Context, tutor domain.UserID, id domain.LessonID, edit LessonEdit) (Created, error) {
	if err := s.admitLessonWrite(ctx, tutor, id); err != nil {
		return Created{}, err
	}
	var outcome writeOutcome
	err := s.writeWithAudienceRetry(ctx, func(repos repositories.Repositories) error {
		var err error
		outcome, err = s.edit(ctx, repos, tutor, id, edit)
		return err
	})
	var overlap *OverlapError
	if errors.As(err, &overlap) {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "lesson.overlap_warned", tutorAttribute(tutor), slog.Int("count", len(overlap.Lessons)+overlap.More))
	}
	if err != nil {
		return Created{}, err
	}
	if outcome.changed {
		s.logEdit(ctx, tutor, id, edit.Scope, outcome)
	}
	return outcome.result, nil
}

func (s *Service) logEdit(ctx context.Context, tutor domain.UserID, id domain.LessonID, scope domain.EditScope, outcome writeOutcome) {
	fields := make([]string, 0, len(outcome.fields))
	for _, field := range outcome.fields {
		fields = append(fields, string(field))
	}
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "lesson.edited", tutorAttribute(tutor), lessonAttributes(id), slog.String("scope", string(scope)),
		slog.String("fields", strings.Join(fields, ",")), slog.Int("count", outcome.count))
	if outcome.split != nil && outcome.original != nil {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "series.split", tutorAttribute(tutor), slog.String("series_id", outcome.original.String()),
			slog.String("split_series_id", outcome.split.String()))
	}
	s.logRows(ctx, outcome.rows)
}

func (s *Service) logRows(ctx context.Context, rows []queuedNotification) {
	for _, row := range rows {
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "notification.created", slog.String("event", string(row.event)),
			slog.String("target_user_id", row.recipient.String()), slog.String("notification_id", row.id.String()))
	}
}

func (s *Service) edit(
	ctx context.Context, repos repositories.Repositories, tutor domain.UserID, id domain.LessonID, edit LessonEdit,
) (writeOutcome, error) {
	if err := shapeProblem(edit); err != nil {
		return writeOutcome{}, err
	}
	write, err := s.lockWrite(ctx, repos, tutor, id, edit.Scope)
	if err != nil {
		return writeOutcome{}, err
	}
	now, err := s.deps.Clock.InstantWithin(ctx, repos.TestClock)
	if err != nil {
		return writeOutcome{}, err
	}
	if err := s.checkState(write, edit.Version, now); err != nil {
		return writeOutcome{}, err
	}
	changes, err := parseEdit(edit)
	if err != nil {
		return writeOutcome{}, err
	}
	if edit.Scope == domain.ScopeFollowing {
		return s.editFollowing(ctx, repos, write, changes, edit.AllowOverlap, now)
	}
	return s.editThis(ctx, repos, write, changes, edit.AllowOverlap, now)
}

func intervalOf(values repositories.EditedValues) schedule.Interval {
	return schedule.Interval{Start: values.StartsAt, End: values.StartsAt.Add(minutes(int(values.DurationMinutes)))}
}

func (s *Service) editThis(
	ctx context.Context, repos repositories.Repositories, write lessonWrite, changes editChanges, allowOverlap bool, now time.Time,
) (writeOutcome, error) {
	zone, err := loadZone(write.selected.Timezone)
	if err != nil {
		return writeOutcome{}, err
	}
	planned := editRule{changes: changes, zone: zone, applyAll: true, overrides: true, now: now}.plan(write.selected)
	if planned.values.Moved && !planned.values.StartsAt.After(now) {
		return writeOutcome{}, ErrLessonInPast
	}
	if !planned.changed() {
		lessons, err := s.tutorViews(ctx, repos, write.account, []domain.LessonID{write.selected.ID})
		return writeOutcome{result: Created{Lessons: lessons}}, err
	}
	if write.membersChanged {
		return writeOutcome{}, errMembersChanged
	}
	if planned.values.Moved && !allowOverlap {
		excluded := overlapExclusions{lessons: map[domain.LessonID]bool{write.selected.ID: true}}
		if err := checkIntervals(ctx, repos, write.account.ID, []schedule.Interval{intervalOf(planned.values)}, excluded); err != nil {
			return writeOutcome{}, err
		}
	}
	revision, err := repos.Lessons.ApplyEdit(ctx, planned.values)
	if err != nil {
		return writeOutcome{}, err
	}
	outcome := writeOutcome{changed: true, fields: planned.fields, count: 1}
	if err := signalLessons(ctx, repos, write.account.ID, write.audience.students(), []domain.LessonID{write.selected.ID}); err != nil {
		return writeOutcome{}, err
	}
	if planned.values.Moved {
		rows, err := s.queueMoved(ctx, repos, write, revision, now)
		if err != nil {
			return writeOutcome{}, err
		}
		outcome.rows = rows
	}
	outcome.result.Lessons, err = s.tutorViews(ctx, repos, write.account, []domain.LessonID{write.selected.ID})
	return outcome, err
}

func (s *Service) queueMoved(ctx context.Context, repos repositories.Repositories, write lessonWrite, revision int32, now time.Time) ([]queuedNotification, error) {
	var rows []queuedNotification
	for _, target := range studentTargets(write.audience) {
		key := domain.LessonReminderKey(domain.EventLessonMoved, write.selected.ID, target.user, revision)
		payload, err := encodeJSON(domain.LessonReminderPayload{Lesson: hexID(write.selected.ID), Recipient: hexID(target.user), Revision: revision})
		if err != nil {
			return nil, err
		}
		if err := s.queueLessonEmail(ctx, repos, lessonEmail{
			kind: domain.EventLessonMoved, priority: domain.PriorityNormal, payload: payload, key: key, now: now,
		}); err != nil {
			return nil, err
		}
		row, err := s.insertLessonRow(ctx, repos, lessonRow{
			event: domain.NotificationLessonMoved, tutor: write.account.ID, subject: write.selected.ID, target: target, key: key, now: now,
		})
		if err != nil {
			return nil, err
		}
		if row != nil {
			rows = append(rows, *row)
		}
	}
	return rows, nil
}

func studentTargets(audience jobAudience) []notificationTarget {
	targets := make([]notificationTarget, 0, len(audience.members))
	for _, member := range audience.members {
		student, relationship := member.student, member.relationship
		targets = append(targets, notificationTarget{user: student, student: &student, relationship: &relationship})
	}
	return targets
}

func (s *Service) editFollowing(
	ctx context.Context, repos repositories.Repositories, write lessonWrite, changes editChanges, allowOverlap bool, now time.Time,
) (writeOutcome, error) {
	original := *write.series
	zone, err := loadZone(original.Timezone)
	if err != nil {
		return writeOutcome{}, err
	}
	var planned []plannedEdit
	var fields []domain.LessonField
	for _, lesson := range write.covered {
		if !editable(lesson, now) {
			continue
		}
		edit := editRule{changes: changes, zone: zone, applyAll: lesson.ID == write.selected.ID, now: now}.plan(lesson)
		if edit.values.Moved && !edit.values.StartsAt.After(now) {
			return writeOutcome{}, ErrLessonInPast
		}
		if edit.changed() {
			planned = append(planned, edit)
			for _, field := range edit.fields {
				if !slices.Contains(fields, field) {
					fields = append(fields, field)
				}
			}
		}
	}
	split := splitValues(original, changes)
	if len(planned) == 0 && !split.differs {
		views, err := s.tutorViews(ctx, repos, write.account, write.coveredIDs())
		return writeOutcome{result: Created{Series: seriesView(original.Series), Lessons: views}}, err
	}
	from := write.occurrenceDate()
	if err := repos.Lessons.EndSeriesBefore(ctx, original.ID, from, now); err != nil {
		return writeOutcome{}, err
	}
	open, err := repos.Lessons.CountOpenSeries(ctx, write.account.ID, now)
	if err != nil {
		return writeOutcome{}, err
	}
	if open >= domain.OpenSeriesCapPerTutor {
		return writeOutcome{}, invalid(fieldBody, domain.FieldTooMany)
	}
	if write.membersChanged {
		return writeOutcome{}, errMembersChanged
	}
	if !allowOverlap {
		if err := s.checkSplitOverlap(ctx, repos, write, planned, split, zone); err != nil {
			return writeOutcome{}, err
		}
	}
	created, err := s.createSplit(ctx, repos, write, split, from, now)
	if err != nil {
		return writeOutcome{}, err
	}
	var changed []domain.LessonID
	for _, edit := range planned {
		if _, err := repos.Lessons.ApplyEdit(ctx, edit.values); err != nil {
			return writeOutcome{}, err
		}
		changed = append(changed, edit.values.ID)
	}
	if err := s.signalCovered(ctx, repos, write); err != nil {
		return writeOutcome{}, err
	}
	rows, err := s.queueSeriesChanged(ctx, repos, write, write.audience, changed, now)
	if err != nil {
		return writeOutcome{}, err
	}
	views, err := s.tutorViews(ctx, repos, write.account, write.coveredIDs())
	if err != nil {
		return writeOutcome{}, err
	}
	outcome := writeOutcome{
		changed: true, fields: fields, split: &created.ID, original: &original.ID, count: len(changed), rows: rows,
		result: Created{Series: seriesView(created), Lessons: views},
	}
	return outcome, nil
}

type splitRule struct {
	start       domain.LocalTime
	duration    int16
	title       string
	description *string
	differs     bool
}

func splitValues(original repositories.LockedSeriesForEdit, changes editChanges) splitRule {
	rule := splitRule{start: original.Start, duration: original.DurationMinutes, title: original.Title, description: original.Description}
	if changes.start != nil && *changes.start != rule.start {
		rule.start, rule.differs = *changes.start, true
	}
	if changes.duration != nil && *changes.duration != rule.duration {
		rule.duration, rule.differs = *changes.duration, true
	}
	if changes.title != nil && *changes.title != rule.title {
		rule.title, rule.differs = *changes.title, true
	}
	if changes.descriptionSet && !samePointer(changes.description, rule.description) {
		rule.description, rule.differs = changes.description, true
	}
	return rule
}

func (s *Service) createSplit(
	ctx context.Context, repos repositories.Repositories, write lessonWrite, rule splitRule, from domain.LocalDate, now time.Time,
) (repositories.Series, error) {
	original := *write.series
	id, err := core.NewID[domain.SeriesID](s.deps.Random)
	if err != nil {
		return repositories.Series{}, err
	}
	split := repositories.SplitSeries{
		ID: id, OriginalID: original.ID, Start: rule.start, DurationMinutes: rule.duration, First: from, Title: rule.title,
		Description: rule.description, Now: now, EndDate: original.EndDate,
	}
	if original.Count != nil {
		covered := len(write.covered)
		if covered < 1 || covered > domain.SeriesMaxCount {
			return repositories.Series{}, errCoveredCount
		}
		count := int16(covered)
		split.Count, split.EndDate = &count, nil
	}
	if err := repos.Lessons.CreateSplitSeries(ctx, split); err != nil {
		return repositories.Series{}, err
	}
	if _, err := repos.Lessons.MoveToSeries(ctx, original.ID, id, from, now); err != nil {
		return repositories.Series{}, err
	}
	return repositories.Series{
		ID: id, Weekdays: original.Weekdays, Start: rule.start, DurationMinutes: rule.duration, Timezone: original.Timezone, First: from,
		EndDate: split.EndDate, Count: split.Count,
	}, nil
}

func (s *Service) checkSplitOverlap(
	ctx context.Context, repos repositories.Repositories, write lessonWrite, planned []plannedEdit, rule splitRule, zone *time.Location,
) error {
	original := *write.series
	var candidates []schedule.Interval
	for _, edit := range planned {
		if edit.values.Moved {
			candidates = append(candidates, intervalOf(edit.values))
		}
	}
	moved := rule.start != original.Start || rule.duration != original.DurationMinutes
	if moved && original.EndDate != nil && original.EndDate.After(original.MaterialisedUntil) {
		series := schedule.Series{Weekdays: original.Weekdays, First: original.First, EndDate: original.EndDate, Start: rule.start, Zone: zone}
		for _, occurrence := range schedule.DatesBetween(series, original.MaterialisedUntil, *original.EndDate) {
			candidates = append(candidates, schedule.Interval{Start: occurrence.StartsAt, End: occurrence.StartsAt.Add(minutes(int(rule.duration)))})
		}
	}
	excluded := overlapExclusions{lessons: map[domain.LessonID]bool{}, series: map[domain.SeriesID]bool{original.ID: true}}
	for _, lesson := range write.covered {
		excluded.lessons[lesson.ID] = true
	}
	return checkIntervals(ctx, repos, write.account.ID, candidates, excluded)
}

func (s *Service) queueSeriesChanged(
	ctx context.Context, repos repositories.Repositories, write lessonWrite, audience jobAudience, affected []domain.LessonID, now time.Time,
) ([]queuedNotification, error) {
	if len(affected) == 0 {
		return nil, nil
	}
	operation, err := core.NewID[domain.OperationID](s.deps.Random)
	if err != nil {
		return nil, err
	}
	listed := make([]string, 0, min(len(affected), domain.SeriesChangeLessonLimit))
	for _, lesson := range affected[:min(len(affected), domain.SeriesChangeLessonLimit)] {
		listed = append(listed, hexID(lesson))
	}
	var rows []queuedNotification
	for _, target := range studentTargets(audience) {
		key := domain.SeriesChangedKey(operation, target.user)
		payload, err := encodeJSON(domain.SeriesChangedPayload{
			Operation: hexID(operation), Tutor: hexID(write.account.ID), Subject: hexID(write.selected.ID), Lessons: listed, Recipient: hexID(target.user),
		})
		if err != nil {
			return nil, err
		}
		if err := s.queueLessonEmail(ctx, repos, lessonEmail{
			kind: domain.EventLessonSeriesChanged, priority: domain.PriorityNormal, payload: payload, key: key, now: now,
		}); err != nil {
			return nil, err
		}
		row, err := s.insertLessonRow(ctx, repos, lessonRow{
			event: domain.NotificationSeriesChanged, tutor: write.account.ID, subject: write.selected.ID, target: target, key: key, now: now,
		})
		if err != nil {
			return nil, err
		}
		if row != nil {
			rows = append(rows, *row)
		}
	}
	return rows, nil
}

func (s *Service) signalCovered(ctx context.Context, repos repositories.Repositories, write lessonWrite) error {
	var scheduled, frozen []domain.LessonID
	for _, lesson := range write.covered {
		if lesson.Status == domain.LessonScheduled {
			scheduled = append(scheduled, lesson.ID)
		} else {
			frozen = append(frozen, lesson.ID)
		}
	}
	if err := signalLessons(ctx, repos, write.account.ID, write.audience.students(), scheduled); err != nil {
		return err
	}
	return signalLessons(ctx, repos, write.account.ID, nil, frozen)
}
