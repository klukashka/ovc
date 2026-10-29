package api

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/schemas"
	"ovc/backend/internal/services/accounts"
	"ovc/backend/internal/services/schedule"
	"ovc/backend/internal/services/tutoring"
)

const studentsPath = "/api/v1/students"

type tutorCodeOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.TutorCode
}

type tutorSettingsOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.TutorSettings
}

type studentListInput struct {
	State  string `query:"state" enum:"active,invited,ended"`
	Group  string `query:"group" format:"uuid"`
	Q      string `query:"q" maxLength:"254"`
	Limit  int32  `query:"limit" default:"30" minimum:"1" maximum:"100"`
	Cursor string `query:"cursor" maxLength:"128"`
}

type studentListOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.StudentCardPage
}

type createCardInput struct {
	ContentType    string `header:"Content-Type"`
	IdempotencyKey string `header:"Idempotency-Key" maxLength:"200" pattern:"^[!-~]+$"`
	Body           schemas.NewStudentCard
}

type cardInput struct {
	ID string `path:"id"`
}

type updateCardInput struct {
	ID          string `path:"id"`
	ContentType string `header:"Content-Type"`
	Body        schemas.StudentCardPatch
}

type cardOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.StudentCard
}

type cardDetailOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.StudentCardDetail
}

type cardLessonsInput struct {
	ID     string `path:"id"`
	Limit  int32  `query:"limit" default:"30" minimum:"1" maximum:"100"`
	Cursor string `query:"cursor" maxLength:"128"`
}

type cardLessonsOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.CardLessonPage
}

type withdrawOutput struct {
	CacheControl string `header:"Cache-Control"`
}

type regeneratedOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.RegeneratedInvitation
}

type invitationPageInput struct {
	Token string `path:"token"`
}

type invitationPageOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.InvitationPage
}

type acceptInvitationInput struct {
	Token       string `path:"token"`
	ContentType string `header:"Content-Type"`
	Body        schemas.InvitationAcceptRequest
}

type acceptInvitationOutput struct {
	Status       int
	CacheControl string `header:"Cache-Control"`
	Body         schemas.InvitationAccepted
}

type myTutorsOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.MyTutors
}

type stopStudyingInput struct {
	RelationshipID string `path:"relationshipId"`
}

type stopStudyingOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         func(huma.Context)
}

type removedOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.RemovedStudent
}

func (r *router) registerTutoring() {
	code := operation("tutor-code", http.MethodGet, "/api/v1/tutor/code")
	code.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound)
	register(r, code, AudienceTutorUnverifiedOK, r.tutorCode)

	regenerate := operation("tutor-code-regenerate", http.MethodPost, "/api/v1/tutor/code/regenerate")
	regenerate.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusTooManyRequests)
	register(r, regenerate, AudienceTutor, r.regenerateTutorCode)

	settings := operation("tutor-settings", http.MethodGet, "/api/v1/tutor/settings")
	settings.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound)
	register(r, settings, AudienceTutorUnverifiedOK, r.tutorSettings)

	r.registerStudentCards()

	page := operation("invitations-get", http.MethodGet, "/api/v1/invitations/{token}")
	page.Errors = []int{http.StatusNotFound, http.StatusInternalServerError, http.StatusServiceUnavailable}
	register(r, page, AudiencePublic, r.invitationPage)

	accept := operation("invitations-accept", http.MethodPost, "/api/v1/invitations/{token}/accept")
	accept.DefaultStatus = http.StatusAccepted
	accept.Middlewares = append(accept.Middlewares, r.recordClientAddress)
	accept.Responses = map[string]*huma.Response{"200": r.signedInAcceptanceResponse()}
	accept.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusConflict, http.StatusTooManyRequests)
	register(r, accept, AudienceOptionalUser, r.acceptInvitation)

	tutors := operation("tutors-list", http.MethodGet, "/api/v1/tutors")
	tutors.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound)
	register(r, tutors, AudienceStudentUnverifiedOK, r.myTutors)

	stop := operation("tutors-end", http.MethodPost, "/api/v1/tutors/{relationshipId}/end")
	stop.Responses = map[string]*huma.Response{"200": {
		Description: "The relationship is ended",
		Content:     map[string]*huma.MediaType{"application/json": {Schema: &huma.Schema{Ref: "#/components/schemas/MyTutor"}}},
	}, "204": {
		Description: "The provisional link was removed",
		Headers:     map[string]*huma.Param{"Cache-Control": {Schema: &huma.Schema{Type: huma.TypeString}}},
	}}
	stop.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound)
	register(r, stop, AudienceStudentUnverifiedOK, r.stopStudying)

	r.registerJoinRequests()
	r.registerGroups()
}

const signedInAcceptanceSchema = "SignedInAcceptance"

func (r *router) signedInAcceptanceResponse() *huma.Response {
	r.api.OpenAPI().Components.Schemas.Map()[signedInAcceptanceSchema] = &huma.Schema{
		Type:                 huma.TypeObject,
		Properties:           map[string]*huma.Schema{"tutor": {Ref: "#/components/schemas/MyTutor"}},
		Required:             []string{"tutor"},
		AdditionalProperties: false,
	}
	return &huma.Response{
		Description: "Accepted by the signed-in account",
		Headers:     map[string]*huma.Param{"Cache-Control": {Schema: &huma.Schema{Type: huma.TypeString}}},
		Content: map[string]*huma.MediaType{"application/json": {
			Schema: &huma.Schema{Ref: "#/components/schemas/" + signedInAcceptanceSchema},
		}},
	}
}

func (r *router) registerStudentCards() {
	list := operation("students-list", http.MethodGet, studentsPath)
	list.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound)
	register(r, list, AudienceTutor, r.listCards)

	create := operation("students-create", http.MethodPost, studentsPath)
	create.DefaultStatus = http.StatusCreated
	create.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusTooManyRequests)
	register(r, create, AudienceTutor, r.createCard)

	get := operation("students-get", http.MethodGet, studentsPath+"/{id}")
	get.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound)
	register(r, get, AudienceTutor, r.card)

	lessons := operation("students-lessons-list", http.MethodGet, studentsPath+"/{id}/lessons")
	lessons.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound)
	register(r, lessons, AudienceTutor, r.cardLessons)

	update := operation("students-update", http.MethodPatch, studentsPath+"/{id}")
	update.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusTooManyRequests)
	register(r, update, AudienceTutor, r.updateCard)

	withdraw := operation("students-withdraw", http.MethodDelete, studentsPath+"/{id}")
	withdraw.DefaultStatus = http.StatusNoContent
	withdraw.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict)
	register(r, withdraw, AudienceTutor, r.withdrawCard)

	invitation := operation("students-invitation-regenerate", http.MethodPost, studentsPath+"/{id}/invitation")
	invitation.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusTooManyRequests)
	register(r, invitation, AudienceTutor, r.regenerateInvitation)

	send := operation("students-invitation-send", http.MethodPost, studentsPath+"/{id}/invitation/send")
	send.DefaultStatus = http.StatusAccepted
	send.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusTooManyRequests)
	register(r, send, AudienceTutor, r.sendInvitation)

	remove := operation("students-remove", http.MethodPost, studentsPath+"/{id}/remove")
	remove.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict)
	register(r, remove, AudienceTutor, r.removeStudent)

	end := operation("students-end", http.MethodPost, studentsPath+"/{id}/end")
	end.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict)
	register(r, end, AudienceTutor, r.endRelationship)
}

func (r *router) tutorCode(ctx context.Context, _ *struct{}) (*tutorCodeOutput, error) {
	current, err := r.tutoring.Code(ctx, principalFrom(ctx).UserID)
	if err != nil {
		return nil, r.internal(ctx, "tutor.code.failed", err)
	}
	return tutorCodeBody(current), nil
}

func (r *router) regenerateTutorCode(ctx context.Context, _ *struct{}) (*tutorCodeOutput, error) {
	regenerated, err := r.tutoring.RegenerateCode(ctx, principalFrom(ctx).UserID)
	var throttled *tutoring.ThrottledError
	switch {
	case err == nil:
		return tutorCodeBody(regenerated), nil
	case errors.As(err, &throttled):
		seconds := int64(math.Ceil(throttled.RetryAfter.Seconds()))
		return nil, NewError(ctx, domain.ErrorRateLimited).withHeader("Retry-After", strconv.FormatInt(max(seconds, 1), 10))
	case errors.Is(err, accounts.ErrUnauthenticated):
		return nil, NewError(ctx, domain.ErrorUnauthenticated)
	}
	return nil, r.internal(ctx, "tutor.code.regenerate.failed", err)
}

func (r *router) tutorSettings(ctx context.Context, _ *struct{}) (*tutorSettingsOutput, error) {
	settings, err := r.tutoring.Settings(ctx, principalFrom(ctx).UserID)
	if err != nil {
		return nil, r.tutoringError(ctx, "tutor.settings.failed", err)
	}
	return &tutorSettingsOutput{CacheControl: noStore, Body: schemas.TutorSettings{
		FileQuotaBytes: settings.FileQuotaBytes, FileUsageBytes: settings.FileUsageBytes,
	}}, nil
}

func tutorCodeBody(code tutoring.TutorCode) *tutorCodeOutput {
	return &tutorCodeOutput{CacheControl: noStore, Body: schemas.TutorCode{Code: code.Code.Display(), Active: code.Active}}
}

func (r *router) listCards(ctx context.Context, input *studentListInput) (*studentListOutput, error) {
	query := tutoring.CardQuery{Filter: domain.CardFilter(input.State), Search: input.Q, Cursor: input.Cursor, Limit: input.Limit}
	if input.Group != "" {
		group, valid := domain.ParseUUID[domain.GroupID](input.Group)
		if !valid {
			return nil, r.tutoringError(ctx, "students.list.failed", &tutoring.ValidationError{
				Fields: []tutoring.FieldProblem{{Path: "query.group", Code: domain.FieldInvalid}},
			})
		}
		query.Group = &group
	}
	page, err := r.tutoring.ListCards(ctx, principalFrom(ctx).UserID, query)
	if err != nil {
		return nil, r.tutoringError(ctx, "students.list.failed", err)
	}
	ids := make([]domain.RelationshipID, 0, len(page.Items))
	for _, card := range page.Items {
		ids = append(ids, card.ID)
	}
	next, err := r.schedule.CardsNextLesson(ctx, principalFrom(ctx).UserID, ids)
	if err != nil {
		return nil, r.scheduleError(ctx, "students.list.failed", err)
	}
	body := schemas.StudentCardPage{Items: make([]schemas.StudentCardSummary, 0, len(page.Items)), NextCursor: page.NextCursor}
	for _, card := range page.Items {
		summary := schemas.StudentCardSummary{CardFields: cardFieldsBody(card)}
		if lesson, found := next[card.ID]; found {
			item := calendarItemBody(lesson)
			summary.NextLesson = &item
		}
		body.Items = append(body.Items, summary)
	}
	return &studentListOutput{CacheControl: noStore, Body: body}, nil
}

func (r *router) createCard(ctx context.Context, input *createCardInput) (*cardOutput, error) {
	if err := requireJSON(ctx, input.ContentType); err != nil {
		return nil, err
	}
	request := tutoring.NewCard{DisplayName: input.Body.DisplayName, Email: input.Body.Email}
	if input.IdempotencyKey != "" {
		request.CreateKey = &input.IdempotencyKey
	}
	card, err := r.tutoring.CreateCard(ctx, principalFrom(ctx).UserID, request)
	if err != nil {
		return nil, r.tutoringError(ctx, "students.create.failed", err)
	}
	return &cardOutput{CacheControl: noStore, Body: cardBody(card)}, nil
}

func (r *router) card(ctx context.Context, input *cardInput) (*cardDetailOutput, error) {
	id, valid := domain.ParseUUID[domain.RelationshipID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	tutor := principalFrom(ctx).UserID
	card, err := r.tutoring.Card(ctx, tutor, id)
	if err != nil {
		return nil, r.tutoringError(ctx, "students.get.failed", err)
	}
	next, err := r.schedule.CardNextLessons(ctx, tutor, id)
	if err != nil {
		return nil, r.scheduleError(ctx, "students.get.failed", err)
	}
	return &cardDetailOutput{CacheControl: noStore, Body: schemas.StudentCardDetail{
		StudentCard: cardBody(card), NextLessons: calendarItemsBody(next), ChannelID: optionalChannel(card.ChannelID),
	}}, nil
}

func (r *router) cardLessons(ctx context.Context, input *cardLessonsInput) (*cardLessonsOutput, error) {
	id, valid := domain.ParseUUID[domain.RelationshipID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	page, err := r.schedule.CardHistory(ctx, principalFrom(ctx).UserID, id, schedule.HistoryQuery{Cursor: input.Cursor, Limit: input.Limit})
	if err != nil {
		return nil, r.scheduleError(ctx, "students.lessons.list.failed", err)
	}
	return &cardLessonsOutput{CacheControl: noStore, Body: schemas.CardLessonPage{Items: calendarItemsBody(page.Items), NextCursor: page.NextCursor}}, nil
}

func (r *router) updateCard(ctx context.Context, input *updateCardInput) (*cardOutput, error) {
	if err := requireJSON(ctx, input.ContentType); err != nil {
		return nil, err
	}
	id, valid := domain.ParseUUID[domain.RelationshipID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	change := tutoring.CardChange{
		DisplayName: input.Body.DisplayName, Notes: input.Body.Notes,
		Email: tutoring.EmailChange{Set: input.Body.Email.Set, Value: input.Body.Email.Value},
	}
	card, err := r.tutoring.UpdateCard(ctx, principalFrom(ctx).UserID, id, change)
	if err != nil {
		return nil, r.tutoringError(ctx, "students.update.failed", err)
	}
	return &cardOutput{CacheControl: noStore, Body: cardBody(card)}, nil
}

func (r *router) withdrawCard(ctx context.Context, input *cardInput) (*withdrawOutput, error) {
	id, valid := domain.ParseUUID[domain.RelationshipID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	if err := r.tutoring.WithdrawCard(ctx, principalFrom(ctx).UserID, id); err != nil {
		return nil, r.tutoringError(ctx, "students.withdraw.failed", err)
	}
	return &withdrawOutput{CacheControl: noStore}, nil
}

func (r *router) regenerateInvitation(ctx context.Context, input *cardInput) (*regeneratedOutput, error) {
	id, valid := domain.ParseUUID[domain.RelationshipID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	issued, err := r.tutoring.RegenerateInvitation(ctx, principalFrom(ctx).UserID, id)
	if err != nil {
		return nil, r.tutoringError(ctx, "students.invitation.regenerate.failed", err)
	}
	body := schemas.RegeneratedInvitation{Invitation: schemas.IssuedInvitation{Status: issued.Status, ExpiresAt: issued.ExpiresAt.UTC()}}
	if issued.URL != nil {
		body.Invitation.URL = *issued.URL
	}
	return &regeneratedOutput{CacheControl: noStore, Body: body}, nil
}

func (r *router) sendInvitation(ctx context.Context, input *cardInput) (*acceptedOutput, error) {
	id, valid := domain.ParseUUID[domain.RelationshipID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	if err := r.tutoring.SendInvitation(ctx, principalFrom(ctx).UserID, id); err != nil {
		return nil, r.tutoringError(ctx, "students.invitation.send.failed", err)
	}
	return &acceptedOutput{CacheControl: noStore}, nil
}

func (r *router) invitationPage(ctx context.Context, input *invitationPageInput) (*invitationPageOutput, error) {
	tutor, err := r.tutoring.InvitationPage(ctx, input.Token)
	if err != nil {
		return nil, r.tutoringError(ctx, "invitations.get.failed", err)
	}
	body := schemas.InvitationPage{Tutor: schemas.PublicTutor{Name: tutor.Name, AvatarID: optionalID(tutor.AvatarID), Subjects: append([]string{}, tutor.Subjects...)}}
	return &invitationPageOutput{CacheControl: noStore, Body: body}, nil
}

func (r *router) removeStudent(ctx context.Context, input *cardInput) (*removedOutput, error) {
	id, valid := domain.ParseUUID[domain.RelationshipID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	card, err := r.tutoring.RemoveStudent(ctx, principalFrom(ctx).UserID, id)
	if err != nil {
		return nil, r.tutoringError(ctx, "students.remove.failed", err)
	}
	return &removedOutput{CacheControl: noStore, Body: schemas.RemovedStudent{NewCard: cardBody(card)}}, nil
}

func (r *router) endRelationship(ctx context.Context, input *cardInput) (*cardOutput, error) {
	id, valid := domain.ParseUUID[domain.RelationshipID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	card, err := r.tutoring.EndByTutor(ctx, principalFrom(ctx).UserID, id)
	if err != nil {
		return nil, r.tutoringError(ctx, "students.end.failed", err)
	}
	return &cardOutput{CacheControl: noStore, Body: cardBody(card)}, nil
}

func (r *router) stopStudying(ctx context.Context, input *stopStudyingInput) (*stopStudyingOutput, error) {
	id, valid := domain.ParseUUID[domain.RelationshipID](input.RelationshipID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	ending, err := r.tutoring.StopStudying(ctx, principalFrom(ctx), id)
	switch {
	case errors.Is(err, accounts.ErrUnauthenticated):
		return nil, r.flowError(ctx, "tutors.end.failed", err)
	case err != nil:
		return nil, r.tutoringError(ctx, "tutors.end.failed", err)
	case ending.Removed:
		return &stopStudyingOutput{CacheControl: noStore, Body: func(response huma.Context) { response.SetStatus(http.StatusNoContent) }}, nil
	}
	tutor := myTutorBody(ending.Tutor)
	return &stopStudyingOutput{CacheControl: noStore, Body: func(response huma.Context) { r.writeTutor(response, tutor) }}, nil
}

func (r *router) writeTutor(response huma.Context, body schemas.MyTutor) {
	response.SetHeader("Content-Type", "application/json")
	response.SetStatus(http.StatusOK)
	if err := huma.DefaultJSONFormat.Marshal(response.BodyWriter(), body); err != nil {
		r.logger.LogAttrs(response.Context(), slog.LevelWarn, "http.response.write_failed", slog.String("kind", core.ErrorKind(err)))
	}
}

func (r *router) acceptInvitation(ctx context.Context, input *acceptInvitationInput) (*acceptInvitationOutput, error) {
	if err := requireJSON(ctx, input.ContentType); err != nil {
		return nil, err
	}
	if principal, signedIn := optionalPrincipal(ctx); signedIn {
		return r.acceptSignedIn(ctx, principal, input.Token, input.Body)
	}
	body := input.Body
	signUp := accounts.InvitationSignUp{
		Name: body.Name, Email: body.Email, Password: body.Password, Language: body.Language, Timezone: body.Timezone, Consent: body.Consent,
	}
	if err := r.auth.Accounts.RegisterThroughInvitation(ctx, input.Token, signUp, clientAddress(ctx)); err != nil {
		return nil, r.flowError(ctx, "invitations.accept.failed", err)
	}
	return &acceptInvitationOutput{Status: http.StatusAccepted, CacheControl: noStore}, nil
}

func (r *router) acceptSignedIn(
	ctx context.Context, principal accounts.Principal, token string, body schemas.InvitationAcceptRequest,
) (*acceptInvitationOutput, error) {
	if err := signUpFieldsWithBearer(ctx, body); err != nil {
		return nil, err
	}
	tutor, err := r.tutoring.AcceptSignedIn(ctx, principal, token)
	switch {
	case err == nil:
		accepted := myTutorBody(tutor)
		return &acceptInvitationOutput{Status: http.StatusOK, CacheControl: noStore, Body: schemas.InvitationAccepted{Tutor: &accepted}}, nil
	case errors.Is(err, tutoring.ErrRoleMismatch):
		return nil, NewError(ctx, domain.ErrorRoleMismatch)
	case errors.Is(err, tutoring.ErrEmailNotVerified):
		return nil, NewError(ctx, domain.ErrorEmailNotVerified)
	case errors.Is(err, accounts.ErrInvitationInvalid), errors.Is(err, accounts.ErrInvitationEmailMismatch), errors.Is(err, accounts.ErrUnauthenticated):
		return nil, r.flowError(ctx, "invitations.accept.failed", err)
	}
	return nil, r.tutoringError(ctx, "invitations.accept.failed", err)
}

func signUpFieldsWithBearer(ctx context.Context, body schemas.InvitationAcceptRequest) error {
	sent := []struct {
		path    string
		present bool
	}{
		{"body.name", body.Name != nil}, {"body.email", body.Email != nil}, {"body.password", body.Password != nil},
		{"body.language", body.Language != nil}, {"body.timezone", body.Timezone != nil}, {"body.consent", body.Consent != nil},
	}
	var fields []schemas.FieldError
	for _, field := range sent {
		if field.present {
			fields = append(fields, schemas.FieldError{Path: field.path, Code: string(domain.FieldNotAllowed)})
		}
	}
	if len(fields) > 0 {
		return &Error{status: http.StatusUnprocessableEntity, ErrorEnvelope: schemas.NewErrorEnvelope(domain.ErrorValidationFailed, core.RequestID(ctx), fields)}
	}
	return nil
}

func (r *router) myTutors(ctx context.Context, _ *struct{}) (*myTutorsOutput, error) {
	tutors, err := r.tutoring.MyTutors(ctx, principalFrom(ctx))
	if err != nil {
		return nil, r.tutoringError(ctx, "tutors.list.failed", err)
	}
	body := schemas.MyTutors{Tutors: make([]schemas.MyTutor, 0, len(tutors))}
	for _, tutor := range tutors {
		body.Tutors = append(body.Tutors, myTutorBody(tutor))
	}
	return &myTutorsOutput{CacheControl: noStore, Body: body}, nil
}

func myTutorBody(tutor tutoring.MyTutor) schemas.MyTutor {
	return schemas.MyTutor{
		RelationshipID: tutor.RelationshipID.String(), State: tutor.State, Colour: tutor.Colour,
		Tutor: schemas.MyTutorProfile{
			Name: tutor.Tutor.Name, AvatarID: optionalID(tutor.Tutor.AvatarID), Subjects: append([]string{}, tutor.Tutor.Subjects...), Bio: tutor.Tutor.Bio,
		},
		EndedAt: utc(tutor.EndedAt), EndedBy: tutor.EndedBy, AccountClosedUntil: utc(tutor.AccountClosedUntil),
		ChannelID: optionalChannel(tutor.ChannelID),
	}
}

func optionalChannel(id *domain.ChannelID) *string {
	if id == nil {
		return nil
	}
	text := id.String()
	return &text
}

func (r *router) tutoringError(ctx context.Context, event string, err error) error {
	var invalid *tutoring.ValidationError
	var exists *tutoring.StudentExistsError
	var throttled *tutoring.ThrottledError
	switch {
	case errors.Is(err, tutoring.ErrCardNotFound), errors.Is(err, tutoring.ErrInvitationNotFound), errors.Is(err, tutoring.ErrGroupNotFound):
		return NewError(ctx, domain.ErrorNotFound)
	case errors.As(err, &exists):
		existing := exists.Existing.String()
		refusal := NewError(ctx, domain.ErrorStudentExists)
		refusal.Body.Details = &schemas.ErrorDetails{StudentID: &existing}
		return refusal
	case errors.Is(err, accounts.ErrUnauthenticated):
		return NewError(ctx, domain.ErrorUnauthenticated)
	case errors.Is(err, tutoring.ErrNotEditable):
		return NewError(ctx, domain.ErrorNotEditable)
	case errors.Is(err, tutoring.ErrRelationshipNotActive):
		return NewError(ctx, domain.ErrorRelationshipNotActive)
	case errors.Is(err, tutoring.ErrRelationshipNotProvisional):
		return NewError(ctx, domain.ErrorRelationshipNotProvisional)
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

func cardFieldsBody(card tutoring.Card) schemas.CardFields {
	summary := schemas.CardFields{
		ID: card.ID.String(), State: card.State, DisplayName: card.DisplayName, Email: card.Email,
		Groups: make([]schemas.CardGroup, 0, len(card.Groups)), LinkedAt: utc(card.LinkedAt), EndedAt: utc(card.EndedAt),
		EndedBy: card.EndedBy, CreatedAt: card.CreatedAt.UTC(),
	}
	if card.Account != nil {
		summary.Account = &schemas.CardAccount{
			Name: card.Account.Name, Email: card.Account.Email, AvatarID: optionalID(card.Account.AvatarID), EmailConfirmed: card.Account.EmailConfirmed,
		}
	}
	for _, group := range card.Groups {
		summary.Groups = append(summary.Groups, schemas.CardGroup{ID: group.ID.String(), Name: group.Name, Colour: group.Colour})
	}
	return summary
}

func cardBody(card tutoring.Card) schemas.StudentCard {
	body := schemas.StudentCard{CardFields: cardFieldsBody(card), Notes: card.Notes}
	if card.Invitation != nil {
		body.Invitation = &schemas.CardInvitation{URL: card.Invitation.URL, ExpiresAt: card.Invitation.ExpiresAt.UTC(), Status: card.Invitation.Status}
	}
	return body
}

func utc(instant *time.Time) *time.Time {
	if instant == nil {
		return nil
	}
	converted := instant.UTC()
	return &converted
}

func optionalID(id *domain.AttachmentID) *string {
	if id == nil {
		return nil
	}
	text := id.String()
	return &text
}
