package meeting

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"ovc/backend/internal/core"
)

const (
	presentationAddress = `https://conferencing-storage.ovc.example/ovc-files/library/presentation-key?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=key%2F20261110%2Fru-central1%2Fs3%2Faws4_request&X-Amz-Signature=0123abcd&quote="<>'`
	presentationHost    = "conferencing-storage.ovc.example"
)

type receivedModules struct {
	XMLName xml.Name `xml:"modules"`
	Modules []struct {
		Name      string `xml:"name,attr"`
		Documents []struct {
			URL      string `xml:"url,attr"`
			Filename string `xml:"filename,attr"`
		} `xml:"document"`
	} `xml:"module"`
}

func samplePresentation() *Presentation {
	presentation := NewPresentation(core.NewSecret(presentationAddress))
	return &presentation
}

func TestBigBlueButtonCreateSendsPresentation(t *testing.T) {
	t.Run("the document travels only in the POST body", func(t *testing.T) {
		fake := newProtocolFake(t)
		provider := newProtocolProvider(t, fake)
		room := meetingInput()

		created, err := provider.CreateMeeting(t.Context(), room, samplePresentation())
		requests := fake.captured(bbbCallCreate)

		requireNoErrors(t, err)
		if created <= 0 || len(requests) != 1 || requests[0].method != http.MethodPost || requests[0].contentType != bbbBodyContentType {
			t.Fatalf("created = %d, requests = %d", created, len(requests))
		}
		if strings.Contains(requests[0].rawQuery, presentationHost) || strings.Contains(requests[0].rawQuery, "X-Amz") || len(requests[0].values) != 10 {
			t.Fatal("the presentation reached the checksummed query")
		}
		var modules receivedModules
		if err := xml.Unmarshal(requests[0].body, &modules); err != nil {
			t.Fatal(err)
		}
		if len(modules.Modules) != 1 || modules.Modules[0].Name != "presentation" || len(modules.Modules[0].Documents) != 1 {
			t.Fatalf("the body holds %d modules", len(modules.Modules))
		}
		document := modules.Modules[0].Documents[0]
		if document.URL != presentationAddress || document.Filename != "presentation.pdf" {
			t.Fatal("the document does not carry the exact address and the neutral file name")
		}
	})
	t.Run("a create without a presentation sends no body", func(t *testing.T) {
		fake := newProtocolFake(t)
		provider := newProtocolProvider(t, fake)

		_, err := provider.CreateMeeting(t.Context(), meetingInput(), nil)
		requests := fake.captured(bbbCallCreate)

		requireNoErrors(t, err)
		if len(requests) != 1 || requests[0].method != http.MethodGet || len(requests[0].body) != 0 || requests[0].contentType != "" {
			t.Fatal("a create without a presentation sent a body")
		}
	})
	t.Run("an unusable address reaches no server", func(t *testing.T) {
		fake := newProtocolFake(t)
		provider := newProtocolProvider(t, fake)
		addresses := map[string]string{
			"plain HTTP":       "http://conferencing-storage.ovc.example/key",
			"user information": "https://user@conferencing-storage.ovc.example/key",
			"no host":          "https:///key",
			"a fragment":       "https://conferencing-storage.ovc.example/key#part",
			"empty":            "",
		}
		for _, address := range addresses {
			presentation := NewPresentation(core.NewSecret(address))

			_, err := provider.CreateMeeting(t.Context(), meetingInput(), &presentation)

			requireFailure(t, err, FailureInvalidInput)
		}
		if fake.requestCount() != 0 {
			t.Fatal("an unusable presentation reached the provider")
		}
	})
}

func TestBigBlueButtonPresentationNeverInDiagnostics(t *testing.T) {
	var records bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&records, &slog.HandlerOptions{Level: slog.LevelDebug}))
	fake := newProtocolFake(t)
	bound := newExpiringBound()
	timing := quickTiming()
	timing.bound = bound.derive
	bounded, err := newBigBlueButton(protocolConfig(fake, logger), timing)
	requireNoErrors(t, err)
	provider, err := newBigBlueButton(protocolConfig(fake, logger), quickTiming())
	requireNoErrors(t, err)
	stall := newStalledResponse("")
	fake.setFault(bbbCallCreate, protocolFault{respond: stall.respond})
	timedOut := make(chan error, 1)

	go func() {
		_, err := bounded.CreateMeeting(t.Context(), meetingInput(), samplePresentation())
		timedOut <- err
	}()
	awaitSignal(t, stall.arrived, "the create request")
	expire := <-bound.cancel
	expire()
	awaitSignal(t, stall.released, "the server to see the connection close")
	timeoutErr := awaitResult(t, timedOut)
	fake.setFault(bbbCallCreate, protocolFault{respond: respondWith(http.StatusInternalServerError, "")})
	_, providerErr := provider.CreateMeeting(t.Context(), meetingInput(), samplePresentation())
	fake.setFault(bbbCallCreate, protocolFault{respond: respondWith(http.StatusOK, `<response><returncode>FAILED</returncode><messageKey>x</messageKey></response>`)})
	_, rejectedErr := provider.CreateMeeting(t.Context(), meetingInput(), samplePresentation())
	fake.server.Close()
	_, transportErr := provider.CreateMeeting(t.Context(), meetingInput(), samplePresentation())

	requireFailure(t, timeoutErr, FailureUnavailable)
	requireFailure(t, providerErr, FailureUnavailable)
	requireFailure(t, rejectedErr, FailureRejected)
	requireFailure(t, transportErr, FailureUnavailable)
	surfaces := []string{records.String(), fmt.Sprintf("%v %+v %#v", timeoutErr, providerErr, rejectedErr), fmt.Sprint(transportErr), fmt.Sprintf("%v %+v %#v", *samplePresentation(), *samplePresentation(), *samplePresentation())}
	for _, surface := range surfaces {
		for _, forbidden := range []string{presentationAddress, presentationHost, "X-Amz", "presentation-key"} {
			if strings.Contains(surface, forbidden) {
				t.Fatalf("the presentation address reached a diagnostic surface: %q", forbidden)
			}
		}
	}
}
