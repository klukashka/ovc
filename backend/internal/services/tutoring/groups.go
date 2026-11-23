package tutoring

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/chat"
)

const (
	fieldGroupName   = "body.name"
	fieldGroupColour = "body.colour"
	fieldGroupAdd    = "body.add"
	fieldGroupRemove = "body.remove"
	fieldGroupCount  = "tutor.groups"
)

var (
	ErrGroupNotFound       = errors.New("the group is not in the tutor's scope")
	ErrGroupMemberNotFound = errors.New("a member is not in the tutor's scope")
	ErrGroupArchived       = errors.New("an archived group takes no new members")
)

type MembersNotActiveError struct {
	Relationships []domain.RelationshipID
}

func (e *MembersNotActiveError) Error() string {
	return "a member's relationship is not active"
}

type GroupMember struct {
	RelationshipID domain.RelationshipID
	DisplayName    string
	Name           string
	AvatarID       *domain.AttachmentID
	AddedAt        time.Time
}

type Group struct {
	ID         domain.GroupID
	Name       string
	Colour     domain.Colour
	ArchivedAt *time.Time
	Members    []GroupMember
	ChannelID  *domain.ChannelID
}

type GroupSummary struct {
	ID           domain.GroupID
	Name         string
	Colour       domain.Colour
	ArchivedAt   *time.Time
	MemberCount  int64
	FirstMembers []GroupMember
}

type NewGroup struct {
	Name      string
	Colour    domain.Colour
	Members   []domain.RelationshipID
	CreateKey *string
}

type GroupChange struct {
	Name   *string
	Colour *domain.Colour
	Add    []domain.RelationshipID
	Remove []domain.RelationshipID
}

type membershipChange struct {
	added   int64
	removed int64
}

func (s *Service) ListGroups(ctx context.Context, tutor domain.UserID) ([]GroupSummary, error) {
	var summaries []GroupSummary
	err := s.deps.Store.WithReadTx(ctx, func(repos repositories.Repositories) error {
		groups, err := repos.Groups.OfTutor(ctx, tutor)
		if err != nil {
			return err
		}
		ids := make([]domain.GroupID, 0, len(groups))
		for _, group := range groups {
			ids = append(ids, group.ID)
		}
		first, err := repos.Groups.FirstMembers(ctx, tutor, ids, domain.GroupPreviewMembers)
		if err != nil {
			return err
		}
		avatars, err := repos.Relationships.AvatarsOf(ctx, firstMemberStudents(first))
		if err != nil {
			return err
		}
		summaries, err = groupSummaries(groups, first, avatars)
		return err
	})
	return summaries, err
}

func firstMemberStudents(members []repositories.FirstGroupMember) []domain.UserID {
	students := make([]domain.UserID, 0, len(members))
	for _, member := range members {
		if member.StudentID != nil {
			students = append(students, *member.StudentID)
		}
	}
	return students
}

func groupSummaries(
	groups []repositories.Group, first []repositories.FirstGroupMember, avatars map[domain.UserID]domain.AttachmentID,
) ([]GroupSummary, error) {
	counts := make(map[domain.GroupID]int64, len(groups))
	previews := make(map[domain.GroupID][]GroupMember, len(groups))
	for _, member := range first {
		counts[member.GroupID] = member.MemberCount
		view, err := memberView(member.RelationshipID, member.DisplayName, member.Name, member.StudentID, member.AddedAt, avatars)
		if err != nil {
			return nil, err
		}
		previews[member.GroupID] = append(previews[member.GroupID], view)
	}
	summaries := make([]GroupSummary, 0, len(groups))
	for _, group := range groups {
		summaries = append(summaries, GroupSummary{
			ID: group.ID, Name: group.Name, Colour: group.Colour, ArchivedAt: group.ArchivedAt,
			MemberCount: counts[group.ID], FirstMembers: previews[group.ID],
		})
	}
	return summaries, nil
}

func memberView(
	relationship domain.RelationshipID, displayName *string, name string, student *domain.UserID, addedAt time.Time, avatars map[domain.UserID]domain.AttachmentID,
) (GroupMember, error) {
	cardName, err := domain.CardName(displayName)
	if err != nil {
		return GroupMember{}, err
	}
	member := GroupMember{RelationshipID: relationship, DisplayName: cardName, Name: name, AddedAt: addedAt}
	if student != nil {
		if avatar, has := avatars[*student]; has {
			member.AvatarID = &avatar
		}
	}
	return member, nil
}

func (s *Service) Group(ctx context.Context, tutor domain.UserID, id domain.GroupID) (Group, error) {
	var group Group
	err := s.deps.Store.WithReadTx(ctx, func(repos repositories.Repositories) error {
		row, err := repos.Groups.ForTutor(ctx, tutor, id)
		if errors.Is(err, repositories.ErrNotFound) {
			return ErrGroupNotFound
		}
		if err != nil {
			return err
		}
		group, err = s.groupView(ctx, repos, row)
		return err
	})
	return group, err
}

func (s *Service) groupView(ctx context.Context, repos repositories.Repositories, row repositories.Group) (Group, error) {
	members, err := repos.Groups.Members(ctx, row.TutorID, row.ID)
	if err != nil {
		return Group{}, err
	}
	students := make([]domain.UserID, 0, len(members))
	for _, member := range members {
		if member.StudentID != nil {
			students = append(students, *member.StudentID)
		}
	}
	avatars, err := repos.Relationships.AvatarsOf(ctx, students)
	if err != nil {
		return Group{}, err
	}
	group := Group{ID: row.ID, Name: row.Name, Colour: row.Colour, ArchivedAt: row.ArchivedAt, Members: make([]GroupMember, 0, len(members))}
	for _, member := range members {
		view, err := memberView(member.RelationshipID, member.DisplayName, member.Name, member.StudentID, member.AddedAt, avatars)
		if err != nil {
			return Group{}, err
		}
		group.Members = append(group.Members, view)
	}
	group.ChannelID, err = repos.Channels.OfGroupFor(ctx, row.TutorID, row.ID)
	return group, err
}

func (s *Service) CreateGroup(ctx context.Context, actor domain.TutorActor, request NewGroup) (Group, error) {
	tutor := actor.UserID
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return Group{}, err
	}
	members := distinctRelationships(request.Members)
	var group Group
	created := false
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		if err := s.lockActingTutor(ctx, repos, actor); err != nil {
			return err
		}
		if request.CreateKey != nil {
			replayed, found, err := s.replayedGroup(ctx, repos, tutor, *request.CreateKey)
			if err != nil || found {
				group = replayed
				return err
			}
		}
		row, err := s.insertGroup(ctx, repos, tutor, request, members, now)
		if err != nil {
			return err
		}
		created = true
		group, err = s.groupView(ctx, repos, row)
		return err
	})
	if err != nil {
		return Group{}, err
	}
	if created {
		attributes := append(groupAttributes(tutor, group.ID), slog.Int("members", len(group.Members)))
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "group.created", attributes...)
	}
	return group, nil
}

func (s *Service) replayedGroup(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, key string) (Group, bool, error) {
	row, err := repos.Groups.ByCreateKey(ctx, tutor, key)
	if errors.Is(err, repositories.ErrNotFound) {
		return Group{}, false, nil
	}
	if err != nil {
		return Group{}, false, err
	}
	group, err := s.groupView(ctx, repos, row)
	return group, true, err
}

func (s *Service) insertGroup(
	ctx context.Context, repos repositories.Repositories, tutor domain.UserID, request NewGroup, members []domain.RelationshipID, now time.Time,
) (repositories.Group, error) {
	candidates, err := lockCandidates(ctx, repos.Relationships.LockForGroup, tutor, members)
	if err != nil {
		return repositories.Group{}, err
	}
	if err := refuseInactive(candidates, members); err != nil {
		return repositories.Group{}, err
	}
	name := strings.TrimSpace(request.Name)
	if problems := groupProblems(&name, &request.Colour); len(problems) > 0 {
		return repositories.Group{}, &ValidationError{Fields: problems}
	}
	count, err := repos.Groups.CountOfTutor(ctx, tutor)
	if err != nil {
		return repositories.Group{}, err
	}
	if count >= domain.GroupLimitPerTutor {
		return repositories.Group{}, &ValidationError{Fields: []FieldProblem{{Path: fieldGroupCount, Code: domain.FieldTooLong}}}
	}
	id, err := core.NewID[domain.GroupID](s.deps.Random)
	if err != nil {
		return repositories.Group{}, err
	}
	row := repositories.NewGroup{ID: id, TutorID: tutor, Name: name, Colour: request.Colour, CreateKey: request.CreateKey, CreatedAt: now}
	if err := repos.Groups.Create(ctx, row); err != nil {
		return repositories.Group{}, err
	}
	if _, err := repos.Groups.AddMembers(ctx, tutor, id, members, now); err != nil {
		return repositories.Group{}, err
	}
	if err := s.mirror(ctx, repos, domain.PathGroupCreation, chat.Audiences{Groups: []domain.GroupID{id}}, now); err != nil {
		return repositories.Group{}, err
	}
	return repos.Groups.ForTutor(ctx, tutor, id)
}

type candidateLock func(ctx context.Context, tutor domain.UserID, ids []domain.RelationshipID) ([]repositories.GroupCandidate, error)

func lockCandidates(
	ctx context.Context, lock candidateLock, tutor domain.UserID, ids []domain.RelationshipID,
) (map[domain.RelationshipID]domain.RelationshipState, error) {
	rows, err := lock(ctx, tutor, ids)
	if err != nil {
		return nil, err
	}
	if len(rows) != len(ids) {
		return nil, ErrGroupMemberNotFound
	}
	states := make(map[domain.RelationshipID]domain.RelationshipState, len(rows))
	for _, row := range rows {
		states[row.ID] = row.State
	}
	return states, nil
}

func refuseInactive(states map[domain.RelationshipID]domain.RelationshipState, joining []domain.RelationshipID) error {
	var inactive []domain.RelationshipID
	for _, id := range joining {
		if states[id] != domain.RelationshipActive {
			inactive = append(inactive, id)
		}
	}
	if len(inactive) > 0 {
		return &MembersNotActiveError{Relationships: inactive}
	}
	return nil
}

func groupProblems(name *string, colour *domain.Colour) []FieldProblem {
	var problems []FieldProblem
	if name != nil && !validGroupName(*name) {
		problems = append(problems, FieldProblem{Path: fieldGroupName, Code: domain.FieldInvalid})
	}
	if colour != nil && !slices.Contains(domain.Palette, *colour) {
		problems = append(problems, FieldProblem{Path: fieldGroupColour, Code: domain.FieldInvalid})
	}
	return problems
}

func validGroupName(name string) bool {
	return name != "" && utf8.RuneCountInString(name) <= domain.GroupNameMaxRunes && !strings.ContainsFunc(name, unicode.IsControl)
}

func distinctRelationships(ids []domain.RelationshipID) []domain.RelationshipID {
	distinct := slices.Clone(ids)
	slices.SortFunc(distinct, func(a, b domain.RelationshipID) int { return bytes.Compare(a[:], b[:]) })
	return slices.Compact(distinct)
}

func (s *Service) UpdateGroup(ctx context.Context, actor domain.TutorActor, id domain.GroupID, change GroupChange) (Group, error) {
	tutor := actor.UserID
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return Group{}, err
	}
	change.Add, change.Remove = distinctRelationships(change.Add), distinctRelationships(change.Remove)
	var group Group
	var fields []string
	var membership membershipChange
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		if err := s.lockActingTutor(ctx, repos, actor); err != nil {
			return err
		}
		row, err := s.lockGroupForChange(ctx, repos, tutor, id, change)
		if err != nil {
			return err
		}
		fields, membership, err = applyGroupChange(ctx, repos, row, change, now)
		if err != nil {
			return err
		}
		if err := s.mirror(ctx, repos, domain.PathGroupChange, groupChangeAudience(row.ID, fields, membership), now); err != nil {
			return err
		}
		updated, err := repos.Groups.ForTutor(ctx, tutor, id)
		if err != nil {
			return err
		}
		group, err = s.groupView(ctx, repos, updated)
		return err
	})
	if err != nil {
		return Group{}, err
	}
	if len(fields) > 0 {
		attributes := append(groupAttributes(tutor, id), slog.String("fields", strings.Join(fields, ",")))
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "group.updated", attributes...)
	}
	if membership.added > 0 || membership.removed > 0 {
		attributes := append(groupAttributes(tutor, id), slog.Int64("added", membership.added), slog.Int64("removed", membership.removed))
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "group.membership_changed", attributes...)
	}
	return group, nil
}

func (s *Service) lockGroupForChange(
	ctx context.Context, repos repositories.Repositories, tutor domain.UserID, id domain.GroupID, change GroupChange,
) (repositories.Group, error) {
	if _, err := repos.Groups.ForTutor(ctx, tutor, id); err != nil {
		return repositories.Group{}, groupScope(err)
	}
	states, err := lockCandidates(ctx, repos.Relationships.LockForGroupChange, tutor, distinctRelationships(append(slices.Clone(change.Add), change.Remove...)))
	if err != nil {
		return repositories.Group{}, err
	}
	row, err := repos.Groups.LockForTutor(ctx, tutor, id)
	if err != nil {
		return repositories.Group{}, groupScope(err)
	}
	if len(change.Add) > 0 && row.ArchivedAt != nil {
		return repositories.Group{}, ErrGroupArchived
	}
	if err := refuseInactive(states, change.Add); err != nil {
		return repositories.Group{}, err
	}
	if problems := changeProblems(change); len(problems) > 0 {
		return repositories.Group{}, &ValidationError{Fields: problems}
	}
	return row, nil
}

func groupScope(err error) error {
	if errors.Is(err, repositories.ErrNotFound) {
		return ErrGroupNotFound
	}
	return err
}

func changeProblems(change GroupChange) []FieldProblem {
	var name *string
	if change.Name != nil {
		trimmed := strings.TrimSpace(*change.Name)
		name = &trimmed
	}
	problems := groupProblems(name, change.Colour)
	for _, id := range change.Remove {
		if slices.Contains(change.Add, id) {
			return append(problems, FieldProblem{Path: fieldGroupRemove, Code: domain.FieldInvalid})
		}
	}
	return problems
}

func applyGroupChange(
	ctx context.Context, repos repositories.Repositories, row repositories.Group, change GroupChange, now time.Time,
) ([]string, membershipChange, error) {
	current, err := repos.Groups.MemberIDs(ctx, row.TutorID, row.ID)
	if err != nil {
		return nil, membershipChange{}, err
	}
	if resultingMembers(current, change) > domain.GroupMemberLimit {
		return nil, membershipChange{}, &ValidationError{Fields: []FieldProblem{{Path: fieldGroupAdd, Code: domain.FieldTooLong}}}
	}
	fields, err := applyGroupDetails(ctx, repos, row, change, now)
	if err != nil {
		return nil, membershipChange{}, err
	}
	removed, err := repos.Groups.RemoveMembers(ctx, row.TutorID, row.ID, change.Remove)
	if err != nil {
		return nil, membershipChange{}, err
	}
	added, err := repos.Groups.AddMembers(ctx, row.TutorID, row.ID, change.Add, now)
	if err != nil {
		return nil, membershipChange{}, err
	}
	return fields, membershipChange{added: added, removed: removed}, nil
}

func groupChangeAudience(id domain.GroupID, fields []string, membership membershipChange) chat.Audiences {
	var audiences chat.Audiences
	if membership.added > 0 || membership.removed > 0 {
		audiences.Groups = []domain.GroupID{id}
	}
	if len(fields) > 0 {
		audiences.Relabelled = []domain.GroupID{id}
	}
	return audiences
}

func resultingMembers(current []domain.RelationshipID, change GroupChange) int {
	members := make(map[domain.RelationshipID]bool, len(current)+len(change.Add))
	for _, id := range current {
		members[id] = true
	}
	for _, id := range change.Remove {
		delete(members, id)
	}
	for _, id := range change.Add {
		members[id] = true
	}
	return len(members)
}

func applyGroupDetails(ctx context.Context, repos repositories.Repositories, row repositories.Group, change GroupChange, now time.Time) ([]string, error) {
	name, colour := row.Name, row.Colour
	var fields []string
	if change.Name != nil && strings.TrimSpace(*change.Name) != row.Name {
		name = strings.TrimSpace(*change.Name)
		fields = append(fields, "name")
	}
	if change.Colour != nil && *change.Colour != row.Colour {
		colour = *change.Colour
		fields = append(fields, "colour")
	}
	if len(fields) == 0 {
		return nil, nil
	}
	applied, err := repos.Groups.UpdateDetails(ctx, repositories.GroupDetailsChange{Name: name, Colour: colour, Now: now, ID: row.ID, TutorID: row.TutorID})
	if err != nil {
		return nil, err
	}
	if !applied {
		return nil, ErrGroupNotFound
	}
	return fields, nil
}

func (s *Service) UnarchiveGroup(ctx context.Context, actor domain.TutorActor, id domain.GroupID) (Group, error) {
	tutor := actor.UserID
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return Group{}, err
	}
	var group Group
	changed := false
	err = s.deps.Store.WithTx(ctx, func(repos repositories.Repositories) error {
		if err := s.lockActingTutor(ctx, repos, actor); err != nil {
			return err
		}
		if _, err := repos.Groups.LockForTutor(ctx, tutor, id); err != nil {
			return groupScope(err)
		}
		if changed, err = repos.Groups.Unarchive(ctx, tutor, id, now); err != nil {
			return err
		}
		if changed {
			if err := s.mirror(ctx, repos, domain.PathUnarchiving, chat.Audiences{Groups: []domain.GroupID{id}}, now); err != nil {
				return err
			}
		}
		group, err = s.currentGroup(ctx, repos, tutor, id)
		return err
	})
	if err != nil {
		return Group{}, err
	}
	if changed {
		attributes := append(groupAttributes(tutor, id), slog.Int("members", len(group.Members)))
		s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "group.unarchived", attributes...)
	}
	return group, nil
}

func (s *Service) currentGroup(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, id domain.GroupID) (Group, error) {
	row, err := repos.Groups.ForTutor(ctx, tutor, id)
	if err != nil {
		return Group{}, err
	}
	return s.groupView(ctx, repos, row)
}

func groupAttributes(tutor domain.UserID, group domain.GroupID) []slog.Attr {
	return []slog.Attr{slog.String("tutor_id", tutor.String()), slog.String("group_id", group.String())}
}
