package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/schemas"
	"ovc/backend/internal/services/accounts"
	"ovc/backend/internal/services/chat"
	"ovc/backend/internal/services/homework"
	"ovc/backend/internal/services/notifications"
	"ovc/backend/internal/services/tutoring"
)

const (
	homePath        = "/api/v1/home"
	homeTutorFilter = "query.tutor"
	homeFailed      = "home.get.failed"
)

type homeInput struct {
	Tutor       string `query:"tutor"`
	tutorValues []string
}

func (i *homeInput) Resolve(ctx huma.Context) []error {
	address := ctx.URL()
	i.tutorValues = address.Query()["tutor"]
	return nil
}

type homeOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.Home
}

func (r *router) registerHome() {
	home := operation("home-get", http.MethodGet, homePath)
	home.Errors = errorsOf(http.StatusUnauthorized, http.StatusNotFound)
	register(r, home, AudienceUnverifiedOK, r.home)
}

func (r *router) home(ctx context.Context, input *homeInput) (*homeOutput, error) {
	principal := principalFrom(ctx)
	if principal.Role == domain.RoleTutor {
		if len(input.tutorValues) > 0 {
			return nil, validationRefusal(ctx, homeTutorFilter, domain.FieldNotAllowed)
		}
		body, err := r.tutorHome(ctx, principal)
		if err != nil {
			return nil, err
		}
		return &homeOutput{CacheControl: noStore, Body: schemas.Home{Tutor: body}}, nil
	}
	body, err := r.studentHome(ctx, principal, input.tutorValues)
	if err != nil {
		return nil, err
	}
	return &homeOutput{CacheControl: noStore, Body: schemas.Home{Student: body}}, nil
}

func (r *router) tutorHome(ctx context.Context, principal accounts.Principal) (*schemas.TutorHome, error) {
	tutor := principal.UserID
	_, unconfirmed := authenticatedAudiences[AudienceTutor].refusal(principal)
	day, err := r.schedule.TutorDay(ctx, tutor)
	if err != nil {
		return nil, r.scheduleError(ctx, homeFailed, err)
	}
	body := &schemas.TutorHome{
		TodayLessons: calendarItemsBody(day[:min(len(day), domain.HomeListLimit)]), TodayLessonsCount: int64(len(day)),
	}
	if !unconfirmed {
		if err := r.tutorHomeWork(ctx, tutor, body); err != nil {
			return nil, err
		}
	}
	unread, err := r.homeUnreadChannels(ctx, tutor)
	if err != nil {
		return nil, err
	}
	body.UnreadChannels = unread
	code, err := r.tutoring.Code(ctx, tutor)
	if err != nil {
		return nil, r.internal(ctx, homeFailed, err)
	}
	body.TutorCode = schemas.TutorCode{Code: code.Code.Display(), Active: code.Active}
	checklist, err := r.homeChecklist(ctx, tutor, !unconfirmed)
	if err != nil {
		return nil, err
	}
	body.Checklist = checklist
	updates, err := r.inbox.RecentUpdates(ctx, principal)
	if err != nil {
		return nil, r.flowError(ctx, homeFailed, err)
	}
	body.RecentUpdates = tutorRecentUpdatesBody(updates)
	recordAccessCount(ctx, accessRecentUpdates, int64(len(updates)))
	recordAccessCount(ctx, accessTodayLessons, body.TodayLessonsCount)
	if body.NeedsChecking != nil {
		recordAccessCount(ctx, accessNeedsChecking, body.NeedsChecking.Count)
	}
	if body.UpcomingDeadlines != nil {
		recordAccessCount(ctx, accessUpcomingDeadlines, body.UpcomingDeadlines.Count)
	}
	recordAccessCount(ctx, accessUnreadChannels, body.UnreadChannels.UnreadTotal)
	if body.JoinRequests != nil {
		recordAccessCount(ctx, accessJoinRequests, body.JoinRequests.Count)
	}
	return body, nil
}

func (r *router) tutorHomeWork(ctx context.Context, tutor domain.UserID, body *schemas.TutorHome) error {
	queue, err := r.homework.ReviewQueue(ctx, tutor, homework.ReviewQueueQuery{Limit: domain.HomePanelLimit})
	if err != nil {
		return r.homeworkError(ctx, homeFailed, err)
	}
	checking := &schemas.TutorHomeNeedsChecking{Count: queue.Count, Items: make([]schemas.ReviewQueueItem, 0, len(queue.Items))}
	for _, item := range queue.Items {
		checking.Items = append(checking.Items, reviewQueueItemBody(item))
	}
	body.NeedsChecking = checking
	upcoming, err := r.homework.UpcomingDeadlines(ctx, tutor)
	if err != nil {
		return r.homeworkError(ctx, homeFailed, err)
	}
	deadlines := &schemas.TutorHomeDeadlines{Count: upcoming.Count, Items: make([]schemas.IssueItem, 0, len(upcoming.Items))}
	for _, item := range upcoming.Items {
		deadlines.Items = append(deadlines.Items, issueItemBody(item))
	}
	body.UpcomingDeadlines = deadlines
	requests, err := r.tutoring.TutorRequests(ctx, tutor, tutoring.PageQuery{Limit: domain.HomePanelLimit})
	if err != nil {
		return r.joinRequestError(ctx, homeFailed, err)
	}
	count, err := r.tutoring.VisibleRequestCount(ctx, tutor)
	if err != nil {
		return r.joinRequestError(ctx, homeFailed, err)
	}
	joins := &schemas.TutorHomeJoinRequests{Count: count, Items: make([]schemas.TutorJoinRequest, 0, len(requests.Items)), NextCursor: requests.NextCursor}
	for _, item := range requests.Items {
		joins.Items = append(joins.Items, tutorJoinRequestBody(item))
	}
	body.JoinRequests = joins
	return nil
}

func (r *router) homeUnreadChannels(ctx context.Context, viewer domain.UserID) (schemas.TutorHomeUnreadChannels, error) {
	page, err := r.chat.List(ctx, viewer, chat.Query{Limit: domain.HomeChannelPage})
	if err != nil {
		return schemas.TutorHomeUnreadChannels{}, r.chatError(ctx, homeFailed, err)
	}
	total, err := r.chat.UnreadChannels(ctx, viewer)
	if err != nil {
		return schemas.TutorHomeUnreadChannels{}, r.chatError(ctx, homeFailed, err)
	}
	unread := schemas.TutorHomeUnreadChannels{Items: []schemas.TutorHomeChannel{}, UnreadTotal: total}
	for _, channel := range page.Items {
		if channel.UnreadCount == 0 || len(unread.Items) == domain.HomePanelLimit {
			continue
		}
		unread.Items = append(unread.Items, schemas.TutorHomeChannel{
			ChannelID: channel.ID.String(), Label: channel.Label, LabelKind: channel.LabelKind, Colour: channel.Colour, UnreadCount: channel.UnreadCount,
		})
	}
	return unread, nil
}

func (r *router) homeChecklist(ctx context.Context, tutor domain.UserID, withCard bool) (schemas.TutorHomeChecklist, error) {
	facts, err := r.tutoring.ChecklistFacts(ctx, tutor, withCard)
	if err != nil {
		return schemas.TutorHomeChecklist{}, r.tutoringError(ctx, homeFailed, err)
	}
	lesson, err := r.schedule.TutorHasLessonAhead(ctx, tutor)
	if err != nil {
		return schemas.TutorHomeChecklist{}, r.scheduleError(ctx, homeFailed, err)
	}
	template, err := r.library.HasTemplate(ctx, tutor)
	if err != nil {
		return schemas.TutorHomeChecklist{}, r.internal(ctx, homeFailed, err)
	}
	return schemas.TutorHomeChecklist{
		OnboardingDismissed: facts.Dismissed,
		Steps:               schemas.TutorHomeSteps{Profile: facts.ProfileDone, Student: facts.HasCard, Lesson: lesson, Assignment: template},
	}, nil
}

func (r *router) studentHome(ctx context.Context, principal accounts.Principal, filterValues []string) (*schemas.StudentHome, error) {
	all, err := r.tutoring.MyTutors(ctx, principal)
	if err != nil {
		return nil, r.tutoringError(ctx, homeFailed, err)
	}
	current := make([]tutoring.MyTutor, 0, len(all))
	for _, tutor := range all {
		if tutor.State == domain.RelationshipActive || tutor.State == domain.RelationshipProvisional {
			current = append(current, tutor)
		}
	}
	filter, found := homeFilter(current, filterValues)
	if !found {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	lessons, err := r.schedule.StudentLessons(ctx, principal.UserID, filter)
	if err != nil {
		return nil, r.scheduleError(ctx, homeFailed, err)
	}
	todo, err := r.homework.Assignments(ctx, principal.UserID, homework.AssignmentQuery{
		Group: domain.AssignmentsTodo, Tutor: filter, Limit: domain.HomePanelLimit,
	})
	if err != nil {
		return nil, r.homeworkError(ctx, homeFailed, err)
	}
	checked, err := r.homework.Assignments(ctx, principal.UserID, homework.AssignmentQuery{
		Group: domain.AssignmentsChecked, Tutor: filter, Limit: domain.HomePanelLimit,
	})
	if err != nil {
		return nil, r.homeworkError(ctx, homeFailed, err)
	}
	body := &schemas.StudentHome{
		TodayLessonsCount:  int64(lessons.TodayCount),
		Todo:               schemas.StudentHomeTodo{Count: todo.Counts.Todo, Items: assignmentItemsBody(todo.Items)},
		RecentlyChecked:    assignmentItemsBody(checked.Items),
		NextSevenDays:      calendarItemsBody(lessons.NextSevenDays[:min(len(lessons.NextSevenDays), domain.HomeListLimit)]),
		NextSevenDaysCount: int64(len(lessons.NextSevenDays)),
		Tutors:             homeTutorsBody(current[:min(len(current), domain.HomeListLimit)]),
		TutorsCount:        int64(len(current)),
	}
	if lessons.Next != nil {
		next := calendarItemBody(*lessons.Next)
		body.NextLesson = &next
	}
	updates, err := r.inbox.RecentUpdates(ctx, principal)
	if err != nil {
		return nil, r.flowError(ctx, homeFailed, err)
	}
	body.RecentUpdates = studentRecentUpdatesBody(updates)
	recordAccessCount(ctx, accessRecentUpdates, int64(len(updates)))
	recordAccessCount(ctx, accessTodo, body.Todo.Count)
	recordAccessCount(ctx, accessRecentlyChecked, int64(len(body.RecentlyChecked)))
	recordAccessCount(ctx, accessNextSevenDays, body.NextSevenDaysCount)
	recordAccessCount(ctx, accessTutors, body.TutorsCount)
	return body, nil
}

func homeFilter(current []tutoring.MyTutor, values []string) (*domain.RelationshipID, bool) {
	if len(values) == 0 {
		return nil, true
	}
	if len(values) != 1 {
		return nil, false
	}
	id, valid := domain.ParseUUID[domain.RelationshipID](values[0])
	if !valid {
		return nil, false
	}
	for _, tutor := range current {
		if tutor.RelationshipID == id {
			return &id, true
		}
	}
	return nil, false
}

func homeTutorsBody(tutors []tutoring.MyTutor) []schemas.StudentHomeTutor {
	body := make([]schemas.StudentHomeTutor, 0, len(tutors))
	for _, tutor := range tutors {
		body = append(body, schemas.StudentHomeTutor{
			RelationshipID: tutor.RelationshipID.String(), State: tutor.State, Name: tutor.Tutor.Name, AvatarID: optionalID(tutor.Tutor.AvatarID),
			Colour: tutor.Colour, Subjects: append([]string{}, tutor.Tutor.Subjects...), ChannelID: optionalChannel(tutor.ChannelID),
		})
	}
	return body
}

func tutorRecentUpdatesBody(items []notifications.Item) []schemas.TutorRecentUpdate {
	body := make([]schemas.TutorRecentUpdate, 0, len(items))
	for _, item := range items {
		shown := schemas.TutorRecentUpdate{
			ID: item.ID.String(), Event: item.Event, CreatedAt: item.CreatedAt, Label: item.Label,
			LabelDeleted: item.LabelDeleted, LabelKind: item.LabelKind, Colour: item.Colour,
		}
		if item.Link != nil {
			shown.Link = &schemas.NotificationLink{Kind: item.Link.Kind, ID: item.Link.ID.String()}
		}
		body = append(body, shown)
	}
	return body
}

func studentRecentUpdatesBody(items []notifications.Item) []schemas.StudentRecentUpdate {
	body := make([]schemas.StudentRecentUpdate, 0, len(items))
	for _, item := range items {
		shown := schemas.StudentRecentUpdate{
			ID: item.ID.String(), Event: item.Event, CreatedAt: item.CreatedAt, Label: item.Label,
			LabelDeleted: item.LabelDeleted, LabelKind: item.LabelKind, Colour: item.Colour,
		}
		if item.Link != nil {
			shown.Link = &schemas.NotificationLink{Kind: item.Link.Kind, ID: item.Link.ID.String()}
		}
		body = append(body, shown)
	}
	return body
}
