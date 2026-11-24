package main

import (
	"log/slog"
	"os"
	"os/user"

	"ovc/backend/cmd/internal/startup"
	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/admin"
	"ovc/backend/internal/core"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/export"
	"ovc/backend/internal/services/files"
	"ovc/backend/internal/services/meetings"
	"ovc/backend/internal/services/operators"
)

func main() {
	os.Exit(run())
}

func run() int {
	settings, err := core.LoadFromEnvironment(core.ProcessAdmin)
	if err != nil {
		admin.Report(os.Stderr, "configuration: "+err.Error())
		return admin.ExitRefused
	}
	logging := core.NewLogging(os.Stderr, settings.LogLevel, settings.LogFormat)
	defer startup.CloseLogging(logging)
	ctx, stop := startup.SignalContext()
	defer stop()
	operator := admin.Operator{UID: "unknown", Name: "unknown"}
	current, err := user.Current()
	if err != nil {
		return admin.Failed(ctx, logging.Logger, os.Stderr, operator, os.Args[1:], err)
	}
	operator = admin.Operator{UID: current.Uid, Name: current.Username}
	database, err := core.Connect(ctx, settings, logging.Logger, clock.System{}.Now)
	if err != nil {
		return admin.Failed(ctx, logging.Logger, os.Stderr, operator, os.Args[1:], err)
	}
	defer startup.CloseDatabase(database, logging.Logger)
	if err := startup.CheckMigrations(ctx, database.SQL, logging.Logger); err != nil {
		return admin.Failed(ctx, logging.Logger, os.Stderr, operator, os.Args[1:], err)
	}
	uploads := files.New(files.Dependencies{Store: repositories.NewStore(database.Pool), Clock: source(settings, database), Random: core.CryptoRandom(), Logger: logging.Logger, Lifetime: ctx})
	exportEffects := export.NewEffects(export.Dependencies{Store: repositories.NewStore(database.Pool), Random: core.CryptoRandom(), Logger: logging.Logger, Cleanup: uploads})
	command := admin.Command{
		Meetings: meetings.NewOperator(repositories.NewStore(database.Pool), source(settings, database), core.CryptoRandom(), logging.Logger.With(slog.String("operator_uid", operator.UID), slog.String("operator", operator.Name))),
		Uploads:  uploads,
		Service:  operators.New(repositories.NewStore(database.Pool), source(settings, database), logging.Logger, exportEffects),
		Operator: operator,
		Logger:   logging.Logger,
		Stdout:   os.Stdout,
		Stderr:   os.Stderr,
	}
	return command.Run(ctx, os.Args[1:])
}

func source(settings core.Settings, database *core.Database) clock.Source {
	if settings.Clock == core.ClockTest {
		return clock.NewShared(database.Pool)
	}
	return clock.System{}
}
