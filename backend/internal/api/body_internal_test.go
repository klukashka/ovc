package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

type bodyServerLimits struct {
	bodyRead time.Duration
	read     time.Duration
	write    time.Duration
}

var defaultBodyServerLimits = bodyServerLimits{bodyRead: bodyReadTimeout, read: requestReadDeadline, write: requestWriteDeadline}

func newBodyServer(t *testing.T) *httptest.Server {
	t.Helper()
	return newBodyServerWithLimits(t, defaultBodyServerLimits)
}

func newBodyServerWithLimits(t *testing.T, limits bodyServerLimits) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	humaAPI := newAPI(mux)
	echo := operation("echo", http.MethodPost, "/echo")
	echo.BodyReadTimeout = limits.bodyRead
	huma.Register(humaAPI, echo, func(_ context.Context, input *echoInput) (*echoOutput, error) {
		output := &echoOutput{}
		output.Body.Name = input.Body.Name
		return output, nil
	})
	handler := &outermost{
		next:          mux,
		logger:        slog.New(slog.DiscardHandler),
		now:           clock.System{}.Now,
		newRequestID:  core.NewRandomID,
		readDeadline:  limits.read,
		writeDeadline: limits.write,
	}
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
		"unknown property":  {body: `{"name": "a", "sentinelkey": 1}`, status: http.StatusUnprocessableEntity, code: "validation_failed"},
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
	(&router{api: humaAPI, logger: slog.New(slog.DiscardHandler)}).registerAll(nil)

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
		wantBytes, wantTimeout := int64(maxJSONBodyBytes), bodyReadTimeout
		if registered.OperationID == "uploads-create" {
			wantBytes, wantTimeout = uploadBodyBytes, uploadReadDeadline
		}
		if registered.MaxBodyBytes != wantBytes || registered.BodyReadTimeout != wantTimeout {
			t.Fatalf("%s has limits %d and %s", registered.OperationID, registered.MaxBodyBytes, registered.BodyReadTimeout)
		}
	}
}

func TestUnknownPropertiesNeverEchoTheirNames(t *testing.T) {
	server := newBodyServer(t)
	for _, body := range []string{`{"name": "a", "sentinelkey": 1}`, `{"name": "a", "sentinel.dotted": 1}`, `{"name": "a", "sentinel[0]": 1}`} {
		t.Run(body, func(t *testing.T) {
			status, response := postBody(t, server, strings.NewReader(body))

			decoded := decode(t, response)
			if status != http.StatusUnprocessableEntity || len(decoded.Error.Fields) != 1 || strings.Contains(response, "sentinel") {
				t.Fatalf("got %d %s", status, response)
			}
			if decoded.Error.Fields[0].Path != "body" || decoded.Error.Fields[0].Code != "unexpected_property" {
				t.Fatalf("unexpected field %+v", decoded.Error.Fields[0])
			}
		})
	}
}

func TestUnsupportedMediaTypeCarriesNoFields(t *testing.T) {
	server := newBodyServer(t)
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL+"/echo", strings.NewReader("name"))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "text/plain")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	text, err := io.ReadAll(response.Body)
	if closeErr := response.Body.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}

	if err != nil {
		t.Fatal(err)
	}
	decoded := decode(t, string(text))
	if response.StatusCode != http.StatusUnsupportedMediaType || decoded.Error.Code != "unsupported_media_type" || len(decoded.Error.Fields) != 0 {
		t.Fatalf("got %d %s", response.StatusCode, text)
	}
}

func sendStalledBody(t *testing.T, server *httptest.Server) (string, time.Duration) {
	t.Helper()
	dialer := net.Dialer{}
	connection, err := dialer.DialContext(t.Context(), "tcp", strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := connection.Close(); err != nil {
			t.Error(err)
		}
	}()
	start := clock.System{}.Now()
	request := "POST /echo HTTP/1.1\r\nHost: test\r\nContent-Type: application/json\r\nContent-Length: 100\r\n\r\n{"
	if _, err := connection.Write([]byte(request)); err != nil {
		t.Fatal(err)
	}
	if err := connection.SetReadDeadline(start.Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	response, err := io.ReadAll(connection)
	if err != nil {
		var timeout net.Error
		if !errors.As(err, &timeout) || !timeout.Timeout() {
			t.Fatal(err)
		}
	}
	return string(response), clock.System{}.Now().Sub(start)
}

func TestAStalledBodyStillGetsAStructuredError(t *testing.T) {
	server := newBodyServerWithLimits(t, bodyServerLimits{bodyRead: 300 * time.Millisecond, read: 300 * time.Millisecond, write: 2 * time.Second})

	response, elapsed := sendStalledBody(t, server)

	if !strings.HasPrefix(response, "HTTP/1.1 400") || !strings.Contains(response, `"code":"bad_request"`) {
		t.Fatalf("expected a structured 400, got %q", response)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("the error arrived only after %s", elapsed)
	}
}

func TestUncleanPathsGetAStructuredNotFound(t *testing.T) {
	server := newBodyServer(t)
	for _, path := range []string{"//echo", "/x/../echo", "/./echo"} {
		t.Run(path, func(t *testing.T) {
			connection, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", strings.TrimPrefix(server.URL, "http://"))
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := connection.Close(); err != nil {
					t.Error(err)
				}
			}()

			if _, err := connection.Write([]byte("GET " + path + " HTTP/1.1\r\nHost: test\r\nConnection: close\r\n\r\n")); err != nil {
				t.Fatal(err)
			}
			response, err := io.ReadAll(connection)

			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(string(response), "HTTP/1.1 404") || !strings.Contains(string(response), `"code":"not_found"`) {
				t.Fatalf("expected a structured 404, got %q", response)
			}
		})
	}
}
