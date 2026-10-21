package startup

import (
	"log/slog"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/adapters/mail"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/notifications"
	"ovc/backend/internal/services/outbox"
	"ovc/backend/internal/worker"
)

const (
	emailLaneConcurrency = 10
	smtpTimeout          = 30 * time.Second
)

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
	queue := outbox.New(repositories.NewStore(database.Pool), random, logger)
	notifier, err := notifications.New(mailer, settings.PublicURL, map[domain.OutboxKind]notifications.Builder{})
	if err != nil {
		return nil, err
	}
	lanes := []worker.Lane{{Name: domain.LaneEmail, Concurrency: emailLaneConcurrency, Perform: notifier.Deliver}}
	jobs := []worker.Job{{
		Name:  domain.JobOutboxCleanup,
		Every: time.Hour,
		Run:   queue.Cleanup,
	}}
	return worker.New(logger, source, queue, lanes, jobs, worker.DefaultTiming()), nil
}
