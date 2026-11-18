package tutoring

import (
	"context"
	"strings"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

type ChecklistFacts struct {
	ProfileDone bool
	Dismissed   bool
	HasCard     *bool
}

func (s *Service) VisibleRequestCount(ctx context.Context, tutor domain.UserID) (int64, error) {
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return 0, err
	}
	return s.deps.Store.Repositories().JoinRequests.VisibleCount(ctx, tutor, now, domain.HomeCountCap)
}

func (s *Service) ChecklistFacts(ctx context.Context, tutor domain.UserID, withCard bool) (ChecklistFacts, error) {
	var facts ChecklistFacts
	err := s.deps.Store.WithReadTx(ctx, func(repos repositories.Repositories) error {
		profile, err := repos.TutorProfiles.ByUser(ctx, tutor)
		if err != nil {
			return err
		}
		facts.ProfileDone = strings.TrimSpace(profile.Bio) != "" || len(profile.Subjects) > 0
		facts.Dismissed = profile.OnboardingDismissedAt != nil
		if !withCard {
			return nil
		}
		hasCard, err := repos.Relationships.TutorHasCard(ctx, tutor)
		if err != nil {
			return err
		}
		facts.HasCard = &hasCard
		return nil
	})
	return facts, err
}
