package startup

import (
	"log/slog"
	"runtime"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/adapters/mail"
	"ovc/backend/internal/adapters/storage"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/accounts"
	"ovc/backend/internal/services/files"
	"ovc/backend/internal/services/notifications"
	"ovc/backend/internal/services/outbox"
	"ovc/backend/internal/worker"
)

const (
	hashWait              = 5 * time.Second
	emailLaneConcurrency  = 10
	deleteLaneConcurrency = 4
	smtpTimeout           = 30 * time.Second
)

func NewFiles(logger *slog.Logger, settings core.Settings, database *core.Database, source clock.Source, random core.RandomSource, objects storage.Store) (*files.Service, error) {
	keys, err := core.DeriveKeys(settings.JWTSecret)
	if err != nil {
		return nil, err
	}
	return files.New(files.Dependencies{
		Store: repositories.NewStore(database.Pool), Objects: objects, Clock: source, Random: random, Keys: keys, Logger: logger,
	}), nil
}

func NewAccounts(logger *slog.Logger, settings core.Settings, database *core.Database, source clock.Source, random core.RandomSource, avatars accounts.AvatarBinder) (*accounts.Service, error) {
	keys, err := core.DeriveKeys(settings.JWTSecret)
	if err != nil {
		return nil, err
	}
	hasher, err := core.NewPasswordHasher(core.ProductionArgon2Params(), random, runtime.GOMAXPROCS(0), hashWait)
	if err != nil {
		return nil, err
	}
	return accounts.New(accounts.Dependencies{
		Store:       repositories.NewStore(database.Pool),
		Clock:       source,
		Random:      random,
		Hasher:      hasher,
		Tokens:      core.NewAccessTokens(keys, settings.PublicURL, settings.AccessTokenTTL),
		Keys:        keys,
		RefreshTTL:  settings.RefreshTokenTTL,
		TutorQuotas: settings.TutorQuotas,
		Avatars:     avatars,
		Logger:      logger,
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

func NewWorker(logger *slog.Logger, settings core.Settings, database *core.Database, source clock.Source, mailer mail.Mailer, random core.RandomSource, objects storage.Store) (*worker.Worker, error) {
	store := repositories.NewStore(database.Pool)
	fileService := files.New(files.Dependencies{Store: store, Objects: objects, Clock: source, Random: random, Logger: logger})
	queue := outbox.New(store, random, logger)
	accountEmails, err := notifications.NewAccountEmails(store, random, settings.PublicURL)
	if err != nil {
		return nil, err
	}
	notifier, err := notifications.New(mailer, settings.PublicURL, accountEmails.Builders())
	if err != nil {
		return nil, err
	}
	lanes := []worker.Lane{
		{Name: domain.LaneEmail, Concurrency: emailLaneConcurrency, Perform: notifier.Deliver},
		{Name: domain.LaneStorageDelete, Concurrency: deleteLaneConcurrency, Perform: fileService.DeleteObject},
	}
	jobs := []worker.Job{
		{Name: domain.JobOutboxCleanup, Every: time.Hour, Run: queue.Cleanup},
		{Name: domain.JobRateLimitCleanup, Every: time.Hour, Run: accounts.CleanupRateLimits(store)},
		{Name: domain.JobUploadsExpire, Every: time.Hour, Run: fileService.ExpirePending},
	}
	return worker.New(logger, source, queue, lanes, jobs, worker.DefaultTiming()), nil
}
