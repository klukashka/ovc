package api

import (
	"context"
	"net/http"
)

const BodyProbePath = "/api/v1/tutor/body-probe"

type bodyProbeInput struct {
	Body struct {
		Name  string `json:"name" minLength:"1" maxLength:"10"`
		Count int    `json:"count"`
	}
}

func NewHandlerWithBodyProbe(options Options) (http.Handler, error) {
	return newHandler(options, func(r *router) {
		probe := operation("body-probe", http.MethodPost, BodyProbePath)
		register(r, probe, AudienceTutor, func(context.Context, *bodyProbeInput) (*struct{}, error) { return &struct{}{}, nil })
	})
}
