package meeting

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/testdb"
)

var sharedFakeChild = flag.Bool("shared-fake-child", false, "Run the shared provider fixture child")

const sharedProcessBytes = 8192

type sharedProcessRequest struct {
	DatabaseURL string             `json:"database_url"`
	Meeting     domain.Meeting     `json:"meeting"`
	Participant domain.Participant `json:"participant"`
	Address     string             `json:"address"`
}

type sharedProcessResponse struct {
	Before  domain.MeetingInfo `json:"before"`
	After   domain.MeetingInfo `json:"after"`
	Failure Failure            `json:"failure"`
}

type boundedProcessOutput struct{ buffer bytes.Buffer }

func (output *boundedProcessOutput) Write(value []byte) (int, error) {
	if len(value) > sharedProcessBytes-output.buffer.Len() {
		return 0, errors.New("child output exceeded its bound")
	}
	return output.buffer.Write(value)
}

func TestSharedFakeAcrossProcesses(t *testing.T) {
	if *sharedFakeChild {
		runSharedFakeChild(t)
		return
	}
	database := testdb.New(t)
	provider := newShared(t, database.Pool, testLimits(), 1)
	room := createdMeeting(t, provider)
	participant := testParticipant()
	link := issuedLink(t, provider, room, participant)
	request := sharedProcessRequest{DatabaseURL: database.URL.Reveal(), Meeting: room, Participant: participant, Address: link.Reveal()}

	entered := invokeSharedFakeChild(t, request)
	parentInfo, infoErr := provider.MeetingInfo(t.Context(), room)
	endErr := provider.EndMeeting(t.Context(), room)
	ended := invokeSharedFakeChild(t, request)
	finalInfo, finalErr := provider.MeetingInfo(t.Context(), room)

	requireNoErrors(t, infoErr, endErr, finalErr)
	if entered.Before != (domain.MeetingInfo{Status: domain.MeetingCreated, CreateTime: room.CreateTime}) || entered.After != (domain.MeetingInfo{Status: domain.MeetingRunning, CreateTime: room.CreateTime, Participants: 1}) || entered.Failure != "" || parentInfo != entered.After {
		t.Fatal("child did not share the exact parent generation and presence")
	}
	if ended.Failure != FailureNotFound || ended.Before != (domain.MeetingInfo{Status: domain.MeetingNotFound}) || ended.After != ended.Before || finalInfo != ended.After {
		t.Fatal("child retained an ended credential")
	}
}

func invokeSharedFakeChild(t *testing.T, request sharedProcessRequest) sharedProcessResponse {
	t.Helper()
	input, err := json.Marshal(request)
	requireNoErrors(t, err)
	if len(input) > sharedProcessBytes {
		t.Fatal("child request exceeded its bound")
	}
	executable, executableErr := os.Executable()
	requireNoErrors(t, executableErr)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, "-test.run=^TestSharedFakeAcrossProcesses$", "-shared-fake-child", "-test.timeout=8s")
	command.Stdin = bytes.NewReader(input)
	var output boundedProcessOutput
	command.Stdout, command.Stderr = &output, &output
	command.WaitDelay = time.Second
	if err := command.Run(); err != nil {
		t.Fatal("shared provider child failed; its private output was withheld")
	}
	var response sharedProcessResponse
	decoder := json.NewDecoder(&output.buffer)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil {
		t.Fatal("shared provider child returned invalid structured output")
	}
	trailer, trailerErr := io.ReadAll(io.MultiReader(decoder.Buffered(), &output.buffer))
	if trailerErr != nil || strings.TrimSpace(string(trailer)) != "PASS" {
		t.Fatal("shared provider child returned unexpected harness output")
	}
	return response
}

func runSharedFakeChild(t *testing.T) {
	t.Helper()
	input, err := io.ReadAll(io.LimitReader(os.Stdin, sharedProcessBytes+1))
	if err != nil || len(input) > sharedProcessBytes {
		t.Fatal("invalid fixture input length")
	}
	var request sharedProcessRequest
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		t.Fatal("invalid fixture input")
	}
	parsed, parseErr := url.Parse(request.DatabaseURL)
	if parseErr != nil || !regexp.MustCompile(`^/ovc_[0-9a-f]{12}_test$`).MatchString(parsed.Path) {
		t.Fatal("invalid fixture database name")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	databaseURL := core.NewSecret(request.DatabaseURL)
	if err := core.VerifyLocalDatabase(ctx, databaseURL); err != nil {
		t.Fatal("fixture database is not local")
	}
	pool := testdb.OpenPool(t, databaseURL)
	provider, providerErr := NewSharedFake(ctx, pool, sharedClock(), sharedRandom(2), testLogger(), testLimits())
	requireNoErrors(t, providerErr)
	before, beforeErr := provider.MeetingInfo(ctx, request.Meeting)
	requireNoErrors(t, beforeErr)
	enterErr := provider.Enter(ctx, NewJoinLink(request.Address), request.Meeting, request.Participant)
	var failure Failure
	if enterErr != nil && !errors.As(enterErr, &failure) {
		t.Fatal("child provider operation failed")
	}
	after, afterErr := provider.MeetingInfo(ctx, request.Meeting)
	requireNoErrors(t, afterErr)
	if err := json.NewEncoder(os.Stdout).Encode(sharedProcessResponse{Before: before, After: after, Failure: failure}); err != nil {
		t.Fatal("child response write failed")
	}
}
