package admin

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/services/operators"
)

const (
	ExitOK      = 0
	ExitRefused = 1
	ExitUsage   = 2

	usage = `usage:
  ovc-admin users list [--role tutor|student] [--email <substring>] [--state unverified|active|blocked|deleted]
  ovc-admin users block <email>
  ovc-admin users unblock <email>
  ovc-admin users restore <email>
  ovc-admin tutors set-quota <email> --files <bytes> --recordings <bytes>`
)

var errUsage = errors.New("the command line is not a known command")

type Operator struct {
	UID  string
	Name string
}

type Command struct {
	Service  *operators.Service
	Operator Operator
	Logger   *slog.Logger
	Stdout   io.Writer
	Stderr   io.Writer
}

type outcome struct {
	subcommand string
	user       *domain.UserID
	err        error
}

func Report(target io.Writer, line string) {
	if _, err := fmt.Fprintln(target, line); err != nil {
		return
	}
}

func Failed(ctx context.Context, logger *slog.Logger, stderr io.Writer, operator Operator, args []string, err error) int {
	logger.LogAttrs(ctx, slog.LevelError, "admin.command",
		slog.String("operator_uid", operator.UID), slog.String("operator", operator.Name), slog.String("subcommand", subcommandOf(args)),
		slog.String("outcome", "failed"), slog.String("kind", core.ErrorKind(err)))
	Report(stderr, "failed: "+core.ErrorKind(err))
	return ExitRefused
}

func subcommandOf(args []string) string {
	if len(args) < 2 {
		return "unknown"
	}
	switch subcommand := args[0] + " " + args[1]; subcommand {
	case "users list", "users block", "users unblock", "users restore", "tutors set-quota":
		return subcommand
	}
	return "unknown"
}

func (c Command) Run(ctx context.Context, args []string) int {
	result := c.dispatch(ctx, args)
	attributes := []slog.Attr{
		slog.String("operator_uid", c.Operator.UID), slog.String("operator", c.Operator.Name),
		slog.String("subcommand", result.subcommand), slog.String("outcome", outcomeName(result.err)),
	}
	if result.user != nil {
		attributes = append(attributes, slog.String("user_id", result.user.String()))
	}
	if result.err != nil && !refused(result.err) && !errors.Is(result.err, errUsage) {
		attributes = append(attributes, slog.String("kind", core.ErrorKind(result.err)))
	}
	c.Logger.LogAttrs(ctx, slog.LevelInfo, "admin.command", attributes...)
	switch {
	case result.err == nil:
		return ExitOK
	case errors.Is(result.err, errUsage):
		c.write(c.Stderr, usage)
		return ExitUsage
	case refused(result.err):
		c.write(c.Stderr, "refused: "+refusal(result.err).Error())
		return ExitRefused
	}
	c.write(c.Stderr, "failed: "+core.ErrorKind(result.err))
	return ExitRefused
}

func (c Command) dispatch(ctx context.Context, args []string) outcome {
	if len(args) < 2 {
		return outcome{subcommand: "unknown", err: errUsage}
	}
	subcommand := args[0] + " " + args[1]
	rest := args[2:]
	switch subcommand {
	case "users list":
		return outcome{subcommand: subcommand, err: c.list(ctx, rest)}
	case "users block":
		return c.change(ctx, subcommand, rest, c.Service.Block)
	case "users unblock":
		return c.change(ctx, subcommand, rest, c.Service.Unblock)
	case "users restore":
		return c.change(ctx, subcommand, rest, c.Service.Restore)
	case "tutors set-quota":
		return c.setQuota(ctx, subcommand, rest)
	}
	return outcome{subcommand: "unknown", err: errUsage}
}

func (c Command) list(ctx context.Context, args []string) error {
	flags := c.flags("users list")
	role := flags.String("role", "", "")
	email := flags.String("email", "", "")
	state := flags.String("state", "", "")
	if err := flags.Parse(args); err != nil || flags.NArg() > 0 {
		return errUsage
	}
	filter := operators.Filter{}
	if *role != "" {
		parsed := domain.Role(*role)
		if parsed != domain.RoleTutor && parsed != domain.RoleStudent {
			return errUsage
		}
		filter.Role = &parsed
	}
	if *state != "" {
		parsed := domain.AccountState(*state)
		if parsed != domain.AccountUnverified && parsed != domain.AccountActive && parsed != domain.AccountBlocked && parsed != domain.AccountDeleted {
			return errUsage
		}
		filter.State = &parsed
	}
	if *email != "" {
		filter.Email = email
	}
	accounts, err := c.Service.List(ctx, filter)
	if err != nil {
		return err
	}
	for _, account := range accounts {
		c.write(c.Stdout, fmt.Sprintf("%s\t%s\t%s\t%s\t%s", account.ID, account.Role, account.State, account.Email, account.CreatedAt.UTC().Format(time.RFC3339)))
	}
	return nil
}

func (c Command) change(ctx context.Context, subcommand string, args []string, action func(context.Context, string) (domain.UserID, error)) outcome {
	if len(args) != 1 || args[0] == "" {
		return outcome{subcommand: subcommand, err: errUsage}
	}
	user, err := action(ctx, args[0])
	result := outcome{subcommand: subcommand, err: err}
	if user != (domain.UserID{}) {
		result.user = &user
	}
	if err == nil {
		c.write(c.Stdout, "done "+user.String())
	}
	return result
}

func (c Command) setQuota(ctx context.Context, subcommand string, args []string) outcome {
	if len(args) < 1 || args[0] == "" {
		return outcome{subcommand: subcommand, err: errUsage}
	}
	flags := c.flags(subcommand)
	files := flags.String("files", "", "")
	recordings := flags.String("recordings", "", "")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() > 0 {
		return outcome{subcommand: subcommand, err: errUsage}
	}
	fileBytes, fileErr := strconv.ParseInt(*files, 10, 64)
	recordingBytes, recordingErr := strconv.ParseInt(*recordings, 10, 64)
	if fileErr != nil || recordingErr != nil {
		return outcome{subcommand: subcommand, err: errUsage}
	}
	return c.change(ctx, subcommand, args[:1], func(ctx context.Context, email string) (domain.UserID, error) {
		return c.Service.SetQuota(ctx, email, fileBytes, recordingBytes)
	})
}

func (c Command) flags(name string) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	return flags
}

func (c Command) write(target io.Writer, line string) {
	if _, err := fmt.Fprintln(target, line); err != nil {
		c.Logger.LogAttrs(context.Background(), slog.LevelWarn, "admin.output.failed", slog.String("kind", core.ErrorKind(err)))
	}
}

var refusals = []error{operators.ErrUnknownAccount, operators.ErrImpossibleState, operators.ErrWrongRole, operators.ErrInvalidQuota}

func refusal(err error) error {
	for _, candidate := range refusals {
		if errors.Is(err, candidate) {
			return candidate
		}
	}
	return nil
}

func refused(err error) bool {
	return refusal(err) != nil
}

func outcomeName(err error) string {
	switch {
	case err == nil:
		return "ok"
	case errors.Is(err, errUsage):
		return "usage"
	case refused(err):
		return "refused"
	}
	return "failed"
}
