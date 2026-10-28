package notifications

import (
	"context"
	"encoding/base64"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/accounts"
)

const (
	cursorSeparator = "_"
	fieldCursor     = "query.cursor"
	fieldIDs        = "body.ids"
	fieldThrough    = "body.through"
)

var ErrNotificationNotFound = errors.New("the notification is not one of the caller's rows")

type InboxDependencies struct {
	Store  *repositories.Store
	Clock  clock.Source
	Logger *slog.Logger
}

type Inbox struct {
	deps InboxDependencies
}

func NewInbox(dependencies InboxDependencies) *Inbox {
	return &Inbox{deps: dependencies}
}

type Link struct {
	Kind domain.NotificationLinkKind
	ID   domain.EntityID
}

type Item struct {
	ID           domain.NotificationID
	Event        domain.NotificationEvent
	CreatedAt    time.Time
	ReadAt       *time.Time
	Label        *string
	LabelDeleted bool
	LabelKind    *domain.LabelKind
	Colour       *domain.Colour
	Link         *Link
}

type Page struct {
	Items       []Item
	NextCursor  *string
	UnreadCount int64
}

type PageQuery struct {
	Cursor string
	Limit  int32
}

type ReadRequest struct {
	IDs     []string
	Through *string
}

func (i *Inbox) List(ctx context.Context, principal accounts.Principal, query PageQuery) (Page, error) {
	after, err := parseCursor(query.Cursor)
	if err != nil {
		return Page{}, err
	}
	now, err := i.deps.Clock.Instant(ctx)
	if err != nil {
		return Page{}, err
	}
	page := Page{Items: []Item{}}
	err = i.deps.Store.WithReadTx(ctx, func(repos repositories.Repositories) error {
		rows, err := repos.Notifications.Of(ctx, principal.UserID, after, query.Limit+1)
		if err != nil {
			return err
		}
		if len(rows) > int(query.Limit) {
			rows = rows[:query.Limit]
			last := rows[len(rows)-1]
			next := encodeCursor(last.CreatedAt, last.ID)
			page.NextCursor = &next
		}
		subjects, err := loadSubjects(ctx, repos, principal, rows)
		if err != nil {
			return err
		}
		for _, row := range rows {
			page.Items = append(page.Items, subjects.item(principal, row, now))
		}
		page.UnreadCount, err = repos.Notifications.UnreadCount(ctx, principal.UserID)
		return err
	})
	return page, err
}

func (i *Inbox) MarkRead(ctx context.Context, principal accounts.Principal, request ReadRequest) (int64, error) {
	ids, through, err := parseReadRequest(request)
	if err != nil {
		return 0, err
	}
	now, err := i.deps.Clock.Instant(ctx)
	if err != nil {
		return 0, err
	}
	var unread, marked int64
	err = i.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		if err := accounts.LockActingAccount(ctx, repos, principal, now, principal.State); err != nil {
			return err
		}
		subject, err := markOwn(ctx, repos, principal.UserID, readTarget{ids: ids, idsGiven: request.IDs != nil, through: through}, now, &marked)
		if err != nil {
			return err
		}
		if marked > 0 {
			if err := repos.Signals.Entity(ctx, []domain.UserID{principal.UserID}, domain.EntityNotification, domain.EntityID(subject)); err != nil {
				return err
			}
		}
		unread, err = repos.Notifications.UnreadCount(ctx, principal.UserID)
		return err
	})
	if err != nil {
		return 0, err
	}
	i.deps.Logger.LogAttrs(ctx, slog.LevelDebug, "notifications.read", slog.Int64("marked", marked))
	return unread, nil
}

type readTarget struct {
	ids      []domain.NotificationID
	idsGiven bool
	through  *domain.NotificationID
}

func markOwn(
	ctx context.Context, repos repositories.Repositories, user domain.UserID, target readTarget, now time.Time, marked *int64,
) (domain.NotificationID, error) {
	ids, through := target.ids, target.through
	if len(ids) > 0 {
		own, err := repos.Notifications.CountOwn(ctx, user, ids)
		if err != nil {
			return domain.NotificationID{}, err
		}
		if own != int64(len(ids)) {
			return domain.NotificationID{}, ErrNotificationNotFound
		}
	}
	var position *repositories.NotificationPosition
	if through != nil {
		found, err := repos.Notifications.Own(ctx, user, *through)
		if errors.Is(err, repositories.ErrNotFound) {
			return domain.NotificationID{}, ErrNotificationNotFound
		}
		if err != nil {
			return domain.NotificationID{}, err
		}
		position = &found
	}
	switch {
	case target.idsGiven && position != nil:
		return domain.NotificationID{}, readProblem(fieldThrough, domain.FieldNotAllowed)
	case !target.idsGiven && position == nil:
		return domain.NotificationID{}, readProblem(fieldIDs, domain.FieldRequired)
	case position == nil && len(ids) == 0:
		return domain.NotificationID{}, readProblem(fieldIDs, domain.FieldInvalid)
	case position != nil:
		var err error
		*marked, err = repos.Notifications.MarkReadThrough(ctx, user, *position, now)
		return position.ID, err
	}
	var err error
	*marked, err = repos.Notifications.MarkRead(ctx, user, ids, now)
	return ids[0], err
}

func parseReadRequest(request ReadRequest) ([]domain.NotificationID, *domain.NotificationID, error) {
	ids := make([]domain.NotificationID, 0, len(request.IDs))
	seen := make(map[domain.NotificationID]bool, len(request.IDs))
	for _, text := range request.IDs {
		id, valid := domain.ParseUUID[domain.NotificationID](text)
		if !valid {
			return nil, nil, ErrNotificationNotFound
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if len(request.IDs) > domain.NotificationReadMaximum {
		return nil, nil, readProblem(fieldIDs, domain.FieldInvalid)
	}
	if request.Through == nil {
		return ids, nil, nil
	}
	id, valid := domain.ParseUUID[domain.NotificationID](*request.Through)
	if !valid {
		return nil, nil, ErrNotificationNotFound
	}
	return ids, &id, nil
}

func readProblem(path string, code domain.FieldCode) error {
	return &accounts.ValidationError{Fields: []accounts.FieldProblem{{Path: path, Code: code}}}
}

type subjects struct {
	relationships map[domain.RelationshipID]repositories.NotificationRelationship
	requests      map[domain.JoinRequestID]repositories.NotificationJoinRequest
	lessons       map[domain.LessonID]string
	groups        map[domain.GroupID]repositories.NotificationGroup
}

func loadSubjects(ctx context.Context, repos repositories.Repositories, principal accounts.Principal, rows []repositories.Notification) (subjects, error) {
	var relationshipIDs []domain.RelationshipID
	var requestIDs []domain.JoinRequestID
	var lessonIDs []domain.LessonID
	var groupIDs []domain.GroupID
	for _, row := range rows {
		if row.RelationshipID != nil {
			relationshipIDs = append(relationshipIDs, *row.RelationshipID)
		}
		if subjectIsRequest(row.Event) {
			requestIDs = append(requestIDs, domain.JoinRequestID(row.SubjectID))
		}
		if subjectIsLesson(row.Event) {
			lessonIDs = append(lessonIDs, domain.LessonID(row.SubjectID))
		}
		if row.Event == domain.NotificationGroupArchived {
			groupIDs = append(groupIDs, domain.GroupID(row.SubjectID))
		}
	}
	loaded := subjects{
		relationships: map[domain.RelationshipID]repositories.NotificationRelationship{},
		requests:      map[domain.JoinRequestID]repositories.NotificationJoinRequest{},
		lessons:       map[domain.LessonID]string{},
		groups:        map[domain.GroupID]repositories.NotificationGroup{},
	}
	if err := loaded.loadLessons(ctx, repos, principal, lessonIDs); err != nil {
		return subjects{}, err
	}
	if principal.Role == domain.RoleStudent && len(groupIDs) > 0 {
		groups, err := repos.Notifications.Groups(ctx, principal.UserID, groupIDs)
		if err != nil {
			return subjects{}, err
		}
		for _, group := range groups {
			loaded.groups[group.ID] = group
		}
	}
	if len(relationshipIDs) > 0 {
		relationships, err := repos.Notifications.Relationships(ctx, relationshipIDs)
		if err != nil {
			return subjects{}, err
		}
		for _, relationship := range relationships {
			loaded.relationships[relationship.ID] = relationship
		}
	}
	if len(requestIDs) > 0 {
		requests, err := repos.Notifications.JoinRequests(ctx, requestIDs)
		if err != nil {
			return subjects{}, err
		}
		for _, request := range requests {
			loaded.requests[request.ID] = request
		}
	}
	return loaded, nil
}

func (s subjects) loadLessons(ctx context.Context, repos repositories.Repositories, principal accounts.Principal, ids []domain.LessonID) error {
	if len(ids) == 0 {
		return nil
	}
	var visible []repositories.NotificationLesson
	var err error
	if principal.Role == domain.RoleTutor {
		visible, err = repos.Lessons.TutorNotificationLessons(ctx, principal.UserID, ids)
	} else {
		visible, err = repos.Lessons.StudentNotificationLessons(ctx, principal.UserID, ids)
	}
	if err != nil {
		return err
	}
	for _, lesson := range visible {
		s.lessons[lesson.ID] = lesson.Title
	}
	return nil
}

func subjectIsRequest(event domain.NotificationEvent) bool {
	switch event {
	case domain.NotificationJoinRequestCreated, domain.NotificationJoinRequestAccepted, domain.NotificationJoinRequestDeclined:
		return true
	case domain.NotificationInvitationAccepted, domain.NotificationRelationshipEnded, domain.NotificationLessonReminder1h,
		domain.NotificationLessonMoved, domain.NotificationLessonCancelled, domain.NotificationSeriesChanged, domain.NotificationGroupArchived:
		return false
	}
	return false
}

func subjectIsLesson(event domain.NotificationEvent) bool {
	switch event {
	case domain.NotificationLessonReminder1h, domain.NotificationLessonMoved, domain.NotificationLessonCancelled, domain.NotificationSeriesChanged:
		return true
	case domain.NotificationInvitationAccepted, domain.NotificationJoinRequestCreated, domain.NotificationJoinRequestAccepted,
		domain.NotificationJoinRequestDeclined, domain.NotificationRelationshipEnded, domain.NotificationGroupArchived:
		return false
	}
	return false
}

func (s subjects) item(principal accounts.Principal, row repositories.Notification, now time.Time) Item {
	item := Item{ID: row.ID, Event: row.Event, CreatedAt: row.CreatedAt, ReadAt: row.ReadAt}
	switch row.Event {
	case domain.NotificationInvitationAccepted, domain.NotificationRelationshipEnded:
		if principal.Role == domain.RoleTutor {
			s.tutorCard(principal.UserID, row, &item)
		} else {
			s.studentRelationship(principal.UserID, row, &item)
		}
	case domain.NotificationJoinRequestCreated:
		s.tutorRequest(principal.UserID, row, now, &item)
	case domain.NotificationJoinRequestAccepted:
		s.studentRelationship(principal.UserID, row, &item)
	case domain.NotificationJoinRequestDeclined:
		if request, found := s.requests[domain.JoinRequestID(row.SubjectID)]; found && request.StudentID == principal.UserID {
			item.Link = &Link{Kind: domain.LinkMyTutors, ID: domain.EntityID(request.ID)}
		}
	case domain.NotificationLessonReminder1h, domain.NotificationLessonMoved, domain.NotificationLessonCancelled, domain.NotificationSeriesChanged:
		s.lessonRow(principal, row, &item)
	case domain.NotificationGroupArchived:
		s.archivedGroupRow(principal, row, &item)
	}
	return item
}

func (s subjects) archivedGroupRow(principal accounts.Principal, row repositories.Notification, item *Item) {
	group, found := s.groups[domain.GroupID(row.SubjectID)]
	if principal.Role != domain.RoleStudent || !found || group.TutorID != row.TutorID {
		return
	}
	if group.Member {
		name := group.Name
		item.Label = &name
	} else {
		kind := domain.LabelGroup
		item.LabelKind = &kind
	}
	s.studentColour(principal.UserID, row, item)
}

func (s subjects) studentColour(student domain.UserID, row repositories.Notification, item *Item) {
	if row.RelationshipID == nil {
		return
	}
	if link, found := s.relationships[*row.RelationshipID]; found && link.StudentID != nil && *link.StudentID == student && listedInMyTutors(link.State) {
		item.Colour = link.Colour
	}
}

func (s subjects) lessonRow(principal accounts.Principal, row repositories.Notification, item *Item) {
	title, visible := s.lessons[domain.LessonID(row.SubjectID)]
	if !visible {
		return
	}
	item.Label = &title
	item.Link = &Link{Kind: domain.NotificationLessonLink, ID: row.SubjectID}
	if principal.Role == domain.RoleTutor {
		return
	}
	s.studentColour(principal.UserID, row, item)
}

func (s subjects) tutorCard(tutor domain.UserID, row repositories.Notification, item *Item) {
	if row.RelationshipID == nil {
		return
	}
	card, found := s.relationships[*row.RelationshipID]
	if !found || card.TutorID != tutor {
		return
	}
	name := card.DisplayName
	item.Label = &name
	if card.State != domain.RelationshipDiscarded {
		item.Link = &Link{Kind: domain.LinkStudentCard, ID: domain.EntityID(card.ID)}
	}
}

func (s subjects) tutorRequest(tutor domain.UserID, row repositories.Notification, now time.Time, item *Item) {
	request, found := s.requests[domain.JoinRequestID(row.SubjectID)]
	if !found || request.TutorID != tutor {
		return
	}
	usable := request.StudentState != domain.AccountBlocked && request.StudentState != domain.AccountDeleted
	visible := usable && request.State == domain.JoinRequestPending && request.Suppressed == domain.SuppressionNone && request.ExpiresAt.After(now)
	switch {
	case request.State == domain.JoinRequestAccepted && request.StudentState == domain.AccountDeleted:
		item.LabelDeleted = true
	case request.State == domain.JoinRequestAccepted && usable:
		item.Label = request.CardName
	case visible:
		name := request.StudentName
		item.Label = &name
		item.Link = &Link{Kind: domain.LinkJoinRequests, ID: domain.EntityID(request.ID)}
	}
}

func (s subjects) studentRelationship(student domain.UserID, row repositories.Notification, item *Item) {
	if row.RelationshipID == nil {
		return
	}
	link, found := s.relationships[*row.RelationshipID]
	if !found || link.StudentID == nil || *link.StudentID != student || !listedInMyTutors(link.State) {
		return
	}
	name := link.TutorName
	item.Label = &name
	item.Colour = link.Colour
	item.Link = &Link{Kind: domain.LinkMyTutors, ID: domain.EntityID(link.ID)}
}

func listedInMyTutors(state domain.RelationshipState) bool {
	switch state {
	case domain.RelationshipActive, domain.RelationshipProvisional, domain.RelationshipEnded:
		return true
	case domain.RelationshipInvited, domain.RelationshipReclaimPending, domain.RelationshipDiscarded:
		return false
	}
	return false
}

func (i *Inbox) Cleanup(ctx context.Context, now time.Time) (int64, error) {
	var deleted int64
	for range cleanupBatches {
		removed, err := i.deps.Store.Repositories().Notifications.DeleteBefore(ctx, now.Add(-domain.NotificationRetention), cleanupBatchRows)
		if err != nil {
			return deleted, err
		}
		deleted += removed
		if removed < cleanupBatchRows {
			break
		}
	}
	return deleted, nil
}

const (
	cleanupBatches   = 10
	cleanupBatchRows = 1000
)

func encodeCursor(createdAt time.Time, id domain.NotificationID) string {
	text := strconv.FormatInt(createdAt.UnixMicro(), 10) + cursorSeparator + id.String()
	return base64.RawURLEncoding.EncodeToString([]byte(text))
}

func parseCursor(text string) (*repositories.NotificationCursor, error) {
	if text == "" {
		return nil, nil
	}
	invalid := readProblem(fieldCursor, domain.FieldInvalid)
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(text)
	if err != nil {
		return nil, invalid
	}
	micros, idText, found := strings.Cut(string(decoded), cursorSeparator)
	if !found {
		return nil, invalid
	}
	instant, err := strconv.ParseInt(micros, 10, 64)
	if err != nil {
		return nil, invalid
	}
	id, valid := domain.ParseUUID[domain.NotificationID](idText)
	if !valid {
		return nil, invalid
	}
	return &repositories.NotificationCursor{CreatedAt: time.UnixMicro(instant).UTC(), ID: id}, nil
}
