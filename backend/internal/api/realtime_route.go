package api

import (
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/realtime"
)

const (
	realtimePath       = "/api/v1/ws"
	realtimeRetryAfter = "5"
	websocketVersion   = "13"
	websocketKeyBytes  = 16
)

func exactOrigin(values []string, origin string) bool {
	return len(values) == 1 && values[0] == origin
}

type realtimeRoute struct {
	gateway        *realtime.Gateway
	origin         string
	trustedProxies []netip.Prefix
	logger         *slog.Logger
}

func (r *router) registerRealtimeAudience() {
	r.operations = append(r.operations, OperationAudience{
		ID: "realtime-socket", Method: http.MethodGet, Path: realtimePath, Audience: AudienceFirstFrame,
	})
}

func (route *realtimeRoute) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !exactOrigin(r.Header.Values("Origin"), route.origin) {
		route.refuse(w, r, "origin", domain.ErrorForbidden)
		return
	}
	if !isWebSocketUpgrade(r) {
		route.refuse(w, r, "not_upgrade", domain.ErrorBadRequest)
		return
	}
	if route.gateway == nil {
		w.Header().Set("Retry-After", realtimeRetryAfter)
		route.refuse(w, r, "busy", domain.ErrorServiceUnavailable)
		return
	}
	address, known := ClientAddress(r.RemoteAddr, r.Header.Values("X-Forwarded-For"), route.trustedProxies)
	if !known {
		route.logger.LogAttrs(ctx, slog.LevelError, "http.client_address.unknown", slog.String("kind", "peer"))
		writeError(ctx, route.logger, w, domain.ErrorInternal)
		return
	}
	reservation, refusal := route.gateway.Reserve(address)
	if refusal != realtime.UpgradeAccepted {
		w.Header().Set("Retry-After", realtimeRetryAfter)
		route.refuse(w, r, string(refusal), domain.ErrorServiceUnavailable)
		return
	}
	controller := http.NewResponseController(w)
	if err := errors.Join(controller.SetReadDeadline(time.Time{}), controller.SetWriteDeadline(time.Time{})); err != nil &&
		!errors.Is(err, http.ErrNotSupported) {
		route.logger.LogAttrs(ctx, slog.LevelWarn, "http.deadline.failed", slog.String("kind", "clear"))
	}
	route.gateway.Serve(w, r, reservation)
}

func (route *realtimeRoute) refuse(w http.ResponseWriter, r *http.Request, reason string, code domain.ErrorCode) {
	route.logger.LogAttrs(r.Context(), slog.LevelInfo, "ws.upgrade.refused", slog.String("reason", reason))
	writeError(r.Context(), route.logger, w, code)
}

func isWebSocketUpgrade(r *http.Request) bool {
	if r.Method != http.MethodGet || !r.ProtoAtLeast(1, 1) || !headerHasToken(r.Header, "Connection", "upgrade") || !headerHasToken(r.Header, "Upgrade", "websocket") {
		return false
	}
	if r.Header.Get("Sec-WebSocket-Version") != websocketVersion {
		return false
	}
	keys := r.Header.Values("Sec-WebSocket-Key")
	if len(keys) != 1 {
		return false
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(keys[0]))
	return err == nil && len(key) == websocketKeyBytes
}

func headerHasToken(header http.Header, name string, token string) bool {
	for _, value := range header.Values(name) {
		for _, part := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(part), token) {
				return true
			}
		}
	}
	return false
}
