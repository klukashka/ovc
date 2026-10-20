package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/schemas"
)

const (
	maxJSONBodyBytes = 1 << 20
	bodyReadTimeout  = 30 * time.Second
	docsPath         = "GET /api/docs/openapi.json"
)

type ReadinessFunc func(ctx context.Context) error

type Options struct {
	Logger       *slog.Logger
	Now          func() time.Time
	NewRequestID func() string
	Readiness    ReadinessFunc
	DocsEnabled  bool
	Mount        func(mux *http.ServeMux)
}

func NewHandler(options Options) (http.Handler, error) {
	mux := http.NewServeMux()
	api := newAPI(mux)
	registerHealth(api, options.Readiness, options.Logger)
	if options.DocsEnabled {
		document, err := json.Marshal(api.OpenAPI())
		if err != nil {
			return nil, fmt.Errorf("encode the OpenAPI document: %w", err)
		}
		mux.HandleFunc(docsPath, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if _, err := w.Write(document); err != nil {
				return
			}
		})
	}
	if options.Mount != nil {
		options.Mount(mux)
	}
	return &outermost{next: mux, logger: options.Logger, now: options.Now, newRequestID: options.NewRequestID}, nil
}

func OpenAPIDocument() ([]byte, error) {
	api := newAPI(http.NewServeMux())
	registerHealth(api, nil, slog.New(slog.DiscardHandler))
	document, err := json.MarshalIndent(api.OpenAPI(), "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode the OpenAPI document: %w", err)
	}
	return append(document, '\n'), nil
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

func registerHealth(api huma.API, readiness ReadinessFunc, logger *slog.Logger) {
	huma.Register(api, operation("health-live", http.MethodGet, "/api/v1/health/live"),
		func(_ context.Context, _ *struct{}) (*liveOutput, error) {
			return &liveOutput{Body: schemas.LiveStatus{Status: "ok"}}, nil
		})
	huma.Register(api, operation("health-ready", http.MethodGet, "/api/v1/health/ready"),
		func(ctx context.Context, _ *struct{}) (*readyOutput, error) {
			if err := readiness(ctx); err != nil {
				logger.LogAttrs(ctx, slog.LevelWarn, "health.ready.failed", slog.String("kind", "database"))
				return nil, NewError(ctx, domain.ErrorServiceUnavailable)
			}
			return &readyOutput{Body: schemas.ReadyStatus{Status: "ready", Checks: schemas.ReadyChecks{Database: "ok"}}}, nil
		})
}
