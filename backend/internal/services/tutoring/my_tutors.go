package tutoring

import (
	"context"
	"errors"
	"fmt"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/accounts"
)

var errRelationshipWithoutColour = errors.New("a linked relationship has no colour")

type TutorProfile struct {
	Name     string
	AvatarID *domain.AttachmentID
	Subjects []string
	Bio      string
}

type MyTutor struct {
	RelationshipID     domain.RelationshipID
	State              domain.RelationshipState
	Colour             domain.Colour
	Tutor              TutorProfile
	EndedAt            *time.Time
	EndedBy            *domain.EndedBy
	AccountClosedUntil *time.Time
	ChannelID          *domain.ChannelID
	tutor              domain.UserID
}

func (s *Service) MyTutors(ctx context.Context, principal accounts.Principal) ([]MyTutor, error) {
	return s.tutorsOf(ctx, s.deps.Store.Repositories(), principal.UserID, nil)
}

func (s *Service) StudentTutorsIn(ctx context.Context, repos repositories.Repositories, student domain.UserID) ([]MyTutor, []domain.UserID, error) {
	tutors, err := s.tutorsOf(ctx, repos, student, nil)
	if err != nil {
		return nil, nil, err
	}
	referenced := make([]domain.UserID, 0, len(tutors))
	for _, tutor := range tutors {
		referenced = append(referenced, tutor.tutor)
	}
	return tutors, referenced, nil
}

func (s *Service) myTutor(ctx context.Context, repos repositories.Repositories, student domain.UserID, id domain.RelationshipID) (MyTutor, error) {
	tutors, err := s.tutorsOf(ctx, repos, student, &id)
	if err != nil {
		return MyTutor{}, err
	}
	if len(tutors) != 1 {
		return MyTutor{}, fmt.Errorf("read the linked relationship %s: %w", id, repositories.ErrNotFound)
	}
	return tutors[0], nil
}

func (s *Service) tutorsOf(
	ctx context.Context, repos repositories.Repositories, student domain.UserID, only *domain.RelationshipID,
) ([]MyTutor, error) {
	now, err := s.deps.Clock.InstantWithin(ctx, repos.TestClock)
	if err != nil {
		return nil, err
	}
	rows, err := repos.Relationships.TutorsOfStudent(ctx, student, only, now)
	if err != nil {
		return nil, err
	}
	ids := make([]domain.UserID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.TutorID)
	}
	avatars, err := repos.Relationships.AvatarsOf(ctx, ids)
	if err != nil {
		return nil, err
	}
	links := make([]domain.RelationshipID, 0, len(rows))
	for _, row := range rows {
		links = append(links, row.ID)
	}
	channels, err := repos.Channels.OfRelationshipsFor(ctx, student, links)
	if err != nil {
		return nil, err
	}
	tutors := make([]MyTutor, 0, len(rows))
	for _, row := range rows {
		if row.Colour == nil {
			return nil, fmt.Errorf("relationship %s: %w", row.ID, errRelationshipWithoutColour)
		}
		tutor := MyTutor{
			RelationshipID: row.ID, State: row.State, Colour: *row.Colour, EndedAt: row.EndedAt, EndedBy: row.EndedBy,
			Tutor: TutorProfile{Name: row.Name, Subjects: row.Subjects, Bio: row.Bio}, tutor: row.TutorID,
		}
		if avatar, has := avatars[row.TutorID]; has {
			tutor.Tutor.AvatarID = &avatar
		}
		if channel, has := channels[row.ID]; has {
			tutor.ChannelID = &channel
		}
		if row.TutorState == domain.AccountDeleted && row.DeletedAt != nil {
			closed := row.DeletedAt.Add(domain.DeletedAccountRetention)
			tutor.AccountClosedUntil = &closed
		}
		tutors = append(tutors, tutor)
	}
	return tutors, nil
}
