package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"ovc/backend/cmd/internal/startup"
	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/adapters/mail"
	"ovc/backend/internal/adapters/storage"
	"ovc/backend/internal/api"
	"ovc/backend/internal/core"
	"ovc/backend/internal/services/accounts"
	"ovc/backend/internal/testcontrol"
)

const (
	readHeaderTimeout = 10 * time.Second
	idleTimeout       = 120 * time.Second
	maxHeaderBytes    = 16 * 1024
	shutdownTimeout   = 40 * time.Second
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	command := ""
	if len(args) > 0 {
		command = args[0]
	}
	switch command {
	case "openapi":
		return writeOpenAPI()
	case "check-settings":
		if len(args) == 1 {
			return startup.CheckSettings(core.ProcessAPI, os.Stdout, os.Stderr)
		}
	case "migrate":
		return withProcess(migrate)
	case "ensure-bucket":
		if len(args) == 1 {
			return ensureBucket()
		}
	case "":
		return withProcess(serve)
	}
	return startup.Usage(os.Stderr, "usage: api [migrate|openapi|check-settings|ensure-bucket]")
}

func writeOpenAPI() int {
	document, err := api.OpenAPIDocument()
	if err != nil {
		fmt.Fprintln(os.Stderr, "openapi: the document could not be built")
		return 1
	}
	if _, err := os.Stdout.Write(document); err != nil {
		return 1
	}
	return 0
}

const ensureBucketTimeout = 30 * time.Second

func ensureBucket() int {
	settings, err := core.StorageFromEnvironment()
	if err != nil {
		fmt.Fprintln(os.Stderr, "configuration:", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), ensureBucketTimeout)
	defer cancel()
	if err := storage.NewS3(settings).EnsureBucket(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "ensure-bucket:", core.ErrorKind(err))
		return 1
	}
	if _, err := fmt.Fprintln(os.Stdout, "ok"); err != nil {
		return 1
	}
	return 0
}

type processFunc func(ctx context.Context, settings core.Settings, logging *core.Logging) error

func withProcess(body processFunc) int {
	settings, err := core.LoadFromEnvironment(core.ProcessAPI)
	if err != nil {
		fmt.Fprintln(os.Stderr, "configuration:", err)
		return 1
	}
	logging := core.NewLogging(os.Stdout, settings.LogLevel, settings.LogFormat)
	defer startup.CloseLogging(logging)
	ctx, stop := startup.SignalContext()
	defer stop()
	core.LogStarted(ctx, logging.Logger, core.ProcessAPI, settings)
	if err := body(ctx, settings, logging); err != nil {
		core.LogFailed(context.Background(), logging.Logger, core.ProcessAPI, err)
		core.LogStopped(context.Background(), logging.Logger, core.ProcessAPI, "error")
		fmt.Fprintln(os.Stderr, "api:", startup.Describe(err))
		return 1
	}
	core.LogStopped(context.Background(), logging.Logger, core.ProcessAPI, "shutdown")
	return 0
}

func migrate(ctx context.Context, settings core.Settings, logging *core.Logging) error {
	database, err := core.Connect(ctx, settings, logging.Logger, clock.System{}.Now)
	if err != nil {
		return err
	}
	defer startup.CloseDatabase(database, logging.Logger)
	return startup.Migrate(ctx, database.SQL, logging.Logger)
}

func serve(ctx context.Context, settings core.Settings, logging *core.Logging) error {
	systemClock := clock.System{}
	database, err := core.Connect(ctx, settings, logging.Logger, systemClock.Now)
	if err != nil {
		return err
	}
	defer startup.CloseDatabase(database, logging.Logger)
	if err := startup.CheckMigrations(ctx, database.SQL, logging.Logger); err != nil {
		return err
	}

	random := core.CryptoRandom()
	var shared *clock.Shared
	var source clock.Source = systemClock
	if settings.Clock == core.ClockTest {
		shared = clock.NewShared(database.Pool)
		source = shared
	}
	objects := storage.NewS3(settings.Storage)
	uploadLifetime, stopUploads := context.WithCancel(context.Background())
	defer stopUploads()
	fileService, err := startup.NewFiles(uploadLifetime, logging.Logger, settings, database, source, random, objects)
	if err != nil {
		return err
	}
	tutoringService, err := startup.NewTutoring(logging.Logger, settings, database, source, random)
	if err != nil {
		return err
	}
	accountService, err := startup.NewAccounts(logging.Logger, settings, database, source, random, fileService, tutoringService)
	if err != nil {
		return err
	}
	embed := &core.Switch{}
	scheduleService, err := startup.NewSchedule(logging.Logger, settings, database, source, random, fileService, embed)
	if err != nil {
		return err
	}
	preferences, err := startup.NewPreferences(logging.Logger, settings, database, source)
	if err != nil {
		return err
	}
	chatService := startup.NewChat(logging.Logger, database, source, random, fileService)
	libraryService, err := startup.NewLibrary(logging.Logger, settings, database, source, random, fileService)
	if err != nil {
		return err
	}
	homeworkService, err := startup.NewHomework(logging.Logger, settings, database, source, random, fileService, libraryService)
	if err != nil {
		return err
	}
	provider, err := startup.NewMeetingProvider(ctx, logging.Logger, database, source, random, settings.Presentation)
	if err != nil {
		return err
	}
	joinProvider, err := startup.NewJoinProvider(settings, provider)
	if err != nil {
		return err
	}
	lifecycle, err := startup.NewLifecycle(logging.Logger, settings, database, source, random, joinProvider, startup.NewPresentations(settings), embed)
	if err != nil {
		return err
	}
	live, err := startup.NewRealtime(logging.Logger, settings, database, source, random, accountService, chatService)
	if err != nil {
		return err
	}
	live.Gateway.Start(ctx, live.Listener)
	listening := core.Run(ctx, logging.Logger, "realtime.listener", func() error {
		live.Listener.Run(ctx)
		return nil
	})
	options := api.Options{
		Logger:       logging.Logger,
		Now:          systemClock.Now,
		NewRequestID: core.NewRandomID,
		Readiness: api.AllReady(
			api.ReadinessCheck{Name: "database", Check: database.Ping},
			api.ReadinessCheck{Name: "storage", Check: objects.Ping},
		),
		Files:       fileService,
		Tutoring:    tutoringService,
		Schedule:    scheduleService,
		Preferences: preferences,
		Inbox:       startup.NewInbox(logging.Logger, database, source),
		Chat:        chatService,
		Library:     libraryService,
		Homework:    homeworkService,
		Meetings:    lifecycle,
		Realtime:    live.Gateway,
		DocsEnabled: settings.APIDocsEnabled,
		Auth:        api.Auth{Accounts: accountService, PublicOrigin: settings.PublicURL, TrustedProxies: settings.TrustedProxies},
	}
	if settings.TestControl {
		sink := mail.NewSink(database.Pool, random)
		background, err := startup.NewWorker(ctx, logging.Logger, settings, database, source, sink, random, objects)
		if err != nil {
			return err
		}
		seeder, err := accounts.NewSeeder(accountService)
		if err != nil {
			return err
		}
		options.Mount = testcontrol.New(testcontrol.Dependencies{
			Logger: logging.Logger, Clock: shared, Worker: background, Sink: sink, Realtime: live.Gateway,
			Seeder: seeder, Room: provider, Embed: embed, FramingOrigin: settings.PublicURL,
		}).Mount
	}
	handler, err := api.NewHandler(options)
	if err != nil {
		return err
	}

	server := &http.Server{
		Addr:              settings.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
		ErrorLog:          logging.ServerErrorLog("http.server.error"),
	}
	serveErrors := core.Run(ctx, logging.Logger, "http-server", server.ListenAndServe)

	select {
	case err := <-serveErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve http: %w", err)
	case <-ctx.Done():
	}
	shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	stopUploadDeadline := context.AfterFunc(shutdownContext, stopUploads)
	defer stopUploadDeadline()
	live.Gateway.Shutdown(shutdownContext)
	if err := server.Shutdown(shutdownContext); err != nil {
		return fmt.Errorf("shut down http: %w", err)
	}
	if err := <-listening; err != nil {
		return fmt.Errorf("stop the realtime listener: %w", err)
	}
	return nil
}
