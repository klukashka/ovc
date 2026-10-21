package schemas

import (
	"github.com/danielgtaylor/huma/v2"

	"ovc/backend/internal/domain"
)

type LiveStatus struct {
	Status string `json:"status" enum:"ok"`
}

type ReadyChecks struct {
	Database string `json:"database" enum:"ok"`
}

type ReadyStatus struct {
	Status string      `json:"status" enum:"ready"`
	Checks ReadyChecks `json:"checks"`
}

type FieldError struct {
	Path string `json:"path"`
	Code string `json:"code"`
}

type ErrorBody struct {
	Code      domain.ErrorCode `json:"code"`
	Message   string           `json:"message"`
	RequestID string           `json:"request_id"`
	Fields    []FieldError     `json:"fields,omitempty"`
}

func (ErrorBody) TransformSchema(_ huma.Registry, schema *huma.Schema) *huma.Schema {
	code, found := schema.Properties["code"]
	if !found {
		panic("ErrorBody schema has no code property")
	}
	code.Enum = code.Enum[:0]
	for _, errorCode := range domain.AllErrorCodes() {
		code.Enum = append(code.Enum, string(errorCode))
	}
	return schema
}

type ErrorEnvelope struct {
	Body ErrorBody `json:"error"`
}

func NewErrorEnvelope(code domain.ErrorCode, requestID string, fields []FieldError) ErrorEnvelope {
	listed := code.Listed()
	return ErrorEnvelope{Body: ErrorBody{Code: listed, Message: listed.Message(), RequestID: requestID, Fields: fields}}
}
