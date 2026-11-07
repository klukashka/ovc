package schedule

import (
	"context"
	"errors"
	"slices"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

type Member struct {
	Label     string
	LabelKind *domain.LabelKind
}

type Audience struct {
	Kind           domain.AudienceKind
	RelationshipID *domain.RelationshipID
	GroupID        *domain.GroupID
	Label          *string
	LabelKind      *domain.LabelKind
	Colour         *domain.Colour
	Members        []Member
}

type TutorLabel struct {
	Name   string
	Colour *domain.Colour
}

type Lesson struct {
	ID              domain.LessonID
	Title           string
	Description     *string
	StartsAt        time.Time
	DurationMinutes int
	Status          domain.LessonStatus
	Timezone        string
	SeriesID        *domain.SeriesID
	OccurrenceDate  *domain.LocalDate
	Series          *SeriesView
	Version         int32
	CancelledAt     *time.Time
	OtherTimezone   *string
	Audience        Audience
	Tutor           *TutorLabel
	ChannelID       *domain.ChannelID
	RoomOpen        bool
	EmbedAllowed    bool
}

func (l Lesson) EndsAt() time.Time {
	return l.StartsAt.Add(time.Duration(l.DurationMinutes) * time.Minute)
}

func fixed(kind domain.LabelKind) *domain.LabelKind {
	return &kind
}

func colourOf(text string) *domain.Colour {
	if text == "" {
		return nil
	}
	colour := domain.Colour(text)
	return &colour
}

type tutorLabelSource struct {
	relationship      *domain.RelationshipID
	group             *domain.GroupID
	cardName          *string
	cardState         string
	cardDiscardReason *domain.DiscardReason
	cardColour        *domain.Colour
	groupName         *string
	groupColour       string
}

func (source tutorLabelSource) audience() Audience {
	if source.relationship == nil {
		return Audience{
			Kind: domain.AudienceGroup, GroupID: source.group, Label: source.groupName, Colour: colourOf(source.groupColour), Members: []Member{},
		}
	}
	audience := Audience{Kind: domain.AudienceOneToOne, Label: source.cardName, Colour: source.cardColour, Members: []Member{}}
	discarded := domain.RelationshipState(source.cardState) == domain.RelationshipDiscarded
	if !discarded {
		audience.RelationshipID = source.relationship
	}
	removed := source.cardDiscardReason != nil && (*source.cardDiscardReason == domain.DiscardRemoved || *source.cardDiscardReason == domain.DiscardTutorDeleted)
	if discarded && removed {
		audience.Label, audience.LabelKind = nil, fixed(domain.LabelRemovedStudent)
	}
	return audience
}

func memberOf(displayName string, accountState string) Member {
	member := Member{Label: displayName}
	if domain.AccountState(accountState) == domain.AccountDeleted {
		member.LabelKind = fixed(domain.LabelDeletedAccount)
	}
	return member
}

func (s *Service) tutorViews(ctx context.Context, repos repositories.Repositories, tutor repositories.User, ids []domain.LessonID) ([]Lesson, error) {
	rows, err := repos.Lessons.TutorViews(ctx, tutor.ID, ids)
	if err != nil {
		return nil, err
	}
	var groups []domain.GroupID
	var frozen []domain.LessonID
	for _, row := range rows {
		switch {
		case row.GroupID == nil:
		case row.Status == domain.LessonScheduled:
			groups = append(groups, *row.GroupID)
		default:
			frozen = append(frozen, row.ID)
		}
	}
	current, err := repos.Lessons.GroupMembers(ctx, tutor.ID, groups)
	if err != nil {
		return nil, err
	}
	roster, err := repos.Lessons.RosterMembers(ctx, tutor.ID, frozen)
	if err != nil {
		return nil, err
	}
	byGroup := map[domain.GroupID][]Member{}
	for _, member := range current {
		byGroup[member.GroupID] = append(byGroup[member.GroupID], memberOf(member.DisplayName, member.AccountState))
	}
	byLesson := map[domain.LessonID][]Member{}
	for _, member := range roster {
		byLesson[member.LessonID] = append(byLesson[member.LessonID], memberOf(member.DisplayName, member.AccountState))
	}
	lessons := make([]Lesson, 0, len(rows))
	for _, row := range rows {
		source := tutorLabelSource{
			relationship: row.RelationshipID, group: row.GroupID, cardName: row.CardName, cardState: row.CardState, cardDiscardReason: row.CardDiscardReason,
			cardColour: row.CardColour, groupName: row.GroupName, groupColour: row.GroupColour,
		}
		lesson := Lesson{
			ID: row.ID, Title: row.Title, Description: row.Description, StartsAt: row.StartsAt.UTC(), DurationMinutes: int(row.DurationMinutes),
			Status: row.Status, Timezone: row.Timezone, SeriesID: row.SeriesID, Version: row.Version, CancelledAt: row.CancelledAt,
			Audience: source.audience(), RoomOpen: row.RoomOpen, EmbedAllowed: s.deps.Embed.On(),
		}
		if row.OccurrenceDate.Valid {
			occurrence := domain.DateOf(row.OccurrenceDate.Time)
			lesson.OccurrenceDate = &occurrence
		}
		if row.GroupID != nil {
			members := byLesson[row.ID]
			if row.Status == domain.LessonScheduled {
				members = byGroup[*row.GroupID]
			}
			lesson.Audience.Members = append([]Member{}, members...)
		}
		state := domain.RelationshipState(row.CardState)
		linked := state == domain.RelationshipActive || state == domain.RelationshipProvisional
		if row.RelationshipID != nil && linked && row.StudentTimezone != nil && *row.StudentTimezone != tutor.Timezone {
			lesson.OtherTimezone = row.StudentTimezone
		}
		lessons = append(lessons, lesson)
	}
	if err := withChannels(ctx, repos, tutor.ID, lessons); err != nil {
		return nil, err
	}
	return withSeries(ctx, repos, lessons)
}

func withChannels(ctx context.Context, repos repositories.Repositories, viewer domain.UserID, lessons []Lesson) error {
	ids := make([]domain.LessonID, 0, len(lessons))
	for _, lesson := range lessons {
		ids = append(ids, lesson.ID)
	}
	channels, err := repos.Channels.OfLessonsFor(ctx, viewer, ids)
	if err != nil {
		return err
	}
	for index := range lessons {
		if channel, found := channels[lessons[index].ID]; found {
			lessons[index].ChannelID = &channel
		}
	}
	return nil
}

func withSeries(ctx context.Context, repos repositories.Repositories, lessons []Lesson) ([]Lesson, error) {
	var ids []domain.SeriesID
	for _, lesson := range lessons {
		if lesson.SeriesID != nil && !slices.Contains(ids, *lesson.SeriesID) {
			ids = append(ids, *lesson.SeriesID)
		}
	}
	rows, err := repos.Lessons.SeriesViews(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := map[domain.SeriesID]*SeriesView{}
	for _, row := range rows {
		byID[row.ID] = seriesView(row)
	}
	for index := range lessons {
		if lessons[index].SeriesID != nil {
			lessons[index].Series = byID[*lessons[index].SeriesID]
		}
	}
	return lessons, nil
}

type studentLabelSource struct {
	group         *domain.GroupID
	groupName     *string
	currentMember bool
	tutorName     string
	myState       string
	myColour      *domain.Colour
}

func listedByMyTutors(state domain.RelationshipState) bool {
	return state == domain.RelationshipActive || state == domain.RelationshipProvisional || state == domain.RelationshipEnded
}

func (source studentLabelSource) labels() (Audience, *TutorLabel) {
	audience := Audience{Kind: domain.AudienceOneToOne, LabelKind: fixed(domain.LabelIndividual), Colour: source.myColour}
	if source.group != nil {
		audience.Kind, audience.LabelKind = domain.AudienceGroup, nil
		audience.Label = source.groupName
		if !source.currentMember {
			audience.Label, audience.LabelKind = nil, fixed(domain.LabelGroup)
		}
	}
	if !listedByMyTutors(domain.RelationshipState(source.myState)) {
		return audience, nil
	}
	return audience, &TutorLabel{Name: source.tutorName, Colour: source.myColour}
}

func (s *Service) Lesson(ctx context.Context, viewer domain.UserID, role domain.Role, id domain.LessonID) (Lesson, error) {
	var lesson Lesson
	err := s.deps.Store.WithReadTx(ctx, func(repos repositories.Repositories) error {
		account, err := repos.Users.ByID(ctx, viewer)
		if err != nil {
			return err
		}
		if role == domain.RoleTutor {
			views, err := s.tutorViews(ctx, repos, account, []domain.LessonID{id})
			if err != nil {
				return err
			}
			if len(views) == 0 {
				return ErrNotFound
			}
			lesson = views[0]
			return nil
		}
		row, err := repos.Lessons.StudentView(ctx, viewer, id)
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		source := studentLabelSource{
			group: row.GroupID, groupName: row.GroupName, currentMember: row.CurrentMember, tutorName: row.TutorName,
			myState: row.MyState, myColour: row.MyColour,
		}
		audience, tutor := source.labels()
		lesson = Lesson{
			ID: row.ID, Title: row.Title, Description: row.Description, StartsAt: row.StartsAt.UTC(), DurationMinutes: int(row.DurationMinutes),
			Status: row.Status, Timezone: row.Timezone, SeriesID: row.SeriesID, Version: row.Version, CancelledAt: row.CancelledAt,
			Audience: audience, Tutor: tutor, RoomOpen: row.RoomOpen, EmbedAllowed: s.deps.Embed.On(),
		}
		if row.Timezone != account.Timezone {
			other := row.Timezone
			lesson.OtherTimezone = &other
		}
		withChannel := []Lesson{lesson}
		if err := withChannels(ctx, repos, viewer, withChannel); err != nil {
			return err
		}
		lesson = withChannel[0]
		seenOnlyThroughRoster := !row.CurrentMember && !row.CurrentLink
		if seenOnlyThroughRoster {
			lesson.SeriesID = nil
			return nil
		}
		withSeriesView, err := withSeries(ctx, repos, []Lesson{lesson})
		if err != nil {
			return err
		}
		lesson = withSeriesView[0]
		return nil
	})
	return lesson, err
}
