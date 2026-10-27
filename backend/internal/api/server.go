package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/realtime"
	"ovc/backend/internal/schemas"
	"ovc/backend/internal/services/files"
	"ovc/backend/internal/services/notifications"
	"ovc/backend/internal/services/schedule"
	"ovc/backend/internal/services/tutoring"
)

const (
	maxJSONBodyBytes = 1 << 20
	bodyReadTimeout  = 30 * time.Second
	docsPath         = "GET /api/docs/openapi.json"
)

type ReadinessFunc func(ctx context.Context) error

const readinessCheckTimeout = time.Second

type ReadinessCheck struct {
	Name  string
	Check ReadinessFunc
}

type ReadinessFailure struct {
	Check string
	Err   error
}

func (e *ReadinessFailure) Error() string {
	return e.Check + " is not ready: " + e.Err.Error()
}

func (e *ReadinessFailure) Unwrap() error {
	return e.Err
}

func AllReady(checks ...ReadinessCheck) ReadinessFunc {
	return func(ctx context.Context) error {
		for _, check := range checks {
			checkContext, cancel := context.WithTimeout(ctx, readinessCheckTimeout)
			err := check.Check(checkContext)
			cancel()
			if err != nil {
				return &ReadinessFailure{Check: check.Name, Err: err}
			}
		}
		return nil
	}
}

type Options struct {
	Logger        *slog.Logger
	Now           func() time.Time
	NewRequestID  func() string
	Readiness     ReadinessFunc
	DocsEnabled   bool
	Mount         func(mux *http.ServeMux)
	ReadDeadline  time.Duration
	WriteDeadline time.Duration
	Auth          Auth
	Files         *files.Service
	Tutoring      *tutoring.Service
	Schedule      *schedule.Service
	Preferences   *notifications.Preferences
	Inbox         *notifications.Inbox
	Realtime      *realtime.Gateway
}

func NewHandler(options Options) (http.Handler, error) {
	return newHandler(options, func(*router) {})
}

func newHandler(options Options, registerExtra func(*router)) (http.Handler, error) {
	mux := http.NewServeMux()
	api := newAPI(mux)
	readiness := &sharedReadiness{check: options.Readiness, now: options.Now, logger: options.Logger}
	routes := &router{
		api: api, auth: options.Auth, files: options.Files, tutoring: options.Tutoring, schedule: options.Schedule, preferences: options.Preferences,
		inbox: options.Inbox,
		now:   options.Now, logger: options.Logger,
	}
	routes.registerAll(readiness.run)
	registerExtra(routes)
	if options.DocsEnabled {
		document, err := json.Marshal(api.OpenAPI())
		if err != nil {
			return nil, fmt.Errorf("encode the OpenAPI document: %w", err)
		}
		mux.HandleFunc(docsPath, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if _, err := w.Write(document); err != nil {
				options.Logger.LogAttrs(r.Context(), slog.LevelWarn, "http.response.write_failed", slog.String("kind", core.ErrorKind(err)))
			}
		})
	}
	mux.Handle(http.MethodGet+" "+realtimePath, &realtimeRoute{
		gateway: options.Realtime, origin: options.Auth.PublicOrigin, trustedProxies: options.Auth.TrustedProxies, logger: options.Logger,
	})
	if options.Mount != nil {
		options.Mount(mux)
	}
	return &outermost{
		next:          mux,
		logger:        options.Logger,
		now:           options.Now,
		newRequestID:  options.NewRequestID,
		readDeadline:  durationOr(options.ReadDeadline, requestReadDeadline),
		writeDeadline: durationOr(options.WriteDeadline, requestWriteDeadline),
	}, nil
}

func durationOr(value time.Duration, fallback time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return fallback
}

func OpenAPIDocument() ([]byte, error) {
	api := newAPI(http.NewServeMux())
	(&router{api: api, logger: slog.New(slog.DiscardHandler)}).registerAll(nil)
	document, err := json.MarshalIndent(api.OpenAPI(), "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode the OpenAPI document: %w", err)
	}
	return append(document, '\n'), nil
}

func Operations() []OperationAudience {
	routes := &router{api: newAPI(http.NewServeMux()), logger: slog.New(slog.DiscardHandler)}
	routes.registerAll(nil)
	return routes.operations
}

func newAPI(mux *http.ServeMux) huma.API {
	installErrorModel()
	config := huma.DefaultConfig("OVC API", "0.1.0")
	config.OpenAPIPath = ""
	config.DocsPath = ""
	config.SchemasPath = ""
	config.CreateHooks = nil
	config.Formats = map[string]huma.Format{"application/json": huma.DefaultJSONFormat, "json": huma.DefaultJSONFormat}
	config.DefaultFormat = "application/json"
	return humago.New(mux, config)
}

func operation(id string, method string, path string) huma.Operation {
	return huma.Operation{
		OperationID:     id,
		Method:          method,
		Path:            path,
		MaxBodyBytes:    maxJSONBodyBytes,
		BodyReadTimeout: bodyReadTimeout,
	}
}

type liveOutput struct {
	Body schemas.LiveStatus
}

type readyOutput struct {
	Body schemas.ReadyStatus
}

func (r *router) registerAll(readiness ReadinessFunc) {
	r.registerHealth(readiness)
	r.registerAuth()
	r.registerTutoring()
	r.registerSchedule()
	r.registerRealtimeAudience()
}

func failedCheck(err error) string {
	var failure *ReadinessFailure
	if errors.As(err, &failure) {
		return failure.Check
	}
	return "unknown"
}

func (r *router) registerHealth(readiness ReadinessFunc) {
	logger := r.logger
	register(r, operation("health-live", http.MethodGet, "/api/v1/health/live"), AudiencePublic,
		func(_ context.Context, _ *struct{}) (*liveOutput, error) {
			return &liveOutput{Body: schemas.LiveStatus{Status: "ok"}}, nil
		})
	register(r, operation("health-ready", http.MethodGet, "/api/v1/health/ready"), AudiencePublic,
		func(ctx context.Context, _ *struct{}) (*readyOutput, error) {
			if err := readiness(ctx); err != nil {
				if ctx.Err() != nil {
					logger.LogAttrs(ctx, slog.LevelDebug, "health.ready.abandoned", slog.String("kind", "canceled"))
				} else {
					logger.LogAttrs(ctx, slog.LevelWarn, "health.ready.failed", slog.String("kind", failedCheck(err)))
				}
				return nil, NewError(ctx, domain.ErrorServiceUnavailable)
			}
			return &readyOutput{Body: schemas.ReadyStatus{Status: "ready", Checks: schemas.ReadyChecks{Database: "ok", Storage: "ok"}}}, nil
		})
}
