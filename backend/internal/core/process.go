package core

import (
	"context"
	"log/slog"
)

func LogStarted(ctx context.Context, logger *slog.Logger, process Process, settings Settings) {
	database := slog.String("database", "unparseable")
	if summary, err := SummarizeDatabase(settings.DatabaseURL); err == nil {
		database = slog.Any("database", summary)
	}
	logger.LogAttrs(ctx, slog.LevelInfo, "process.started",
		slog.String("process", string(process)),
		slog.String("http_addr", settings.HTTPAddr),
		database,
		slog.Int("db_pool_size", int(settings.DBPoolSize)),
		slog.String("log_level", settings.LogLevel.String()),
		slog.String("log_format", string(settings.LogFormat)),
		slog.Bool("api_docs_enabled", settings.APIDocsEnabled),
		slog.Bool("test_control", settings.TestControl),
		slog.String("clock", string(settings.Clock)),
		slog.String("meeting_provider", string(settings.MeetingProvider)),
		slog.String("mail_backend", string(settings.MailBackend)),
	)
}

func LogFailed(ctx context.Context, logger *slog.Logger, process Process, err error) {
	logger.LogAttrs(ctx, slog.LevelError, "process.failed", slog.String("process", string(process)), slog.String("kind", ErrorKind(err)))
}

func LogStopped(ctx context.Context, logger *slog.Logger, process Process, reason string) {
	logger.LogAttrs(ctx, slog.LevelInfo, "process.stopped", slog.String("process", string(process)), slog.String("reason", reason))
}
