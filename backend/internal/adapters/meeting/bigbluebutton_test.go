package meeting

import (
	"bytes"
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
)

const (
	protocolPublicURL = "https://ovc.example"
	guardDeadline     = 30 * time.Second
)

func protocolConfig(fake *protocolFake, logger *slog.Logger) BigBlueButtonConfig {
	return BigBlueButtonConfig{
		Endpoint: fake.endpoint(), PublicURL: protocolPublicURL, Secret: core.NewSecret(protocolSecret),
		Algorithm: ChecksumSHA256, Roots: fake.roots(), Logger: logger,
	}
}

func quickTiming() bigBlueButtonTiming {
	timing := defaultBigBlueButtonTiming()
	timing.confirmInterval = time.Millisecond
	return timing
}

func newProtocolProvider(t *testing.T, fake *protocolFake) *BigBlueButton {
	t.Helper()
	provider, err := newBigBlueButton(protocolConfig(fake, testLogger()), quickTiming())
	requireNoErrors(t, err)
	return provider
}

func TestBigBlueButtonProviderContract(t *testing.T) {
	runProviderContract(t, func(t *testing.T) Provider { return newProtocolProvider(t, newProtocolFake(t)) })
}

func TestBigBlueButtonConstructionRefusals(t *testing.T) {
	fake := newProtocolFake(t)
	valid := protocolConfig(fake, testLogger())
	cases := map[string]func(*BigBlueButtonConfig){
		"plain HTTP endpoint":   func(config *BigBlueButtonConfig) { config.Endpoint = "http://bbb.example/bigbluebutton/api" },
		"endpoint credentials":  func(config *BigBlueButtonConfig) { config.Endpoint = "https://user@bbb.example/bigbluebutton/api" },
		"endpoint query":        func(config *BigBlueButtonConfig) { config.Endpoint = "https://bbb.example/api?x=1" },
		"endpoint without host": func(config *BigBlueButtonConfig) { config.Endpoint = "https:///api" },
		"plain HTTP public URL": func(config *BigBlueButtonConfig) { config.PublicURL = "http://ovc.example" },
		"endpoint fragment":     func(config *BigBlueButtonConfig) { config.Endpoint = "https://bbb.example/api#x" },
		"public credentials":    func(config *BigBlueButtonConfig) { config.PublicURL = "https://user@ovc.example" },
		"public query":          func(config *BigBlueButtonConfig) { config.PublicURL = "https://ovc.example?x=1" },
		"public fragment":       func(config *BigBlueButtonConfig) { config.PublicURL = "https://ovc.example#x" },
		"short secret": func(config *BigBlueButtonConfig) {
			config.Secret = core.NewSecret(strings.Repeat("x", bbbMinSecretBytes-1))
		},
		"absent secret":      func(config *BigBlueButtonConfig) { config.Secret = core.Secret{} },
		"SHA-1 algorithm":    func(config *BigBlueButtonConfig) { config.Algorithm = "sha1" },
		"absent trust roots": func(config *BigBlueButtonConfig) { config.Roots = nil },
		"absent logger":      func(config *BigBlueButtonConfig) { config.Logger = nil },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			config := valid
			change(&config)

			provider, err := NewBigBlueButton(config)

			requireFailure(t, err, FailureInvalidInput)
			if provider != nil {
				t.Fatal("a refused configuration returned a provider")
			}
		})
	}
	t.Run("a secret of exactly the minimum length is accepted", func(t *testing.T) {
		config := valid
		config.Secret = core.NewSecret(strings.Repeat("x", bbbMinSecretBytes))

		provider, err := NewBigBlueButton(config)

		requireNoErrors(t, err)
		if provider == nil {
			t.Fatal("a valid configuration returned no provider")
		}
	})
	t.Run("the bounds are the contracted ones", func(t *testing.T) {
		before := clock.System{}.Now()

		bounded, cancel := defaultBigBlueButtonTiming().bound(t.Context())
		defer cancel()
		deadline, limited := bounded.Deadline()
		after := clock.System{}.Now()

		if !limited || deadline.Before(before.Add(15*time.Second)) || deadline.After(after.Add(15*time.Second)) || bbbMaxResponseBytes != 1<<20 {
			t.Fatalf("deadline set = %v, maximum = %d", limited, bbbMaxResponseBytes)
		}
	})
}

func TestBigBlueButtonCreateRequest(t *testing.T) {
	for _, algorithm := range []ChecksumAlgorithm{ChecksumSHA256, ChecksumSHA384, ChecksumSHA512} {
		t.Run(string(algorithm), func(t *testing.T) {
			fake := newProtocolFake(t)
			config := protocolConfig(fake, testLogger())
			config.Algorithm = algorithm
			provider, err := NewBigBlueButton(config)
			requireNoErrors(t, err)
			room := meetingInput()

			created, createErr := provider.CreateMeeting(t.Context(), room)
			requests := fake.captured(bbbCallCreate)

			requireNoErrors(t, createErr)
			if created <= 0 || len(requests) != 1 || requests[0].method != http.MethodGet {
				t.Fatalf("created = %d, requests = %d", created, len(requests))
			}
			expected := map[string]string{
				"meetingID": room.ID.String(), "name": room.Name,
				"record": "false", "autoStartRecording": "false", "allowStartStopRecording": "false",
				"meetingExpireIfNoUserJoinedInMinutes": "10", "endWhenNoModerator": "true", "endWhenNoModeratorDelayInMinutes": "15",
				"logoutURL":       protocolPublicURL + "/lessons/" + room.LessonID.String(),
				"meetingEndedURL": protocolPublicURL + "/api/v1/integrations/bbb/" + room.RoutingKey.String() + "/ended",
			}
			for name, value := range expected {
				if requests[0].values.Get(name) != value {
					t.Fatalf("%s = %q, want %q", name, requests[0].values.Get(name), value)
				}
			}
			if len(requests[0].values) != len(expected) {
				t.Fatalf("create sent unexpected parameters: %d", len(requests[0].values))
			}
		})
	}
	t.Run("each meeting has its own callback route", func(t *testing.T) {
		fake := newProtocolFake(t)
		provider := newProtocolProvider(t, fake)
		first, second := meetingInput(), meetingInput()
		second.ID, second.RoutingKey = domain.ProviderMeetingID{8}, domain.MeetingRoutingKey{9}

		_, firstErr := provider.CreateMeeting(t.Context(), first)
		_, secondErr := provider.CreateMeeting(t.Context(), second)
		requests := fake.captured(bbbCallCreate)

		requireNoErrors(t, firstErr, secondErr)
		if requests[0].values.Get("meetingEndedURL") == requests[1].values.Get("meetingEndedURL") || !strings.Contains(requests[1].values.Get("meetingEndedURL"), second.RoutingKey.String()) {
			t.Fatal("the callback URL does not route to its own meeting")
		}
	})
	t.Run("invalid input reaches no provider", func(t *testing.T) {
		fake := newProtocolFake(t)
		provider := newProtocolProvider(t, fake)
		inputs := map[string]func(*domain.Meeting){
			"zero identifier":  func(room *domain.Meeting) { room.ID = domain.ProviderMeetingID{} },
			"short name":       func(room *domain.Meeting) { room.Name = "x" },
			"long name":        func(room *domain.Meeting) { room.Name = strings.Repeat("x", 65) },
			"control in name":  func(room *domain.Meeting) { room.Name = "a\x01b" },
			"zero routing key": func(room *domain.Meeting) { room.RoutingKey = domain.MeetingRoutingKey{} },
			"zero lesson":      func(room *domain.Meeting) { room.LessonID = domain.LessonID{} },
		}
		for name, change := range inputs {
			room := meetingInput()
			change(&room)

			_, err := provider.CreateMeeting(t.Context(), room)

			if !errors.Is(err, FailureInvalidInput) {
				t.Fatalf("%s: error = %v", name, err)
			}
		}
		if fake.requestCount() != 0 {
			t.Fatal("invalid input reached the provider")
		}
	})
}

func TestBigBlueButtonDuplicateCreate(t *testing.T) {
	fake := newProtocolFake(t)
	provider := newProtocolProvider(t, fake)
	room := createdMeeting(t, provider)
	differing := meetingInput()
	differing.Name = "Another room name"
	known, unknown := meetingInput(), meetingInput()
	known.CreateTime = room.CreateTime
	unknown.CreateTime = room.CreateTime + 1
	absent := meetingInput()
	absent.ID, absent.CreateTime = domain.ProviderMeetingID{7}, 5

	repeated, repeatedErr := provider.CreateMeeting(t.Context(), meetingInput())
	undetected, undetectedErr := provider.CreateMeeting(t.Context(), differing)
	creates := len(fake.captured(bbbCallCreate))
	boundRepeat, boundErr := provider.CreateMeeting(t.Context(), known)
	_, staleErr := provider.CreateMeeting(t.Context(), unknown)
	_, absentErr := provider.CreateMeeting(t.Context(), absent)
	fake.setFault(bbbCallCreate, protocolFault{respond: respondWith(http.StatusOK, protocolFailure(bbbKeyIDNotUnique))})
	_, refusedErr := provider.CreateMeeting(t.Context(), meetingInput())

	requireNoErrors(t, repeatedErr, undetectedErr, boundErr)
	requireFailure(t, staleErr, FailureConflict)
	requireFailure(t, absentErr, FailureInvalidInput)
	requireFailure(t, refusedErr, FailureConflict)
	if repeated != room.CreateTime || undetected != room.CreateTime || boundRepeat != room.CreateTime || creates != 3 {
		t.Fatalf("repeated = %d, undetected = %d, bound = %d, creates = %d", repeated, undetected, boundRepeat, creates)
	}
}

func respondWith(status int, body string) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(status)
		if _, err := io.WriteString(writer, body); err != nil {
			panic("test response write failed: " + err.Error())
		}
	}
}

func TestBigBlueButtonResponseFailures(t *testing.T) {
	room := meetingInput()
	id := room.ID.String()
	valid := func(meetingID string) string {
		return `<response><returncode>SUCCESS</returncode><meetingID>` + meetingID + `</meetingID><createTime>5</createTime><running>false</running><participantCount>0</participantCount></response>`
	}
	oversized := valid(id) + strings.Repeat(" ", bbbMaxResponseBytes+1-len(valid(id)))
	cases := map[string]struct {
		status   int
		body     string
		expected Failure
	}{
		"server error":         {http.StatusInternalServerError, valid(id), FailureUnavailable},
		"unauthorized":         {http.StatusUnauthorized, "", FailureUnavailable},
		"redirect status":      {http.StatusFound, "", FailureRedirected},
		"not XML":              {http.StatusOK, "plain text", FailureMalformed},
		"empty":                {http.StatusOK, "", FailureMalformed},
		"wrong root":           {http.StatusOK, `<other><returncode>SUCCESS</returncode></other>`, FailureMalformed},
		"unknown return code":  {http.StatusOK, `<response><returncode>MAYBE</returncode></response>`, FailureMalformed},
		"trailing document":    {http.StatusOK, valid(id) + `<response/>`, FailureMalformed},
		"trailing text":        {http.StatusOK, valid(id) + `extra`, FailureMalformed},
		"leading text":         {http.StatusOK, `junk` + valid(id), FailureMalformed},
		"leading comment":      {http.StatusOK, `<!-- note -->` + valid(id), FailureMalformed},
		"leading directive":    {http.StatusOK, `<!DOCTYPE response>` + valid(id), FailureMalformed},
		"foreign instruction":  {http.StatusOK, `<?other x?>` + valid(id), FailureMalformed},
		"repeated meeting":     {http.StatusOK, strings.Replace(valid(id), `<meetingID>`, `<meetingID>other</meetingID><meetingID>`, 1), FailureMalformed},
		"repeated create time": {http.StatusOK, strings.Replace(valid(id), `<createTime>`, `<createTime>9</createTime><createTime>`, 1), FailureMalformed},
		"repeated return code": {http.StatusOK, strings.Replace(valid(id), `<returncode>`, `<returncode>FAILED</returncode><returncode>`, 1), FailureMalformed},
		"undeclared entity":    {http.StatusOK, `<response><returncode>&a;</returncode></response>`, FailureMalformed},
		"trailing comment":     {http.StatusOK, valid(id) + `<!-- note -->`, FailureMalformed},
		"trailing instruction": {http.StatusOK, valid(id) + `<?other x?>`, FailureMalformed},
		"non-numeric time":     {http.StatusOK, `<response><returncode>SUCCESS</returncode><meetingID>` + id + `</meetingID><createTime>soon</createTime></response>`, FailureMalformed},
		"missing create time":  {http.StatusOK, `<response><returncode>SUCCESS</returncode><meetingID>` + id + `</meetingID></response>`, FailureMalformed},
		"zero create time":     {http.StatusOK, `<response><returncode>SUCCESS</returncode><meetingID>` + id + `</meetingID><createTime>0</createTime><running>false</running><participantCount>0</participantCount></response>`, FailureMalformed},
		"wrong meeting":        {http.StatusOK, valid("00000000-0000-4000-8000-000000000099"), FailureMismatch},
		"absent meeting echo":  {http.StatusOK, `<response><returncode>SUCCESS</returncode><createTime>5</createTime></response>`, FailureMismatch},
		"provider refusal":     {http.StatusOK, protocolFailure("checksumError"), FailureRejected},
		"oversized":            {http.StatusOK, oversized, FailureOversized},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fake := newProtocolFake(t)
			provider := newProtocolProvider(t, fake)
			fault := protocolFault{respond: respondWith(testCase.status, testCase.body)}
			fake.setFault(bbbCallCreate, fault)
			fake.setFault(bbbCallInfo, fault)

			created, createErr := provider.CreateMeeting(t.Context(), room)
			info, inspectErr := provider.InspectMeeting(t.Context(), room.ID)

			requireFailure(t, createErr, testCase.expected)
			requireFailure(t, inspectErr, testCase.expected)
			if created != 0 || info != (domain.MeetingInfo{}) {
				t.Fatalf("a failed response returned created = %d, info = %+v", created, info)
			}
		})
	}
	t.Run("nested and unrelated elements do not count as the used fields", func(t *testing.T) {
		fake := newProtocolFake(t)
		provider := newProtocolProvider(t, fake)
		nested := `<attendees><attendee><userID>u</userID></attendee></attendees><metadata><meetingID>other</meetingID><createTime>9</createTime></metadata>`
		fake.setFault(bbbCallInfo, protocolFault{respond: respondWith(http.StatusOK, strings.Replace(valid(id), `</response>`, nested+`</response>`, 1))})

		info, err := provider.InspectMeeting(t.Context(), room.ID)

		requireNoErrors(t, err)
		if info.CreateTime != 5 {
			t.Fatalf("info = %+v", info)
		}
	})
	t.Run("an XML declaration and surrounding whitespace are accepted", func(t *testing.T) {
		fake := newProtocolFake(t)
		provider := newProtocolProvider(t, fake)
		fake.setFault(bbbCallInfo, protocolFault{respond: respondWith(http.StatusOK, "<?xml version=\"1.0\"?>\n"+valid(id)+"\n")})

		info, err := provider.InspectMeeting(t.Context(), room.ID)

		requireNoErrors(t, err)
		if info.CreateTime != 5 {
			t.Fatalf("info = %+v", info)
		}
	})
	t.Run("the largest allowed response is accepted", func(t *testing.T) {
		fake := newProtocolFake(t)
		provider := newProtocolProvider(t, fake)
		body := valid(id) + strings.Repeat(" ", bbbMaxResponseBytes-len(valid(id)))
		if !protocolWellFormed(body) {
			t.Fatal("the fixture is not a well-formed document")
		}
		fake.setFault(bbbCallInfo, protocolFault{respond: respondWith(http.StatusOK, body)})

		info, err := provider.InspectMeeting(t.Context(), room.ID)

		requireNoErrors(t, err)
		if info.CreateTime != 5 {
			t.Fatalf("info = %+v", info)
		}
	})
	t.Run("a create response for an ended generation is refused", func(t *testing.T) {
		fake := newProtocolFake(t)
		provider := newProtocolProvider(t, fake)
		fake.setFault(bbbCallCreate, protocolFault{respond: respondWith(http.StatusOK, `<response><returncode>SUCCESS</returncode><meetingID>`+id+`</meetingID><createTime>5</createTime><hasBeenForciblyEnded>true</hasBeenForciblyEnded></response>`)})

		_, err := provider.CreateMeeting(t.Context(), room)

		requireFailure(t, err, FailureConflict)
	})
}

func TestBigBlueButtonRefusesRedirects(t *testing.T) {
	fake := newProtocolFake(t)
	elsewhere := newProtocolFake(t)
	provider := newProtocolProvider(t, fake)
	target := elsewhere.endpoint() + "/" + bbbCallInfo
	fake.setFault(bbbCallInfo, protocolFault{respond: func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, target, http.StatusTemporaryRedirect)
	}})

	_, err := provider.InspectMeeting(t.Context(), meetingInput().ID)

	requireFailure(t, err, FailureRedirected)
	if elsewhere.requestCount() != 0 {
		t.Fatal("the adapter followed a redirect")
	}
}

func TestBigBlueButtonTrustsOnlyInjectedRoots(t *testing.T) {
	fake := newProtocolFake(t)
	config := protocolConfig(fake, testLogger())
	config.Roots = x509.NewCertPool()
	provider, err := NewBigBlueButton(config)
	requireNoErrors(t, err)

	_, callErr := provider.InspectMeeting(t.Context(), meetingInput().ID)

	requireFailure(t, callErr, FailureUnavailable)
	if fake.requestCount() != 0 {
		t.Fatal("a request crossed an untrusted connection")
	}
}

func TestBigBlueButtonCreateIsNotReplayed(t *testing.T) {
	fake := newProtocolFake(t)
	provider := newProtocolProvider(t, fake)
	room := meetingInput()
	_, err := provider.InspectMeeting(t.Context(), room.ID)
	requireNoErrors(t, err)
	fake.setFault(bbbCallCreate, protocolFault{applyState: true, respond: func(writer http.ResponseWriter, _ *http.Request) {
		connection, _, hijackErr := http.NewResponseController(writer).Hijack()
		if hijackErr != nil {
			panic("test hijack failed: " + hijackErr.Error())
		}
		if closeErr := connection.Close(); closeErr != nil {
			panic("test connection close failed: " + closeErr.Error())
		}
	}})

	_, createErr := provider.CreateMeeting(t.Context(), room)
	info, inspectErr := provider.InspectMeeting(t.Context(), room.ID)

	requireFailure(t, createErr, FailureUnavailable)
	requireNoErrors(t, inspectErr)
	if len(fake.captured(bbbCallCreate)) != 1 || info.Status != domain.MeetingCreated {
		t.Fatalf("creates = %d, info = %+v", len(fake.captured(bbbCallCreate)), info)
	}
}

type stalledResponse struct {
	arrived  chan struct{}
	released chan struct{}
	partial  string
}

func newStalledResponse(partial string) *stalledResponse {
	return &stalledResponse{arrived: make(chan struct{}, 8), released: make(chan struct{}, 8), partial: partial}
}

func (stall *stalledResponse) respond(writer http.ResponseWriter, request *http.Request) {
	if stall.partial != "" {
		if _, err := io.WriteString(writer, stall.partial); err != nil {
			panic("test partial write failed: " + err.Error())
		}
		if err := http.NewResponseController(writer).Flush(); err != nil {
			panic("test flush failed: " + err.Error())
		}
	}
	stall.arrived <- struct{}{}
	<-request.Context().Done()
	stall.released <- struct{}{}
}

func awaitSignal(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	timer := time.NewTimer(guardDeadline)
	defer timer.Stop()
	select {
	case <-signal:
	case <-timer.C:
		t.Fatalf("timed out waiting for %s", description)
	}
}

type expiringBound struct {
	cancel chan context.CancelFunc
}

func newExpiringBound() *expiringBound {
	return &expiringBound{cancel: make(chan context.CancelFunc, 1)}
}

func (bound *expiringBound) derive(ctx context.Context) (context.Context, context.CancelFunc) {
	derived, cancel := context.WithCancel(ctx)
	bound.cancel <- cancel
	return derived, cancel
}

func stalledOperations() map[string]struct {
	call    string
	prepare bool
	run     func(context.Context, *BigBlueButton, domain.Meeting) error
} {
	return map[string]struct {
		call    string
		prepare bool
		run     func(context.Context, *BigBlueButton, domain.Meeting) error
	}{
		"create": {call: bbbCallCreate, run: func(ctx context.Context, provider *BigBlueButton, room domain.Meeting) error {
			_, err := provider.CreateMeeting(ctx, room)
			return err
		}},
		"inspect": {call: bbbCallInfo, run: func(ctx context.Context, provider *BigBlueButton, room domain.Meeting) error {
			_, err := provider.InspectMeeting(ctx, room.ID)
			return err
		}},
		"end": {call: bbbCallEnd, prepare: true, run: func(ctx context.Context, provider *BigBlueButton, room domain.Meeting) error {
			return provider.EndMeeting(ctx, room)
		}},
	}
}

func awaitResult(t *testing.T, result <-chan error) error {
	t.Helper()
	timer := time.NewTimer(guardDeadline)
	defer timer.Stop()
	select {
	case err := <-result:
		return err
	case <-timer.C:
		t.Fatal("timed out waiting for the operation to return")
		return nil
	}
}

func TestBigBlueButtonDeadlinesAndCancellation(t *testing.T) {
	partialBody := `<response><returncode>SUCCESS</returncode>`
	for operation, stalled := range stalledOperations() {
		for stage, partial := range map[string]string{"before headers": "", "inside the body": partialBody} {
			arrange := func(t *testing.T, timing bigBlueButtonTiming) (*BigBlueButton, *stalledResponse, domain.Meeting) {
				t.Helper()
				fake := newProtocolFake(t)
				provider, err := newBigBlueButton(protocolConfig(fake, testLogger()), timing)
				requireNoErrors(t, err)
				room := meetingInput()
				if stalled.prepare {
					room = createdMeetingWith(t, fake)
				}
				stall := newStalledResponse(partial)
				fake.setFault(stalled.call, protocolFault{respond: stall.respond})
				return provider, stall, room
			}
			t.Run("the caller's cancellation releases "+operation+" "+stage, func(t *testing.T) {
				provider, stall, room := arrange(t, quickTiming())
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				result := make(chan error, 1)

				go func() { result <- stalled.run(ctx, provider, room) }()
				awaitSignal(t, stall.arrived, "the request")
				cancel()
				awaitSignal(t, stall.released, "the server to see the connection close")
				err := awaitResult(t, result)

				requireFailure(t, err, context.Canceled)
			})
			t.Run("the operation bound releases "+operation+" "+stage, func(t *testing.T) {
				bound := newExpiringBound()
				timing := quickTiming()
				timing.bound = bound.derive
				provider, stall, room := arrange(t, timing)
				result := make(chan error, 1)

				go func() { result <- stalled.run(t.Context(), provider, room) }()
				awaitSignal(t, stall.arrived, "the request")
				expire := <-bound.cancel
				expire()
				awaitSignal(t, stall.released, "the server to see the connection close")
				err := awaitResult(t, result)

				requireFailure(t, err, FailureUnavailable)
			})
		}
	}
	t.Run("the caller's earlier deadline reports itself", func(t *testing.T) {
		fake := newProtocolFake(t)
		provider := newProtocolProvider(t, fake)
		stall := newStalledResponse("")
		fake.setFault(bbbCallInfo, protocolFault{respond: stall.respond})
		ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
		defer cancel()

		_, err := provider.InspectMeeting(ctx, meetingInput().ID)

		requireFailure(t, err, context.DeadlineExceeded)
	})
	t.Run("the end of the bound interrupts the wait between End confirmations", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		err := waitBigBlueButton(ctx, time.Hour)

		requireFailure(t, err, FailureUnavailable)
	})
	t.Run("the caller's cancellation during End confirmation reports itself", func(t *testing.T) {
		fake := newProtocolFake(t)
		timing := quickTiming()
		timing.confirmInterval = time.Hour
		provider, err := newBigBlueButton(protocolConfig(fake, testLogger()), timing)
		requireNoErrors(t, err)
		room := createdMeeting(t, provider)
		fake.setEndReads(100)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		result := make(chan error, 1)

		go func() { result <- provider.EndMeeting(ctx, room) }()
		awaitSignal(t, fake.infoReads, "the generation read")
		awaitSignal(t, fake.infoReads, "the first confirmation read")
		cancel()
		endErr := awaitResult(t, result)

		requireFailure(t, endErr, context.Canceled)
	})
	t.Run("a cancelled context sends nothing", func(t *testing.T) {
		fake := newProtocolFake(t)
		provider := newProtocolProvider(t, fake)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		_, createErr := provider.CreateMeeting(ctx, meetingInput())
		_, inspectErr := provider.InspectMeeting(ctx, meetingInput().ID)
		_, infoErr := provider.MeetingInfo(ctx, meetingInput())
		_, linkErr := provider.JoinLink(ctx, meetingInput(), testParticipant())
		endErr := provider.EndMeeting(ctx, meetingInput())

		for _, err := range []error{createErr, inspectErr, infoErr, linkErr, endErr} {
			requireFailure(t, err, context.Canceled)
		}
		if fake.requestCount() != 0 {
			t.Fatal("a cancelled operation reached the provider")
		}
	})
}

func createdMeetingWith(t *testing.T, fake *protocolFake) domain.Meeting {
	t.Helper()
	return createdMeeting(t, newProtocolProvider(t, fake))
}

func TestBigBlueButtonGenerationBinding(t *testing.T) {
	fake := newProtocolFake(t)
	provider := newProtocolProvider(t, fake)
	old := createdMeeting(t, provider)
	requireNoErrors(t, provider.EndMeeting(t.Context(), old))
	replacement := createdMeeting(t, provider)
	if replacement.CreateTime == old.CreateTime {
		t.Fatal("the replacement kept the old generation")
	}
	ends := len(fake.captured(bbbCallEnd))

	staleInfo, staleInfoErr := provider.MeetingInfo(t.Context(), old)
	_, staleLinkErr := provider.JoinLink(t.Context(), old, testParticipant())
	staleEndErr := provider.EndMeeting(t.Context(), old)
	current, currentErr := provider.InspectMeeting(t.Context(), old.ID)

	requireNoErrors(t, staleInfoErr, currentErr)
	requireFailure(t, staleLinkErr, FailureNotFound)
	requireFailure(t, staleEndErr, FailureNotFound)
	if staleInfo != (domain.MeetingInfo{Status: domain.MeetingNotFound}) || current.CreateTime != replacement.CreateTime || current.Status != domain.MeetingCreated {
		t.Fatalf("stale info = %+v, current = %+v", staleInfo, current)
	}
	if len(fake.captured(bbbCallEnd)) != ends {
		t.Fatal("a stale End reached the provider and could end the replacement")
	}
}

func TestBigBlueButtonEndConfirmation(t *testing.T) {
	t.Run("an asynchronous end is observed before success", func(t *testing.T) {
		fake := newProtocolFake(t)
		provider := newProtocolProvider(t, fake)
		room := createdMeeting(t, provider)
		fake.setEndReads(2)

		err := provider.EndMeeting(t.Context(), room)
		info, infoErr := provider.InspectMeeting(t.Context(), room.ID)

		requireNoErrors(t, err, infoErr)
		if info.Status != domain.MeetingNotFound {
			t.Fatalf("info = %+v", info)
		}
	})
	t.Run("an unobserved end is not reported as finished and can be retried", func(t *testing.T) {
		fake := newProtocolFake(t)
		provider := newProtocolProvider(t, fake)
		room := createdMeeting(t, provider)
		fake.setEndReads(bbbEndConfirmReads)

		firstErr := provider.EndMeeting(t.Context(), room)
		secondErr := provider.EndMeeting(t.Context(), room)

		requireFailure(t, firstErr, FailureUnavailable)
		requireNoErrors(t, secondErr)
	})
	t.Run("an ended room still reported by the provider counts as absent", func(t *testing.T) {
		fake := newProtocolFake(t)
		provider := newProtocolProvider(t, fake)
		fake.setReportEnded(true)
		room := createdMeeting(t, provider)

		err := provider.EndMeeting(t.Context(), room)
		info, infoErr := provider.MeetingInfo(t.Context(), room)
		discovered, discoverErr := provider.InspectMeeting(t.Context(), room.ID)
		_, linkErr := provider.JoinLink(t.Context(), room, testParticipant())

		requireNoErrors(t, err, infoErr, discoverErr)
		requireFailure(t, linkErr, FailureNotFound)
		if info.Status != domain.MeetingNotFound || discovered != (domain.MeetingInfo{Status: domain.MeetingNotFound}) {
			t.Fatalf("info = %+v, discovered = %+v", info, discovered)
		}
	})
	t.Run("a replacement appearing during confirmation is a conflict", func(t *testing.T) {
		fake := newProtocolFake(t)
		provider := newProtocolProvider(t, fake)
		room := createdMeeting(t, provider)
		fake.setFault(bbbCallEnd, protocolFault{respond: func(writer http.ResponseWriter, _ *http.Request) {
			fake.mutex.Lock()
			fake.rooms[room.ID.String()].createTime++
			fake.mutex.Unlock()
			writeProtocolAnswer(writer, protocolSuccess(bbbKeyEndRequestSent))
		}})

		err := provider.EndMeeting(t.Context(), room)

		requireFailure(t, err, FailureConflict)
	})
	t.Run("a provider refusal is not success", func(t *testing.T) {
		fake := newProtocolFake(t)
		provider := newProtocolProvider(t, fake)
		room := createdMeeting(t, provider)
		fake.setFault(bbbCallEnd, protocolFault{respond: respondWith(http.StatusOK, protocolFailure("checksumError"))})

		err := provider.EndMeeting(t.Context(), room)

		requireFailure(t, err, FailureRejected)
	})
	t.Run("an unexpected acknowledgement is not success", func(t *testing.T) {
		fake := newProtocolFake(t)
		provider := newProtocolProvider(t, fake)
		room := createdMeeting(t, provider)
		fake.setFault(bbbCallEnd, protocolFault{respond: respondWith(http.StatusOK, `<response><returncode>SUCCESS</returncode></response>`)})

		err := provider.EndMeeting(t.Context(), room)

		requireFailure(t, err, FailureMalformed)
	})
	t.Run("an end refused as already finished is confirmed by reading", func(t *testing.T) {
		fake := newProtocolFake(t)
		provider := newProtocolProvider(t, fake)
		room := createdMeeting(t, provider)
		fake.setFault(bbbCallEnd, protocolFault{applyState: true, respond: respondWith(http.StatusOK, protocolFailure(bbbKeyForciblyEnded))})

		err := provider.EndMeeting(t.Context(), room)

		requireNoErrors(t, err)
	})
}

func redeem(t *testing.T, fake *protocolFake, address string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), guardDeadline)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, http.NoBody)
	requireNoErrors(t, err)
	answer, err := fake.browser().Do(request)
	requireNoErrors(t, err)
	defer func() { requireNoErrors(t, answer.Body.Close()) }()
	body, err := io.ReadAll(answer.Body)
	requireNoErrors(t, err)
	return string(body)
}

func parsedLink(t *testing.T, link JoinLink) *url.URL {
	t.Helper()
	parsed, err := url.Parse(link.Reveal())
	requireNoErrors(t, err)
	return parsed
}

func TestBigBlueButtonJoinLinks(t *testing.T) {
	t.Run("the link is bound to the configured origin, generation and participant", func(t *testing.T) {
		fake := newProtocolFake(t)
		provider := newProtocolProvider(t, fake)
		room := createdMeeting(t, provider)
		participant := testParticipant()

		link := issuedLink(t, provider, room, participant)
		parsed := parsedLink(t, link)
		query := parsed.Query()
		requestsBefore := fake.captured(bbbCallJoin)

		if !strings.HasPrefix(link.Reveal(), fake.endpoint()+"/join?") || len(requestsBefore) != 0 {
			t.Fatal("the link is not on the configured endpoint, or issuing it joined")
		}
		if query.Get("meetingID") != room.ID.String() || query.Get("createTime") != strconv.FormatInt(int64(room.CreateTime), 10) || query.Get("role") != "MODERATOR" || query.Get("userID") != participant.ID.String() || query.Get("fullName") != participant.Name {
			t.Fatal("the link parameters do not bind the generation and participant")
		}
		if len(query) != 6 || strings.Contains(link.Reveal(), protocolSecret) {
			t.Fatal("the link carries unexpected parameters or the shared secret")
		}
	})
	t.Run("a viewer enters with the viewer role and the room runs", func(t *testing.T) {
		fake := newProtocolFake(t)
		provider := newProtocolProvider(t, fake)
		room := createdMeeting(t, provider)
		participant := testParticipant()
		participant.Role = domain.ParticipantViewer
		link := issuedLink(t, provider, room, participant)

		answer := redeem(t, fake, link.Reveal())
		info, err := provider.MeetingInfo(t.Context(), room)

		requireNoErrors(t, err)
		if !strings.Contains(answer, "successfullyJoined") || fake.participants(room.ID.String())[participant.ID.String()] != "VIEWER" || info.Status != domain.MeetingRunning || info.Participants != 1 {
			t.Fatalf("answer = %q, info = %+v", answer, info)
		}
	})
	t.Run("a changed parameter fails the signature", func(t *testing.T) {
		fake := newProtocolFake(t)
		provider := newProtocolProvider(t, fake)
		room := createdMeeting(t, provider)
		participant := testParticipant()
		participant.Role = domain.ParticipantViewer
		address := issuedLink(t, provider, room, participant).Reveal()

		answer := redeem(t, fake, strings.Replace(address, "role=VIEWER", "role=MODERATOR", 1))

		if !strings.Contains(answer, "checksumError") || len(fake.participants(room.ID.String())) != 0 {
			t.Fatalf("a tampered link was accepted: %q", answer)
		}
	})
	t.Run("an earlier link stops working after End and for a replacement", func(t *testing.T) {
		fake := newProtocolFake(t)
		provider := newProtocolProvider(t, fake)
		room := createdMeeting(t, provider)
		address := issuedLink(t, provider, room, testParticipant()).Reveal()
		requireNoErrors(t, provider.EndMeeting(t.Context(), room))

		ended := redeem(t, fake, address)
		replacement := createdMeeting(t, provider)
		stale := redeem(t, fake, address)

		if !strings.Contains(ended, bbbKeyForciblyEnded) || !strings.Contains(stale, "mismatchCreateTimeParam") || len(fake.participants(replacement.ID.String())) != 0 {
			t.Fatalf("ended = %q, stale = %q", ended, stale)
		}
	})
	t.Run("names encode exactly as the provider rebuilds them", func(t *testing.T) {
		fake := newProtocolFake(t)
		provider := newProtocolProvider(t, fake)
		room := createdMeeting(t, provider)
		participant := testParticipant()
		participant.Name = `Zoë a*b~c %2A &= + "<x>" 李`

		link := issuedLink(t, provider, room, participant)
		answer := redeem(t, fake, link.Reveal())

		if !strings.Contains(answer, "successfullyJoined") || parsedLink(t, link).Query().Get("fullName") != participant.Name {
			t.Fatalf("answer = %q", answer)
		}
	})
	t.Run("invalid participants receive no link", func(t *testing.T) {
		fake := newProtocolFake(t)
		provider := newProtocolProvider(t, fake)
		room := createdMeeting(t, provider)
		inputs := []domain.Participant{
			{Name: "Name", Role: domain.ParticipantViewer},
			{ID: domain.ParticipantID{2}, Name: " ", Role: domain.ParticipantViewer},
			{ID: domain.ParticipantID{2}, Name: strings.Repeat("x", domain.ParticipantNameMaxRunes+1), Role: domain.ParticipantViewer},
			{ID: domain.ParticipantID{2}, Name: "Name", Role: "owner"},
		}
		for _, participant := range inputs {
			link, err := provider.JoinLink(t.Context(), room, participant)

			requireFailure(t, err, FailureInvalidInput)
			if !link.IsZero() {
				t.Fatal("a refused participant received a link")
			}
		}
	})
	t.Run("an unavailable provider issues no link", func(t *testing.T) {
		fake := newProtocolFake(t)
		provider := newProtocolProvider(t, fake)
		room := createdMeeting(t, provider)
		fake.setFault(bbbCallInfo, protocolFault{respond: respondWith(http.StatusBadGateway, "")})

		link, err := provider.JoinLink(t.Context(), room, testParticipant())
		fake.clearFaults()

		requireFailure(t, err, FailureUnavailable)
		if !link.IsZero() {
			t.Fatal("a failed inspection released a link")
		}
	})
}

func TestBigBlueButtonDiagnostics(t *testing.T) {
	fake := newProtocolFake(t)
	var records bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&records, &slog.HandlerOptions{Level: slog.LevelDebug}))
	config := protocolConfig(fake, logger)
	provider, err := newBigBlueButton(config, quickTiming())
	requireNoErrors(t, err)
	room := createdMeeting(t, provider)
	participant := testParticipant()
	link := issuedLink(t, provider, room, participant)
	providerDetail := "private provider diagnostic body"
	fake.setFault(bbbCallInfo, protocolFault{respond: respondWith(http.StatusOK, `<response><returncode>FAILED</returncode><messageKey>`+providerDetail+`</messageKey></response>`)})
	_, rejectedErr := provider.InspectMeeting(t.Context(), room.ID)
	fake.setFault(bbbCallInfo, protocolFault{respond: respondWith(http.StatusOK, providerDetail)})
	_, malformedErr := provider.InspectMeeting(t.Context(), room.ID)
	fake.server.CloseClientConnections()
	closed := fake.endpoint()
	fake.server.Close()
	_, transportErr := provider.InspectMeeting(t.Context(), room.ID)

	output := records.String()
	surfaces := []string{output, fmt.Sprint(rejectedErr), fmt.Sprint(malformedErr), fmt.Sprint(transportErr), fmt.Sprintf("%+v", link)}
	configuration := fmt.Sprintf("%+v %#v", config, config.Secret)

	requireFailure(t, rejectedErr, FailureRejected)
	requireFailure(t, malformedErr, FailureMalformed)
	requireFailure(t, transportErr, FailureUnavailable)
	for _, marker := range []string{`"msg":"meeting.provider.completed","operation":"create","outcome":"ok"`, `"msg":"meeting.provider.completed","operation":"join_link","outcome":"ok"`, `"msg":"meeting.provider.failed","operation":"inspect","kind":"rejected"`, `"msg":"meeting.provider.failed","operation":"inspect","kind":"malformed_response"`, `"msg":"meeting.provider.failed","operation":"inspect","kind":"unavailable"`} {
		if !strings.Contains(output, marker) {
			t.Fatalf("a diagnostic with fixed fields is missing: %s", marker)
		}
	}
	if strings.Contains(configuration, protocolSecret) {
		t.Fatal("formatting the configuration exposed the shared secret")
	}
	checksum := parsedLink(t, link).Query().Get("checksum")
	for _, surface := range surfaces {
		for _, forbidden := range []string{protocolSecret, closed, link.Reveal(), checksum, "checksum=", providerDetail, room.Name, participant.Name, room.ID.String(), participant.ID.String(), meetingInput().RoutingKey.String(), meetingInput().LessonID.String()} {
			if strings.Contains(surface, forbidden) {
				t.Fatalf("an unsafe value reached a diagnostic surface: %q", forbidden)
			}
		}
	}
}

func TestBigBlueButtonProcessSelectionDisabled(t *testing.T) {
	root, err := os.OpenRoot(filepath.Join("..", "..", ".."))
	requireNoErrors(t, err)
	defer func() { requireNoErrors(t, root.Close()) }()
	tree := root.FS()
	var references []string

	walkErr := fs.WalkDir(tree, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || path == "internal/adapters/meeting/bigbluebutton.go" {
			return err
		}
		source, readErr := fs.ReadFile(tree, path)
		if readErr != nil {
			return readErr
		}
		if bytes.Contains(source, []byte("NewBigBlueButton")) || bytes.Contains(source, []byte("newBigBlueButton")) {
			references = append(references, path)
		}
		return nil
	})

	requireNoErrors(t, walkErr)
	if len(references) != 0 {
		t.Fatalf("production code selects the BigBlueButton adapter before remote evidence exists: %v", references)
	}
}
