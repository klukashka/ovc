package api

import (
	"context"
	"errors"
	"net/http"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/schemas"
	"ovc/backend/internal/services/tutoring"
)

const groupsPath = "/api/v1/groups"

type groupListOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.GroupList
}

type createGroupInput struct {
	ContentType    string `header:"Content-Type"`
	IdempotencyKey string `header:"Idempotency-Key" maxLength:"200" pattern:"^[!-~]+$"`
	Body           schemas.NewGroup
}

type groupInput struct {
	ID string `path:"id"`
}

type updateGroupInput struct {
	ID          string `path:"id"`
	ContentType string `header:"Content-Type"`
	Body        schemas.GroupPatch
}

type groupOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.Group
}

type groupDetailOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         schemas.GroupDetail
}

func (r *router) registerGroups() {
	list := operation("groups-list", http.MethodGet, groupsPath)
	list.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound)
	register(r, list, AudienceTutor, r.listGroups)

	create := operation("groups-create", http.MethodPost, groupsPath)
	create.DefaultStatus = http.StatusCreated
	create.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict)
	register(r, create, AudienceTutor, r.createGroup)

	get := operation("groups-get", http.MethodGet, groupsPath+"/{id}")
	get.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound)
	register(r, get, AudienceTutor, r.group)

	update := operation("groups-update", http.MethodPatch, groupsPath+"/{id}")
	update.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict)
	register(r, update, AudienceTutor, r.updateGroup)

	archive := operation("groups-archive", http.MethodPost, groupsPath+"/{id}/archive")
	archive.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict)
	register(r, archive, AudienceTutor, r.archiveGroup)

	unarchive := operation("groups-unarchive", http.MethodPost, groupsPath+"/{id}/unarchive")
	unarchive.Errors = errorsOf(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound)
	register(r, unarchive, AudienceTutor, r.unarchiveGroup)
}

func (r *router) listGroups(ctx context.Context, _ *struct{}) (*groupListOutput, error) {
	groups, err := r.tutoring.ListGroups(ctx, principalFrom(ctx).UserID)
	if err != nil {
		return nil, r.groupError(ctx, "groups.list.failed", err)
	}
	ids := make([]domain.GroupID, 0, len(groups))
	for _, group := range groups {
		ids = append(ids, group.ID)
	}
	next, err := r.schedule.GroupsNextLesson(ctx, principalFrom(ctx).UserID, ids)
	if err != nil {
		return nil, r.scheduleError(ctx, "groups.list.failed", err)
	}
	body := schemas.GroupList{Groups: make([]schemas.GroupSummary, 0, len(groups))}
	for _, group := range groups {
		summary := schemas.GroupSummary{
			ID: group.ID.String(), Name: group.Name, Colour: group.Colour, ArchivedAt: utc(group.ArchivedAt),
			MemberCount: group.MemberCount, Members: groupMembersBody(group.FirstMembers),
		}
		if lesson, found := next[group.ID]; found {
			item := calendarItemBody(lesson)
			summary.NextLesson = &item
		}
		body.Groups = append(body.Groups, summary)
	}
	return &groupListOutput{CacheControl: noStore, Body: body}, nil
}

func (r *router) createGroup(ctx context.Context, input *createGroupInput) (*groupOutput, error) {
	if err := requireJSON(ctx, input.ContentType); err != nil {
		return nil, err
	}
	members, err := relationshipIDs(input.Body.MemberIDs)
	if err != nil {
		return nil, r.internal(ctx, "groups.create.failed", err)
	}
	request := tutoring.NewGroup{Name: input.Body.Name, Colour: input.Body.Colour, Members: members}
	if input.IdempotencyKey != "" {
		request.CreateKey = &input.IdempotencyKey
	}
	group, err := r.tutoring.CreateGroup(ctx, principalFrom(ctx).UserID, request)
	if err != nil {
		return nil, r.groupError(ctx, "groups.create.failed", err)
	}
	return &groupOutput{CacheControl: noStore, Body: groupBody(group)}, nil
}

func (r *router) group(ctx context.Context, input *groupInput) (*groupDetailOutput, error) {
	id, valid := domain.ParseUUID[domain.GroupID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	tutor := principalFrom(ctx).UserID
	group, err := r.tutoring.Group(ctx, tutor, id)
	if err != nil {
		return nil, r.groupError(ctx, "groups.get.failed", err)
	}
	lessons, err := r.schedule.GroupLessons(ctx, tutor, id)
	if err != nil {
		return nil, r.scheduleError(ctx, "groups.get.failed", err)
	}
	body := schemas.GroupDetail{
		Group: groupBody(group), NextLessons: calendarItemsBody(lessons.Next), FutureLessonCount: lessons.FutureLessonCount,
		HasOpenSeries: lessons.HasOpenSeries,
	}
	return &groupDetailOutput{CacheControl: noStore, Body: body}, nil
}

func (r *router) updateGroup(ctx context.Context, input *updateGroupInput) (*groupOutput, error) {
	if err := requireJSON(ctx, input.ContentType); err != nil {
		return nil, err
	}
	id, valid := domain.ParseUUID[domain.GroupID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	add, err := relationshipIDs(input.Body.Add)
	if err != nil {
		return nil, r.internal(ctx, "groups.update.failed", err)
	}
	remove, err := relationshipIDs(input.Body.Remove)
	if err != nil {
		return nil, r.internal(ctx, "groups.update.failed", err)
	}
	change := tutoring.GroupChange{Name: input.Body.Name, Colour: input.Body.Colour, Add: add, Remove: remove}
	group, err := r.tutoring.UpdateGroup(ctx, principalFrom(ctx).UserID, id, change)
	if err != nil {
		return nil, r.groupError(ctx, "groups.update.failed", err)
	}
	return &groupOutput{CacheControl: noStore, Body: groupBody(group)}, nil
}

func (r *router) archiveGroup(ctx context.Context, input *groupInput) (*groupOutput, error) {
	return r.changeArchive(ctx, input, "groups.archive.failed", r.tutoring.ArchiveGroup)
}

func (r *router) unarchiveGroup(ctx context.Context, input *groupInput) (*groupOutput, error) {
	return r.changeArchive(ctx, input, "groups.unarchive.failed", r.tutoring.UnarchiveGroup)
}

func (r *router) changeArchive(
	ctx context.Context, input *groupInput, event string, change func(context.Context, domain.UserID, domain.GroupID) (tutoring.Group, error),
) (*groupOutput, error) {
	id, valid := domain.ParseUUID[domain.GroupID](input.ID)
	if !valid {
		return nil, NewError(ctx, domain.ErrorNotFound)
	}
	group, err := change(ctx, principalFrom(ctx).UserID, id)
	if err != nil {
		return nil, r.groupError(ctx, event, err)
	}
	return &groupOutput{CacheControl: noStore, Body: groupBody(group)}, nil
}

var errMemberIDUnparsed = errors.New("a member identifier passed the schema but is not a UUID")

func relationshipIDs(texts []string) ([]domain.RelationshipID, error) {
	ids := make([]domain.RelationshipID, 0, len(texts))
	for _, text := range texts {
		id, valid := domain.ParseUUID[domain.RelationshipID](text)
		if !valid {
			return nil, errMemberIDUnparsed
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func groupBody(group tutoring.Group) schemas.Group {
	return schemas.Group{
		ID: group.ID.String(), Name: group.Name, Colour: group.Colour, ArchivedAt: utc(group.ArchivedAt), Members: groupMembersBody(group.Members),
	}
}

func groupMembersBody(members []tutoring.GroupMember) []schemas.GroupMember {
	body := make([]schemas.GroupMember, 0, len(members))
	for _, member := range members {
		body = append(body, schemas.GroupMember{
			RelationshipID: member.RelationshipID.String(), DisplayName: member.DisplayName, Name: member.Name,
			AvatarID: optionalID(member.AvatarID), AddedAt: member.AddedAt.UTC(),
		})
	}
	return body
}

func (r *router) groupError(ctx context.Context, event string, err error) error {
	var inactive *tutoring.MembersNotActiveError
	switch {
	case errors.Is(err, tutoring.ErrGroupNotFound), errors.Is(err, tutoring.ErrGroupMemberNotFound):
		return NewError(ctx, domain.ErrorNotFound)
	case errors.Is(err, tutoring.ErrGroupArchived):
		return NewError(ctx, domain.ErrorAudienceNotActive)
	case errors.Is(err, tutoring.ErrConflict):
		return NewError(ctx, domain.ErrorConflict)
	case errors.As(err, &inactive):
		ids := make([]string, 0, len(inactive.Relationships))
		for _, id := range inactive.Relationships {
			ids = append(ids, id.String())
		}
		refusal := NewError(ctx, domain.ErrorRelationshipNotActive)
		refusal.Body.Details = &schemas.ErrorDetails{RelationshipIDs: ids}
		return refusal
	}
	return r.tutoringError(ctx, event, err)
}
