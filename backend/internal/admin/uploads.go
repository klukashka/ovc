package admin

import (
	"context"
	"errors"
	"fmt"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/services/files"
)

func (c Command) upload(ctx context.Context, subcommand string, args []string) outcome {
	result := outcome{subcommand: subcommand}
	if len(args) == 0 {
		result.err = errUsage
		return result
	}
	id, valid := domain.ParseUUID[domain.AttachmentID](args[0])
	if !valid {
		result.err = errUsage
		return result
	}
	result.attachment = &id
	if subcommand == "uploads inspect" {
		result.err = c.inspectUpload(ctx, id, args[1:])
	} else {
		result.err = c.retryUpload(ctx, id, args[1:])
	}
	return result
}

func (c Command) inspectUpload(ctx context.Context, id domain.AttachmentID, args []string) error {
	flags := c.flags("uploads inspect")
	afterText := flags.String("after", "", "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return errUsage
	}
	var after domain.UploadAttemptID
	if *afterText != "" {
		parsed, valid := domain.ParseUUID[domain.UploadAttemptID](*afterText)
		if !valid {
			return errUsage
		}
		after = parsed
	}
	inspection, err := c.Uploads.InspectUpload(ctx, id, after)
	if err != nil {
		return err
	}
	c.write(c.Stdout, fmt.Sprintf("intent\t%s\t%s\t%s\tcompletion_started=%t\tcompletion_confirmed=%t\tcleanup_requested=%t\tfinal_cleanup_required=%t\treceipts=%d", inspection.Attachment, inspection.Phase, inspection.Result, inspection.CompletionStarted, inspection.CompletionConfirmed, inspection.CleanupRequested, inspection.FinalCleanupRequired, inspection.ReceiptCount))
	if inspection.Outbox != nil {
		c.write(c.Stdout, "outbox\t"+inspection.Outbox.String())
	}
	if inspection.Generation != nil {
		c.write(c.Stdout, fmt.Sprintf("generation\t%d", *inspection.Generation))
	}
	for _, receipt := range inspection.Receipts {
		c.write(c.Stdout, fmt.Sprintf("receipt\t%s\t%s\t%s", receipt.Attempt, receipt.Outbox, receipt.Status))
	}
	if inspection.Next != nil {
		c.write(c.Stdout, "next\t"+inspection.Next.String())
	}
	return nil
}

func (c Command) retryUpload(ctx context.Context, id domain.AttachmentID, args []string) error {
	flags := c.flags("uploads retry-cleanup")
	attemptText := flags.String("attempt", "", "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return errUsage
	}
	attempt, valid := domain.ParseUUID[domain.UploadAttemptID](*attemptText)
	if !valid || attempt == (domain.UploadAttemptID{}) {
		return errUsage
	}
	cycle, err := c.Uploads.RetryUploadCleanup(ctx, id, attempt)
	if err != nil && !errors.Is(err, files.ErrUploadCleanupActive) {
		return err
	}
	c.write(c.Stdout, fmt.Sprintf("cycle\t%s\t%s\t%s\t%s", id, cycle.Attempt, cycle.Outbox, cycle.Status))
	return err
}
