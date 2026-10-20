package schemas

import "ovc/backend/internal/domain"

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

type ErrorEnvelope struct {
	Body ErrorBody `json:"error"`
}

func NewErrorEnvelope(code domain.ErrorCode, requestID string, fields []FieldError) ErrorEnvelope {
	return ErrorEnvelope{Body: ErrorBody{Code: code, Message: code.Message(), RequestID: requestID, Fields: fields}}
}
