package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/core"
)

type echoInput struct {
	Body struct {
		Name string `json:"name" maxLength:"5"`
	}
}

type echoOutput struct {
	Body struct {
		Name string `json:"name"`
	}
}

func newBodyServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	humaAPI := newAPI(mux)
	huma.Register(humaAPI, operation("echo", http.MethodPost, "/echo"), func(_ context.Context, input *echoInput) (*echoOutput, error) {
		output := &echoOutput{}
		output.Body.Name = input.Body.Name
		return output, nil
	})
	handler := &outermost{next: mux, logger: slog.New(slog.DiscardHandler), now: clock.System{}.Now, newRequestID: core.NewRandomID}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

func postBody(t *testing.T, server *httptest.Server, body io.Reader) (int, string) {
	t.Helper()
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL+"/echo", body)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			t.Error(err)
		}
	}()
	text, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, string(text)
}

type decodedError struct {
	Error struct {
		Code   string `json:"code"`
		Fields []struct {
			Path string `json:"path"`
			Code string `json:"code"`
		} `json:"fields"`
	} `json:"error"`
}

func decode(t *testing.T, body string) decodedError {
	t.Helper()
	var decoded decodedError
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		t.Fatalf("not a structured error: %s", body)
	}
	return decoded
}

func TestBodyErrorsAreStructuredAndNeverEchoInput(t *testing.T) {
	server := newBodyServer(t)
	cases := map[string]struct {
		body   string
		status int
		code   string
	}{
		"malformed JSON":    {body: `{"name": "sentinel-value"`, status: http.StatusBadRequest, code: "bad_request"},
		"validation failed": {body: `{"name": "sentinel-value"}`, status: http.StatusUnprocessableEntity, code: "validation_failed"},
		"too large":         {body: `{"name": "` + strings.Repeat("s", 1<<20) + `"}`, status: http.StatusRequestEntityTooLarge, code: "payload_too_large"},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			status, body := postBody(t, server, strings.NewReader(testCase.body))

			decoded := decode(t, body)
			if status != testCase.status || decoded.Error.Code != testCase.code {
				t.Fatalf("got %d %s", status, body)
			}
			if strings.Contains(body, "sentinel") || strings.Contains(body, "sss") {
				t.Fatalf("the body echoes input: %s", body)
			}
		})
	}
}

func TestValidationNamesTheField(t *testing.T) {
	server := newBodyServer(t)

	status, body := postBody(t, server, strings.NewReader(`{"name": "too-long"}`))

	decoded := decode(t, body)
	if status != http.StatusUnprocessableEntity || len(decoded.Error.Fields) != 1 || decoded.Error.Fields[0].Path != "body.name" {
		t.Fatalf("got %d %s", status, body)
	}
}

func TestValidBodiesAreAccepted(t *testing.T) {
	server := newBodyServer(t)

	status, body := postBody(t, server, strings.NewReader(`{"name": "anna"}`))

	if status != http.StatusOK || strings.TrimSpace(body) != `{"name":"anna"}` {
		t.Fatalf("got %d %s", status, body)
	}
}

func TestEveryRegisteredOperationSetsTheBodyLimits(t *testing.T) {
	humaAPI := newAPI(http.NewServeMux())
	registerHealth(humaAPI, nil, slog.New(slog.DiscardHandler))

	var operations []*huma.Operation
	for _, item := range humaAPI.OpenAPI().Paths {
		for _, candidate := range []*huma.Operation{item.Get, item.Put, item.Post, item.Delete, item.Patch} {
			if candidate != nil {
				operations = append(operations, candidate)
			}
		}
	}

	if len(operations) == 0 {
		t.Fatal("no operations registered")
	}
	for _, registered := range operations {
		if registered.MaxBodyBytes != maxJSONBodyBytes || registered.BodyReadTimeout != bodyReadTimeout {
			t.Fatalf("%s has limits %d and %s", registered.OperationID, registered.MaxBodyBytes, registered.BodyReadTimeout)
		}
	}
}
