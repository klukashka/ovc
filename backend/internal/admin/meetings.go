package admin

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/services/meetings"
)

func (c Command) meeting(ctx context.Context, subcommand string, args []string) error {
	if subcommand == "meetings status" {
		return c.meetingStatus(ctx, args)
	}
	if len(args) == 0 {
		return errUsage
	}
	id, valid := domain.ParseUUID[domain.ProviderMeetingID](args[0])
	if !valid || id == (domain.ProviderMeetingID{}) {
		return errUsage
	}
	flags := c.flags(subcommand)
	if subcommand == "meetings inspect" {
		after := flags.String("after", "0", "")
		if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 {
			return errUsage
		}
		cycle, err := strconv.ParseInt(*after, 10, 64)
		if err != nil || cycle < 0 {
			return errUsage
		}
		inspection, err := c.Meetings.Inspect(ctx, id, domain.MeetingRecoveryCycleNumber(cycle))
		if err != nil {
			return err
		}
		c.writeMeeting(inspection.Recovery)
		for _, receipt := range inspection.Cycles {
			c.write(c.Stdout, fmt.Sprintf("cycle\t%d\t%s\t%s\t%s", receipt.CycleNumber, receipt.OutboxID, receipt.Mode, receipt.Result))
		}
		if inspection.Next != nil {
			c.write(c.Stdout, fmt.Sprintf("next\t%d", *inspection.Next))
		}
		return nil
	}
	revisionText := flags.String("revision", "", "")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 {
		return errUsage
	}
	revision, err := strconv.ParseInt(*revisionText, 10, 64)
	if err != nil || revision <= 0 {
		return errUsage
	}
	receipt, err := c.Meetings.Retry(ctx, id, domain.MeetingRecoveryRevision(revision))
	if err != nil {
		return err
	}
	c.write(c.Stdout, fmt.Sprintf("cycle\t%d\t%s\t%s\t%s", receipt.CycleNumber, receipt.OutboxID, receipt.Mode, receipt.Result))
	return nil
}

func (c Command) meetingStatus(ctx context.Context, args []string) error {
	flags := c.flags("meetings status")
	afterText := flags.String("after", "", "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return errUsage
	}
	var after domain.ProviderMeetingID
	if *afterText != "" {
		parsed, valid := domain.ParseUUID[domain.ProviderMeetingID](*afterText)
		if !valid || parsed == (domain.ProviderMeetingID{}) {
			return errUsage
		}
		after = parsed
	}
	status, err := c.Meetings.Status(ctx, after)
	if err != nil {
		return err
	}
	c.write(c.Stdout, fmt.Sprintf("capacity\tunsettled=%d\tlimit=%d", status.Unsettled, domain.MeetingRecoveryCapacity))
	for _, row := range status.Rows {
		c.writeMeeting(row)
	}
	if status.Next != nil {
		c.write(c.Stdout, "next\t"+status.Next.String())
	}
	return nil
}

func (c Command) writeMeeting(row meetings.RecoveryEntry) {
	generation := "unknown"
	if row.CreateTime != nil {
		generation = strconv.FormatInt(int64(*row.CreateTime), 10)
	}
	settlement := "none"
	if row.Settlement != nil {
		settlement = string(*row.Settlement)
	}
	dispatched := "none"
	if row.DispatchedAt != nil {
		dispatched = row.DispatchedAt.UTC().Format(time.RFC3339)
	}
	c.write(c.Stdout, fmt.Sprintf("recovery\t%s\t%s\torigin=%s\tcreate=%s\trevision=%d\tcleanup_requested=%t\tsettled=%t\tcycle=%d\tresult=%s\torphaned=%t\tgeneration=%s\tfinal_cycle_requested=%t\tfinal_cycle_used=%t\tsettlement=%s\tdispatched_at=%s\treconcile_cycle_used=%t\toperator_action=%t", row.ProviderMeetingID, row.Provider, row.Origin, row.CreateState, row.Revision, row.CleanupRequested, row.Settled, row.CycleNumber, row.CycleResult, row.Orphaned, generation, row.FinalCycleRequested, row.FinalCycleUsed, settlement, dispatched, row.ReconcileCycleUsed, row.OperatorAction))
}
