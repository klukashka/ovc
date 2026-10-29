package chat

import (
	"context"
	"errors"
	"slices"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

func (m Mirror) ForgetMember(ctx context.Context, repos repositories.Repositories, user domain.UserID) error {
	locked, err := repos.Channels.LockOfMember(ctx, user)
	if err != nil || len(locked) == 0 {
		return err
	}
	rows, err := repos.Channels.ActiveMembers(ctx, locked)
	if err != nil {
		return err
	}
	if _, err := repos.Channels.DeleteMembershipsOf(ctx, user); err != nil {
		return err
	}
	outcome := reconciled{path: domain.PathStudentPurge}
	var signals []domain.EntitySignal
	for _, row := range rows {
		if row.UserID != user || row.TutorID == user {
			continue
		}
		left := user
		outcome.changes = append(outcome.changes, memberChange{channel: row.ChannelID, tutor: row.TutorID, user: &left, change: domain.MembershipLeft})
		tutorActive := slices.ContainsFunc(rows, func(other repositories.ActiveMember) bool {
			return other.ChannelID == row.ChannelID && other.UserID == row.TutorID
		})
		if tutorActive {
			signals = append(signals, domain.EntitySignal{Users: []domain.UserID{row.TutorID}, Entity: domain.EntityChannel, ID: domain.EntityID(row.ChannelID)})
		}
	}
	if err := repos.Signals.Entities(ctx, signals); err != nil {
		return err
	}
	repos.AfterCommit(ctx, func(ctx context.Context) { m.log(ctx, outcome) })
	return nil
}

func (m Mirror) CardRenamed(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, relationship domain.RelationshipID) error {
	channel, err := repos.Channels.LockOfRelationshipForShare(ctx, relationship)
	if errors.Is(err, repositories.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return signalMembers(ctx, repos, []domain.ChannelID{channel}, func(member repositories.ActiveMember) bool { return member.UserID == tutor })
}

func (m Mirror) TutorRenamed(ctx context.Context, repos repositories.Repositories, tutor domain.UserID) error {
	channels, err := repos.Channels.LockOfTutorForShare(ctx, tutor)
	if err != nil || len(channels) == 0 {
		return err
	}
	return signalMembers(ctx, repos, channels, func(member repositories.ActiveMember) bool { return member.UserID != tutor })
}

func signalMembers(
	ctx context.Context, repos repositories.Repositories, channels []domain.ChannelID, addressed func(repositories.ActiveMember) bool,
) error {
	rows, err := repos.Channels.ActiveMembers(ctx, channels)
	if err != nil {
		return err
	}
	recipients := map[domain.ChannelID][]domain.UserID{}
	for _, row := range rows {
		if addressed(row) {
			recipients[row.ChannelID] = append(recipients[row.ChannelID], row.UserID)
		}
	}
	signals := make([]domain.EntitySignal, 0, len(recipients))
	for _, channel := range channels {
		if users := recipients[channel]; len(users) > 0 {
			signals = append(signals, domain.EntitySignal{Users: users, Entity: domain.EntityChannel, ID: domain.EntityID(channel)})
		}
	}
	return repos.Signals.Entities(ctx, signals)
}
