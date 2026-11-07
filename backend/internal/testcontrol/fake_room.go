package testcontrol

import (
	"context"
	"log/slog"
	"net/http"

	"ovc/backend/internal/adapters/meeting"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/schemas"
)

type fakeRoomRequest struct {
	Capability string `json:"capability"`
}

type fakeRoomResponse struct {
	Status          meeting.FakeRoomStatus `json:"status"`
	RoomName        string                 `json:"room_name,omitempty"`
	ParticipantName string                 `json:"participant_name,omitempty"`
	Role            domain.ParticipantRole `json:"role,omitempty"`
}

type availabilityRequest struct {
	Available *bool `json:"available"`
}

type availabilityResponse struct {
	Available bool `json:"available"`
}

type embedRequest struct {
	Allowed *bool `json:"allowed"`
}

type embedResponse struct {
	Allowed bool `json:"allowed"`
}

type fakeRoomOperation func(context.Context, string) (meeting.FakeRoomView, error)

func (routes *Routes) mountFakeRoom(mux *http.ServeMux) {
	mux.Handle("GET "+meeting.FakeRoomPath, routes.asset("text/html; charset=utf-8", fakeRoomPage))
	mux.Handle("GET "+meeting.FakeRoomPath+"/room.js", routes.asset("text/javascript; charset=utf-8", fakeRoomScript))
	mux.Handle("GET "+meeting.FakeRoomPath+"/room.css", routes.asset("text/css; charset=utf-8", fakeRoomStyle))
	mux.Handle("POST "+meeting.FakeRoomPath+"/state", routes.guard(routes.fakeRoom(routes.room.ViewRoom)))
	mux.Handle("POST "+meeting.FakeRoomPath+"/enter", routes.guard(routes.fakeRoom(routes.room.EnterRoom)))
	mux.Handle("POST "+meeting.FakeRoomPath+"/leave", routes.guard(routes.fakeRoom(routes.room.LeaveRoom)))
	mux.Handle("POST /__test__/fake-provider/end", routes.guard(routes.fakeRoom(routes.room.EndRoom)))
	mux.Handle("POST /__test__/fake-provider/availability", routes.guard(routes.setAvailability))
}

func (routes *Routes) asset(contentType string, body string) http.Handler {
	policy := "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src data:; base-uri 'none'; form-action 'none'; frame-ancestors " + routes.framingOrigin
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if !isLoopbackHost(r.Host) {
			writeError(ctx, routes.logger, w, domain.ErrorNotFound, nil)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", policy)
		if _, err := w.Write([]byte(body)); err != nil {
			routes.logger.LogAttrs(ctx, slog.LevelWarn, "http.response.write_failed", slog.String("kind", core.ErrorKind(err)))
		}
	})
}

func (routes *Routes) fakeRoom(run fakeRoomOperation) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request fakeRoomRequest
		if !decode(routes.logger, w, r, &request) {
			return
		}
		ctx := r.Context()
		view, err := run(ctx, request.Capability)
		if err != nil {
			routes.fail(ctx, w, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(ctx, routes.logger, w, fakeRoomResponse{
			Status: view.Status, RoomName: view.RoomName, ParticipantName: view.ParticipantName, Role: view.Role,
		})
	}
}

func (routes *Routes) setAvailability(w http.ResponseWriter, r *http.Request) {
	var request availabilityRequest
	if !decode(routes.logger, w, r, &request) {
		return
	}
	ctx := r.Context()
	if request.Available == nil {
		writeRequired(ctx, routes, w, "available")
		return
	}
	if err := routes.room.SimulateUnavailable(ctx, !*request.Available); err != nil {
		routes.fail(ctx, w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(ctx, routes.logger, w, availabilityResponse{Available: *request.Available})
}

func (routes *Routes) setEmbed(w http.ResponseWriter, r *http.Request) {
	var request embedRequest
	if !decode(routes.logger, w, r, &request) {
		return
	}
	ctx := r.Context()
	if request.Allowed == nil {
		writeRequired(ctx, routes, w, "allowed")
		return
	}
	routes.embed.Set(*request.Allowed)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(ctx, routes.logger, w, embedResponse{Allowed: *request.Allowed})
}

func writeRequired(ctx context.Context, routes *Routes, w http.ResponseWriter, path string) {
	writeError(ctx, routes.logger, w, domain.ErrorValidationFailed, []schemas.FieldError{{Path: path, Code: string(domain.FieldRequired)}})
}
