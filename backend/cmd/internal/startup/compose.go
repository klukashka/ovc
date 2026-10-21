package startup

import (
	"log/slog"
	"runtime"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/adapters/mail"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/accounts"
	"ovc/backend/internal/services/notifications"
	"ovc/backend/internal/services/outbox"
	"ovc/backend/internal/worker"
)

const (
	hashWait             = 5 * time.Second
	emailLaneConcurrency = 10
	smtpTimeout          = 30 * time.Second
)

func NewAccounts(logger *slog.Logger, settings core.Settings, database *core.Database, source clock.Source, random core.RandomSource) (*accounts.Service, error) {
	keys, err := core.DeriveKeys(settings.JWTSecret)
	if err != nil {
		return nil, err
	}
	hasher, err := core.NewPasswordHasher(core.ProductionArgon2Params(), random, runtime.GOMAXPROCS(0), hashWait)
	if err != nil {
		return nil, err
	}
	return accounts.New(accounts.Dependencies{
		Store:      repositories.NewStore(database.Pool),
		Clock:      source,
		Random:     random,
		Hasher:     hasher,
		Tokens:     core.NewAccessTokens(keys, settings.PublicURL, settings.AccessTokenTTL),
		Keys:       keys,
		RefreshTTL: settings.RefreshTokenTTL,
		Logger:     logger,
	}), nil
}

type Mailer struct {
	mail.Mailer
	Close func() error
}

func NewMailer(logger *slog.Logger, settings core.Settings, database *core.Database, random core.RandomSource) Mailer {
	if settings.MailBackend == core.MailBackendSMTP {
		smtp := mail.NewSMTP(mail.SMTPConfig{
			Host:     settings.SMTP.Host,
			Port:     settings.SMTP.Port,
			TLS:      settings.SMTP.TLS,
			Username: settings.SMTP.Username,
			Password: settings.SMTP.Password,
			From:     settings.MailFrom,
			PoolSize: emailLaneConcurrency,
			Timeout:  smtpTimeout,
			Logger:   logger,
		})
		return Mailer{Mailer: smtp, Close: smtp.Close}
	}
	return Mailer{Mailer: mail.NewSink(database.Pool, random), Close: func() error { return nil }}
}

func NewWorker(logger *slog.Logger, settings core.Settings, database *core.Database, source clock.Source, mailer mail.Mailer, random core.RandomSource) (*worker.Worker, error) {
	store := repositories.NewStore(database.Pool)
	queue := outbox.New(store, random, logger)
	notifier, err := notifications.New(mailer, settings.PublicURL, map[domain.OutboxKind]notifications.Builder{})
	if err != nil {
		return nil, err
	}
	lanes := []worker.Lane{{Name: domain.LaneEmail, Concurrency: emailLaneConcurrency, Perform: notifier.Deliver}}
	jobs := []worker.Job{
		{Name: domain.JobOutboxCleanup, Every: time.Hour, Run: queue.Cleanup},
		{Name: domain.JobRateLimitCleanup, Every: time.Hour, Run: accounts.CleanupRateLimits(store)},
	}
	return worker.New(logger, source, queue, lanes, jobs, worker.DefaultTiming()), nil
}
