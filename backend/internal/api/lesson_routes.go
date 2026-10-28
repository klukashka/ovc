package api

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/schemas"
	"ovc/backend/internal/services/schedule"
)

const (
	lessonsPath  = "/api/v1/lessons"
	calendarPath = "/api/v1/calendar"
)

type calendarInput struct {
	From  string `query:"from" required:"true" format:"date-time"`
	To    string `query:"to" required:"true" format:"date-time"`
	Tutor string `query:"tutor" format:"uuid"`
}

type calendarOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.Calendar
}

type createLessonsInput struct {
	ContentType    string `header:"Content-Type"`
	IdempotencyKey string `header:"Idempotency-Key" maxLength:"200" pattern:"^[!-~]+$"`
	Body           schemas.NewLesson
}

type createdLessonsOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.CreatedLessons
}

type lessonInput struct {
	ID string `path:"id"`
}

type editLessonInput struct {
	ID          string `path:"id"`
	ContentType string `header:"Content-Type"`
	Body        schemas.LessonPatch
}

type cancelLessonInput struct {
	ID          string `path:"id"`
	ContentType string `header:"Content-Type"`
	Body        schemas.LessonCancellation
}

type lessonOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.LessonDetail
}

func (r *router) registerSchedule() {
	calendar := operation("calendar-get", http.MethodGet, calendarPath)
	calendar.Errors = errorsOf(http.StatusUnauthorized, http.StatusNotFound)
	register(r, calendar, AudienceUnverifiedOK, r.calendar)

	create := operation("lessons-create", http.MethodPost, lessonsPath)
	create.DefaultStatus = http.StatusCreated
	create.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusTooManyRequests)
	register(r, create, AudienceTutor, r.createLessons)

	get := operation("lessons-get", http.MethodGet, lessonsPath+"/{id}")
	get.Errors = errorsOf(http.StatusUnauthorized, http.StatusNotFound)
	register(r, get, AudienceUnverifiedOK, r.lesson)

	edit := operation("lessons-update", http.MethodPatch, lessonsPath+"/{id}")
	edit.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusTooManyRequests)
	register(r, edit, AudienceTutor, r.editLesson)

	cancel := operation("lessons-cancel", http.MethodPost, lessonsPath+"/{id}/cancel")
	cancel.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusTooManyRequests)
	register(r, cancel, AudienceTutor, r.cancelLesson)
}

var errQueryUnparsed = errors.New("a calendar bound passed the schema but is not an RFC 3339 instant")

func (r *router) calendar(ctx context.Context, input *calendarInput) (*calendarOutput, error) {
	from, fromErr := time.Parse(time.RFC3339, input.From)
	to, toErr := time.Parse(time.RFC3339, input.To)
	if fromErr != nil || toErr != nil {
		return nil, r.internal(ctx, "calendar.get.failed", errors.Join(errQueryUnparsed, fromErr, toErr))
	}
	query := schedule.CalendarQuery{From: from.UTC(), To: to.UTC()}
	if input.Tutor != "" {
		id, valid := domain.ParseUUID[domain.RelationshipID](input.Tutor)
		if !valid {
			return nil, NewError(ctx, domain.ErrorNotFound)
		}
		query.Tutor = &id
	}
	principal := principalFrom(ctx)
	calendar, err := r.schedule.Calendar(ctx, principal.UserID, principal.Role, query)
	if err != nil {
		return nil, r.scheduleError(ctx, "calendar.get.failed", err)
	}
	body := schemas.Calendar{Items: calendarItemsBody(calendar.Items), Truncated: calendar.Truncated}
	return &calendarOutput{CacheControl: noStore, Body: body}, nil
}

func calendarItemBody(item schedule.CalendarItem) schemas.CalendarItem {
	return schemas.CalendarItem{
		ID: item.ID.String(), Title: item.Title, StartsAt: item.StartsAt, EndsAt: item.EndsAt(), Status: item.Status, Timezone: item.Timezone,
		Audience: schemas.CalendarAudience{Kind: item.Audience.Kind, Label: item.Audience.Label, LabelKind: item.Audience.LabelKind, Colour: item.Audience.Colour},
		Tutor:    tutorLabelBody(item.Tutor),
	}
}

func calendarItemsBody(items []schedule.CalendarItem) []schemas.CalendarItem {
	body := make([]schemas.CalendarItem, 0, len(items))
	for _, item := range items {
		body = append(body, calendarItemBody(item))
	}
	return body
}

var errAudienceUnparsed = errors.New("an audience identifier passed the schema but is not a UUID")

func (r *router) createLessons(ctx context.Context, input *createLessonsInput) (*createdLessonsOutput, error) {
	if err := requireJSON(ctx, input.ContentType); err != nil {
		return nil, err
	}
	request := schedule.NewLesson{
		Title: input.Body.Title, Description: input.Body.Description, Date: input.Body.Date, Start: input.Body.Start,
		DurationMinutes: input.Body.DurationMinutes, Weekdays: input.Body.Weekdays, EndDate: input.Body.EndDate, Count: input.Body.Count,
		AllowOverlap: input.Body.AllowOverlap,
	}
	if input.Body.RelationshipID != nil {
		id, valid := domain.ParseUUID[domain.RelationshipID](*input.Body.RelationshipID)
		if !valid {
			return nil, r.internal(ctx, "lessons.create.failed", errAudienceUnparsed)
		}
		request.RelationshipID = &id
	}
	if input.Body.GroupID != nil {
		id, valid := domain.ParseUUID[domain.GroupID](*input.Body.GroupID)
		if !valid {
			return nil, r.internal(ctx, "lessons.create.failed", errAudienceUnparsed)
		}
		request.GroupID = &id
	}
	if input.IdempotencyKey != "" {
		request.CreateKey = &input.IdempotencyKey
	}
	created, err := r.schedule.CreateLessons(ctx, principalFrom(ctx).UserID, request)
	if err != nil {
		return nil, r.scheduleError(ctx, "lessons.create.failed", err)
	}
	return &createdLessonsOutput{CacheControl: noStore, Body: createdBody(created)}, nil
}

func (r *router) lesson(ctx context.Context, input *lessonInput) (*lessonOutput, error) {
	id, valid := domain.ParseUUID[domain.LessonID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	principal := principalFrom(ctx)
	lesson, err := r.schedule.Lesson(ctx, principal.UserID, principal.Role, id)
	if err != nil {
		return nil, r.scheduleError(ctx, "lessons.get.failed", err)
	}
	return &lessonOutput{CacheControl: noStore, Body: lessonBody(lesson)}, nil
}

func (r *router) editLesson(ctx context.Context, input *editLessonInput) (*createdLessonsOutput, error) {
	if err := requireJSON(ctx, input.ContentType); err != nil {
		return nil, err
	}
	id, valid := domain.ParseUUID[domain.LessonID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	edit := schedule.LessonEdit{
		Scope: input.Body.Scope, Version: input.Body.Version, Date: input.Body.Date, Start: input.Body.Start,
		DurationMinutes: input.Body.DurationMinutes, Title: input.Body.Title, DescriptionSet: input.Body.Description.Set,
		Description:  input.Body.Description.Value,
		AllowOverlap: input.Body.AllowOverlap,
	}
	changed, err := r.schedule.EditLesson(ctx, principalFrom(ctx).UserID, id, edit)
	if err != nil {
		return nil, r.scheduleError(ctx, "lessons.update.failed", err)
	}
	return &createdLessonsOutput{CacheControl: noStore, Body: createdBody(changed)}, nil
}

func (r *router) cancelLesson(ctx context.Context, input *cancelLessonInput) (*createdLessonsOutput, error) {
	if err := requireJSON(ctx, input.ContentType); err != nil {
		return nil, err
	}
	id, valid := domain.ParseUUID[domain.LessonID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	cancellation := schedule.LessonCancellation{Scope: input.Body.Scope, Version: input.Body.Version}
	cancelled, err := r.schedule.CancelLesson(ctx, principalFrom(ctx).UserID, id, cancellation)
	if err != nil {
		return nil, r.scheduleError(ctx, "lessons.cancel.failed", err)
	}
	return &createdLessonsOutput{CacheControl: noStore, Body: createdBody(cancelled)}, nil
}

func createdBody(created schedule.Created) schemas.CreatedLessons {
	body := schemas.CreatedLessons{Lessons: make([]schemas.LessonDetail, 0, len(created.Lessons))}
	if created.Series != nil {
		body.Series = seriesBody(*created.Series)
	}
	for _, lesson := range created.Lessons {
		body.Lessons = append(body.Lessons, lessonBody(lesson))
	}
	return body
}

func seriesBody(series schedule.SeriesView) *schemas.LessonSeries {
	body := &schemas.LessonSeries{
		ID: series.ID.String(), Weekdays: make([]int, 0, len(series.Weekdays)), Start: series.Start.String(), DurationMinutes: series.DurationMinutes,
		Timezone: series.Timezone, FirstDate: series.First.String(), Count: series.Count,
	}
	for _, day := range series.Weekdays {
		body.Weekdays = append(body.Weekdays, int(day))
	}
	if series.EndDate != nil {
		end := series.EndDate.String()
		body.EndDate = &end
	}
	return body
}

func tutorLabelBody(label *schedule.TutorLabel) *schemas.LessonTutor {
	if label == nil {
		return nil
	}
	return &schemas.LessonTutor{Name: label.Name, Colour: label.Colour}
}

func lessonBody(lesson schedule.Lesson) schemas.LessonDetail {
	audience := schemas.LessonAudience{
		Kind: lesson.Audience.Kind, Label: lesson.Audience.Label, LabelKind: lesson.Audience.LabelKind, Colour: lesson.Audience.Colour,
	}
	if lesson.Audience.RelationshipID != nil {
		relationship := lesson.Audience.RelationshipID.String()
		audience.RelationshipID = &relationship
	}
	if lesson.Audience.GroupID != nil {
		group := lesson.Audience.GroupID.String()
		audience.GroupID = &group
	}
	if lesson.Audience.Members != nil {
		audience.Members = make([]schemas.LessonMember, 0, len(lesson.Audience.Members))
		for _, member := range lesson.Audience.Members {
			audience.Members = append(audience.Members, schemas.LessonMember{Label: member.Label, LabelKind: member.LabelKind})
		}
	}
	body := schemas.LessonDetail{
		ID: lesson.ID.String(), Title: lesson.Title, Description: lesson.Description, StartsAt: lesson.StartsAt, EndsAt: lesson.EndsAt(),
		DurationMinutes: lesson.DurationMinutes, Status: lesson.Status, Timezone: lesson.Timezone, Version: lesson.Version,
		CancelledAt: utc(lesson.CancelledAt), OtherTimezone: lesson.OtherTimezone, Audience: audience, Tutor: tutorLabelBody(lesson.Tutor),
	}
	if lesson.SeriesID != nil {
		series := lesson.SeriesID.String()
		body.SeriesID = &series
	}
	if lesson.Series != nil {
		body.Series = seriesBody(*lesson.Series)
	}
	return body
}

func (r *router) scheduleError(ctx context.Context, event string, err error) error {
	var invalid *schedule.ValidationError
	var throttled *schedule.ThrottledError
	var overlap *schedule.OverlapError
	switch {
	case errors.Is(err, schedule.ErrNotFound):
		return NewError(ctx, domain.ErrorNotFound)
	case errors.Is(err, schedule.ErrUnauthenticated):
		return NewError(ctx, domain.ErrorUnauthenticated)
	case errors.Is(err, schedule.ErrAudienceNotActive):
		return NewError(ctx, domain.ErrorAudienceNotActive)
	case errors.Is(err, schedule.ErrConflict), errors.Is(err, schedule.ErrStale):
		return NewError(ctx, domain.ErrorConflict)
	case errors.Is(err, schedule.ErrNotEditable):
		return NewError(ctx, domain.ErrorNotEditable)
	case errors.Is(err, schedule.ErrLessonInPast):
		return NewError(ctx, domain.ErrorLessonInPast)
	case errors.As(err, &overlap):
		refusal := NewError(ctx, domain.ErrorLessonOverlap)
		lessons := make([]schemas.OverlappingLesson, 0, len(overlap.Lessons))
		for _, conflict := range overlap.Lessons {
			lessons = append(lessons, schemas.OverlappingLesson{Title: conflict.Title, StartsAt: conflict.StartsAt, EndsAt: conflict.EndsAt})
		}
		more := overlap.More
		refusal.Body.Details = &schemas.ErrorDetails{Lessons: lessons, More: &more}
		return refusal
	case errors.As(err, &invalid):
		fields := make([]schemas.FieldError, 0, len(invalid.Fields))
		for _, field := range invalid.Fields {
			fields = append(fields, schemas.FieldError{Path: field.Path, Code: string(field.Code)})
		}
		return &Error{status: http.StatusUnprocessableEntity, ErrorEnvelope: schemas.NewErrorEnvelope(domain.ErrorValidationFailed, core.RequestID(ctx), fields)}
	case errors.As(err, &throttled):
		seconds := int64(math.Ceil(throttled.RetryAfter.Seconds()))
		return NewError(ctx, domain.ErrorRateLimited).withHeader("Retry-After", strconv.FormatInt(max(seconds, 1), 10))
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return NewError(ctx, domain.ErrorServiceUnavailable)
	}
	return r.internal(ctx, event, err)
}
