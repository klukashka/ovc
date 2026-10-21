package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"ovc/backend/cmd/internal/startup"
	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/adapters/storage"
	"ovc/backend/internal/core"
)

func main() {
	os.Exit(run())
}

func run() int {
	settings, err := core.LoadFromEnvironment(core.ProcessWorker)
	if err != nil {
		fmt.Fprintln(os.Stderr, "configuration:", err)
		return 1
	}
	logging := core.NewLogging(os.Stdout, settings.LogLevel, settings.LogFormat)
	defer startup.CloseLogging(logging)
	ctx, stop := startup.SignalContext()
	defer stop()
	core.LogStarted(ctx, logging.Logger, core.ProcessWorker, settings)
	if err := work(ctx, settings, logging); err != nil {
		core.LogFailed(context.Background(), logging.Logger, core.ProcessWorker, err)
		core.LogStopped(context.Background(), logging.Logger, core.ProcessWorker, "error")
		fmt.Fprintln(os.Stderr, "worker:", startup.Describe(err))
		return 1
	}
	core.LogStopped(context.Background(), logging.Logger, core.ProcessWorker, "shutdown")
	return 0
}

func work(ctx context.Context, settings core.Settings, logging *core.Logging) error {
	systemClock := clock.System{}
	database, err := core.Connect(ctx, settings, logging.Logger, systemClock.Now)
	if err != nil {
		return err
	}
	defer startup.CloseDatabase(database, logging.Logger)
	if err := startup.CheckMigrations(ctx, database.SQL, logging.Logger); err != nil {
		return err
	}
	if settings.Clock == core.ClockTest {
		<-ctx.Done()
		return nil
	}
	random := core.CryptoRandom()
	mailer := startup.NewMailer(logging.Logger, settings, database, random)
	background, err := startup.NewWorker(logging.Logger, settings, database, systemClock, mailer, random, storage.NewS3(settings.Storage))
	if err != nil {
		return errors.Join(err, mailer.Close())
	}
	return errors.Join(background.Run(ctx), mailer.Close())
}
