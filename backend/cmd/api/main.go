package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ovc/backend/cmd/internal/startup"
	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/api"
	"ovc/backend/internal/core"
	"ovc/backend/internal/testcontrol"
	"ovc/backend/internal/worker"
)

const (
	readHeaderTimeout = 10 * time.Second
	idleTimeout       = 120 * time.Second
	maxHeaderBytes    = 16 * 1024
	shutdownTimeout   = 20 * time.Second
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
	case "migrate":
		return withProcess(migrate)
	case "":
		return withProcess(serve)
	}
	fmt.Fprintln(os.Stderr, "usage: api [migrate|openapi]")
	return 2
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

type processFunc func(ctx context.Context, settings core.Settings, logging *core.Logging) error

func withProcess(body processFunc) int {
	settings, err := core.LoadFromEnvironment(core.ProcessAPI)
	if err != nil {
		fmt.Fprintln(os.Stderr, "configuration:", err)
		return 1
	}
	logging := core.NewLogging(os.Stdout, settings.LogLevel, settings.LogFormat)
	defer logging.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
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
	defer database.Close()
	return startup.Migrate(ctx, database.SQL, logging.Logger)
}

func serve(ctx context.Context, settings core.Settings, logging *core.Logging) error {
	systemClock := clock.System{}
	database, err := core.Connect(ctx, settings, logging.Logger, systemClock.Now)
	if err != nil {
		return err
	}
	defer database.Close()
	if err := startup.CheckMigrations(ctx, database.SQL, logging.Logger); err != nil {
		return err
	}

	options := api.Options{
		Logger:       logging.Logger,
		Now:          systemClock.Now,
		NewRequestID: core.NewRandomID,
		Readiness:    database.Ping,
		DocsEnabled:  settings.APIDocsEnabled,
	}
	if settings.TestControl {
		shared := clock.NewShared(database.Pool)
		routes := testcontrol.New(logging.Logger, shared, worker.New(logging.Logger, shared))
		options.Mount = routes.Mount
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
	if err := server.Shutdown(shutdownContext); err != nil {
		return fmt.Errorf("shut down http: %w", err)
	}
	return nil
}
