package startup

import (
	"context"
	"log/slog"
	"runtime"
	"time"
	_ "time/tzdata"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/adapters/listener"
	"ovc/backend/internal/adapters/mail"
	"ovc/backend/internal/adapters/meeting"
	"ovc/backend/internal/adapters/storage"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/realtime"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/accounts"
	"ovc/backend/internal/services/chat"
	"ovc/backend/internal/services/files"
	"ovc/backend/internal/services/library"
	"ovc/backend/internal/services/meetings"
	"ovc/backend/internal/services/notifications"
	"ovc/backend/internal/services/outbox"
	"ovc/backend/internal/services/schedule"
	"ovc/backend/internal/services/tutoring"
	"ovc/backend/internal/worker"
)

const (
	hashWait                = 5 * time.Second
	emailLaneConcurrency    = 10
	deleteLaneConcurrency   = 4
	providerLaneConcurrency = 4
	smtpTimeout             = 30 * time.Second
)

func NewFiles(lifetime context.Context, logger *slog.Logger, settings core.Settings, database *core.Database, source clock.Source, random core.RandomSource, objects storage.Store) (*files.Service, error) {
	keys, err := core.DeriveKeys(settings.JWTSecret)
	if err != nil {
		return nil, err
	}
	return files.New(files.Dependencies{
		Store: repositories.NewStore(database.Pool), Objects: objects, Clock: source, Random: random, Keys: keys, Logger: logger, Lifetime: lifetime,
	}), nil
}

func NewTutoring(logger *slog.Logger, settings core.Settings, database *core.Database, source clock.Source, random core.RandomSource) (*tutoring.Service, error) {
	keys, err := core.DeriveKeys(settings.JWTSecret)
	if err != nil {
		return nil, err
	}
	cipher, err := core.NewFieldCipher(settings.FieldKey, core.CryptoRandom())
	if err != nil {
		return nil, err
	}
	return tutoring.New(tutoring.Dependencies{
		Store: repositories.NewStore(database.Pool), Clock: source, Random: random, Keys: keys, Cipher: cipher,
		PublicURL: settings.PublicURL, Logger: logger,
	}), nil
}

func NewChat(logger *slog.Logger, database *core.Database, source clock.Source, random core.RandomSource, fileService *files.Service) *chat.Service {
	return chat.New(chat.Dependencies{
		Store: repositories.NewStore(database.Pool), Clock: source, Random: random, Addresses: fileService, Erasure: chat.DefaultErasureSettings(),
		Logger: logger,
	})
}

func NewLibrary(
	logger *slog.Logger, settings core.Settings, database *core.Database, source clock.Source, random core.RandomSource, fileService *files.Service,
) (*library.Service, error) {
	keys, err := core.DeriveKeys(settings.JWTSecret)
	if err != nil {
		return nil, err
	}
	return library.New(library.Dependencies{
		Store: repositories.NewStore(database.Pool), Clock: source, Random: random, Keys: keys, Addresses: fileService, Logger: logger,
	}), nil
}

func NewSchedule(
	logger *slog.Logger, settings core.Settings, database *core.Database, source clock.Source, random core.RandomSource, fileService *files.Service,
	embed *core.Switch,
) (*schedule.Service, error) {
	keys, err := core.DeriveKeys(settings.JWTSecret)
	if err != nil {
		return nil, err
	}
	return schedule.New(schedule.Dependencies{Store: repositories.NewStore(database.Pool), Clock: source, Random: random, Keys: keys, Logger: logger, Addresses: fileService, Embed: embed}), nil
}

func NewAccounts(
	logger *slog.Logger, settings core.Settings, database *core.Database, source clock.Source, random core.RandomSource,
	fileService *files.Service, tutoringService *tutoring.Service,
) (*accounts.Service, error) {
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
		Avatars:     fileService,
		Uploads:     fileService,
		Tutoring:    tutoringService,
		Logger:      logger,
	}), nil
}

func NewPreferences(logger *slog.Logger, settings core.Settings, database *core.Database, source clock.Source) (*notifications.Preferences, error) {
	keys, err := core.DeriveKeys(settings.JWTSecret)
	if err != nil {
		return nil, err
	}
	tokens, err := core.NewUnsubscribeTokens(settings.UnsubscribeKey)
	if err != nil {
		return nil, err
	}
	return notifications.NewPreferences(notifications.PreferenceDependencies{
		Store: repositories.NewStore(database.Pool), Clock: source, Tokens: tokens, Keys: keys, Logger: logger,
	}), nil
}

func NewInbox(logger *slog.Logger, database *core.Database, source clock.Source) *notifications.Inbox {
	return notifications.NewInbox(notifications.InboxDependencies{Store: repositories.NewStore(database.Pool), Clock: source, Logger: logger})
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

func NewWorker(ctx context.Context, logger *slog.Logger, settings core.Settings, database *core.Database, source clock.Source, mailer mail.Mailer, random core.RandomSource, objects storage.Store) (*worker.Worker, error) {
	store := repositories.NewStore(database.Pool)
	provider, err := NewMeetingProvider(ctx, logger, database, source, random, "")
	if err != nil {
		return nil, err
	}
	meetingService := meetings.New(store, provider, domain.MeetingProviderFake, logger)
	lifecycle, err := NewLifecycle(logger, settings, database, source, random, provider, meetings.NoPresentations{}, nil)
	if err != nil {
		return nil, err
	}
	meetingScanner := meetings.NewRecoveryScanner(store, random, logger, clock.System{}.Now)
	cipher, err := core.NewFieldCipher(settings.FieldKey, core.CryptoRandom())
	if err != nil {
		return nil, err
	}
	fileService := files.New(files.Dependencies{Store: store, Objects: objects, Clock: source, Random: random, Logger: logger})
	tutoringService := tutoring.New(tutoring.Dependencies{Store: store, Clock: source, Random: random, Cipher: cipher, Logger: logger})
	purger := accounts.NewPurger(store, fileService, tutoringService, logger)
	inbox := notifications.NewInbox(notifications.InboxDependencies{Store: store, Clock: source, Logger: logger})
	chatService := chat.New(chat.Dependencies{
		Store: store, Clock: source, Random: random, Addresses: fileService, Erasure: chat.DefaultErasureSettings(), Logger: logger,
	})
	queue := outbox.New(store, random, logger)
	accountEmails, err := notifications.NewAccountEmails(store, random, settings.PublicURL)
	if err != nil {
		return nil, err
	}
	relationshipEmails, err := notifications.NewRelationshipEmails(store, cipher, settings.PublicURL)
	if err != nil {
		return nil, err
	}
	unsubscribeTokens, err := core.NewUnsubscribeTokens(settings.UnsubscribeKey)
	if err != nil {
		return nil, err
	}
	lessonEmails, err := notifications.NewLessonEmails(store, unsubscribeTokens, settings.PublicURL)
	if err != nil {
		return nil, err
	}
	digestEmails, err := notifications.NewDigestEmails(store, unsubscribeTokens, settings.PublicURL)
	if err != nil {
		return nil, err
	}
	builders, err := notifications.Combine(accountEmails.Builders(), relationshipEmails.Builders(), lessonEmails.Builders(), digestEmails.Builders())
	if err != nil {
		return nil, err
	}
	keys, err := core.DeriveKeys(settings.JWTSecret)
	if err != nil {
		return nil, err
	}
	notifier, err := notifications.New(mailer, settings.PublicURL, builders, notifications.Dispatch{Store: store, Keys: keys, Logger: logger})
	if err != nil {
		return nil, err
	}
	lessonJobs := schedule.NewJobs(
		schedule.New(schedule.Dependencies{Store: store, Clock: source, Random: random, Keys: keys, Logger: logger, Addresses: fileService}),
		schedule.DefaultJobSettings(),
	)
	lanes := []worker.Lane{
		{Name: domain.LaneProviderCall, Concurrency: providerLaneConcurrency, PerformMeetingRecovery: meetingService.Cleanup, Perform: lifecycle.CheckEndedHint},
		{Name: domain.LaneEmail, Concurrency: emailLaneConcurrency, Perform: notifier.Deliver},
		{Name: domain.LaneStorageDelete, Concurrency: deleteLaneConcurrency, Perform: fileService.DeleteObject},
	}
	jobs := []worker.Job{
		{Name: domain.JobMeetingRecovery, Every: time.Minute, Run: meetingScanner.Run},
		{Name: domain.JobOutboxCleanup, Every: time.Hour, Run: queue.Cleanup},
		{Name: domain.JobRateLimitCleanup, Every: time.Hour, Run: accounts.CleanupRateLimits(store)},
		{Name: domain.JobSessionsCleanup, Every: time.Hour, Run: accounts.CleanupSessions(store)},
		{Name: domain.JobEmailRequestsCleanup, Every: time.Hour, Run: accounts.CleanupEmailRequests(store)},
		{Name: domain.JobUploadsExpire, Every: time.Hour, Run: fileService.ExpirePending},
		{Name: domain.JobDailyUploadsCleanup, Every: time.Minute, Run: fileService.CleanupDailyUploads},
		{Name: domain.JobUploadsRecover, Every: time.Minute, Run: fileService.RecoverUploads},
		{Name: domain.JobAccountsPurge, Every: time.Hour, Run: purger.PurgeDeleted},
		{Name: domain.JobAccountsCleanupUnverified, Every: time.Hour, Run: purger.CleanupUnverified},
		{Name: domain.JobInvitationsExpire, Every: time.Hour, Run: tutoringService.ExpireInvitations},
		{Name: domain.JobReclaimExpire, Every: time.Hour, Run: tutoringService.ExpireReclaims},
		{Name: domain.JobJoinRequestsExpire, Every: time.Hour, Run: tutoringService.ExpireJoinRequests},
		{Name: domain.JobNotificationsCleanup, Every: time.Hour, Run: inbox.Cleanup},
		{Name: domain.JobMessagesDigest, Every: time.Minute, Run: chatService.QueueDigests},
		{Name: domain.JobEraseChannels, Every: time.Minute, Run: chatService.EraseChannels},
		{Name: domain.JobExpirePresence, Every: time.Minute, Run: chatService.ExpirePresence},
		{Name: domain.JobLessonStatus, Every: time.Minute, Run: lessonJobs.EndDue},
		{Name: domain.JobLessonReminders, Every: time.Minute, Run: lessonJobs.QueueReminders},
		{Name: domain.JobSeriesExtension, Every: time.Hour, OwnLoop: true, Run: lessonJobs.ExtendHorizons},
	}
	return worker.New(logger, source, queue, lanes, jobs, worker.DefaultTiming()), nil
}

type Realtime struct {
	Gateway  *realtime.Gateway
	Listener *listener.Listener
}

func NewRealtime(
	logger *slog.Logger, settings core.Settings, database *core.Database, source clock.Source, random core.RandomSource, sessions realtime.Sessions,
	chatService realtime.Chat,
) (Realtime, error) {
	gateway, err := realtime.New(realtime.Dependencies{
		Sessions: sessions, Chat: chatService, Scheduler: realtime.SystemScheduler{}, Clock: source, Now: clock.System{}.Now, NewID: core.NewRandomID,
		Logger:       logger,
		PublicOrigin: settings.PublicURL, Timing: realtime.DefaultTiming(), Limits: realtime.DefaultLimits(),
	})
	if err != nil {
		return Realtime{}, err
	}
	instance, err := core.NewID[domain.InstanceID](random)
	if err != nil {
		return Realtime{}, err
	}
	adapter, err := listener.New(listener.Dependencies{
		DatabaseURL: settings.DatabaseURL, Instance: instance, Heartbeats: listener.ConnectionHeartbeats{},
		Consumer: gateway, Logger: logger, Timing: listener.DefaultTiming(),
	})
	if err != nil {
		return Realtime{}, err
	}
	return Realtime{Gateway: gateway, Listener: adapter}, nil
}

func NewLifecycle(
	logger *slog.Logger, settings core.Settings, database *core.Database, source clock.Source, random core.RandomSource, provider meeting.Provider,
	presentations meetings.PresentationSigner, embed *core.Switch,
) (*meetings.Lifecycle, error) {
	keys, err := core.DeriveKeys(settings.JWTSecret)
	if err != nil {
		return nil, err
	}
	store := repositories.NewStore(database.Pool)
	timing := meetings.DefaultRoomTiming()
	rooms := meetings.NewRooms(meetings.RoomDependencies{
		Store: store, Provider: provider, Presentations: presentations, Kind: domain.MeetingProviderFake, Clock: source, Random: random, Logger: logger,
		Monotonic: clock.System{}.Now, Timing: timing,
	})
	return meetings.NewLifecycle(meetings.LifecycleDependencies{
		Rooms: rooms, Store: store, Provider: provider, Clock: source, Random: random, Keys: keys, Logger: logger,
		Enabled: settings.TestControl, Embed: embed, ProviderCall: timing.ProviderCall,
	}), nil
}

func NewMeetingProvider(
	ctx context.Context, logger *slog.Logger, database *core.Database, source clock.Source, random core.RandomSource, presentationOrigin string,
) (*meeting.SharedFake, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return meeting.NewSharedFakeReceivingPresentations(ctx, database.Pool, source, random, logger, meeting.DefaultFakeLimits(), presentationOrigin)
}

func NewPresentations(settings core.Settings) *storage.Presigner {
	return storage.NewPresigner(settings.Storage, settings.Presentation)
}

func NewJoinProvider(settings core.Settings, provider *meeting.SharedFake) (meeting.Provider, error) {
	if !settings.TestControl {
		return provider, nil
	}
	origin := settings.FakeRoomOrigin
	if origin == "" {
		origin = "http://" + settings.HTTPAddr
	}
	return meeting.NewNavigableFake(provider, origin)
}
