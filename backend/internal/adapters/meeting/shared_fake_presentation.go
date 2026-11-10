package meeting

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"ovc/backend/internal/domain"
)

const (
	fakePresentationDeadline = 15 * time.Second
	fakePresentationMaxBytes = domain.LibraryMaxBytes
)

type ReceivedPresentation struct {
	Size   int64
	SHA256 [sha256.Size]byte
}

type presentationReceiver struct {
	origin string
	client *http.Client
}

func newPresentationReceiver(origin string) (presentationReceiver, error) {
	if origin == "" {
		return presentationReceiver{}, nil
	}
	canonical, local := localOrigin(origin)
	if !local {
		return presentationReceiver{}, FailureInvalidInput
	}
	return presentationReceiver{origin: canonical, client: &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport:     &http.Transport{Proxy: nil, DisableKeepAlives: true, DisableCompression: true, MaxResponseHeaderBytes: bbbMaxResponseHeaderBytes},
	}}, nil
}

func localOrigin(raw string) (string, bool) {
	parsed, err := url.Parse(strings.TrimRight(raw, "/"))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || parsed.Opaque != "" {
		return "", false
	}
	host := parsed.Hostname()
	address, addressErr := netip.ParseAddr(host)
	loopback := strings.EqualFold(host, "localhost") || (addressErr == nil && address.Zone() == "" && address.IsLoopback())
	return parsed.Scheme + "://" + parsed.Host, loopback
}

func (r presentationReceiver) address(presentation Presentation) (*url.URL, bool) {
	parsed, err := url.Parse(presentation.Reveal())
	if err != nil || r.origin == "" || parsed.User != nil || parsed.Opaque != "" || parsed.Fragment != "" {
		return nil, false
	}
	return parsed, parsed.Scheme+"://"+parsed.Host == r.origin
}

func (f *SharedFake) receivePresentation(ctx context.Context, requested domain.Meeting, presentation *Presentation) (*ReceivedPresentation, error) {
	if presentation == nil {
		return nil, nil
	}
	address, allowed := f.presentations.address(*presentation)
	if !allowed {
		return nil, FailureInvalidInput
	}
	existing, err := f.roomExists(ctx, requested.ID)
	if err != nil || existing || requested.CreateTime != 0 {
		return nil, err
	}
	received, err := f.presentations.fetch(ctx, address)
	if cause := ctx.Err(); cause != nil {
		return nil, cause
	}
	var failure Failure
	if errors.As(err, &failure) {
		f.logger.LogAttrs(ctx, slog.LevelWarn, "meeting.provider.presentation_unfetched", slog.String("kind", string(failure)))
		return nil, nil
	}
	if err != nil {
		return nil, FailureUnavailable
	}
	return received, nil
}

func (f *SharedFake) roomExists(ctx context.Context, id domain.ProviderMeetingID) (bool, error) {
	var existing bool
	if err := f.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM fake_meeting_rooms WHERE id = $1)`, [16]byte(id)).Scan(&existing); err != nil {
		return false, sharedFailure(ctx, err)
	}
	return existing, nil
}

func (r presentationReceiver) fetch(ctx context.Context, address *url.URL) (*ReceivedPresentation, error) {
	bounded, cancel := context.WithTimeout(ctx, fakePresentationDeadline)
	defer cancel()
	request, err := http.NewRequestWithContext(bounded, http.MethodGet, address.String(), http.NoBody)
	if err != nil {
		return nil, FailureInvalidInput
	}
	response, err := r.client.Do(request)
	if err != nil {
		return nil, FailureUnavailable
	}
	if response.StatusCode != http.StatusOK {
		if err := response.Body.Close(); err != nil {
			return nil, FailureUnavailable
		}
		return nil, FailureRejected
	}
	digest := sha256.New()
	size, copyErr := io.Copy(digest, io.LimitReader(response.Body, fakePresentationMaxBytes+1))
	closeErr := response.Body.Close()
	if copyErr != nil || closeErr != nil {
		return nil, FailureUnavailable
	}
	if size > fakePresentationMaxBytes {
		return nil, FailureOversized
	}
	received := &ReceivedPresentation{Size: size}
	copy(received.SHA256[:], digest.Sum(nil))
	return received, nil
}

func (f *SharedFake) ReceivedPresentation(ctx context.Context, id domain.ProviderMeetingID) (ReceivedPresentation, bool, error) {
	var size *int64
	var digest []byte
	err := f.pool.QueryRow(ctx, `SELECT presentation_size, presentation_sha256 FROM fake_meeting_rooms WHERE id = $1`, [16]byte(id)).Scan(&size, &digest)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReceivedPresentation{}, false, FailureNotFound
	}
	if err != nil {
		return ReceivedPresentation{}, false, sharedFailure(ctx, err)
	}
	if size == nil {
		return ReceivedPresentation{}, false, nil
	}
	if len(digest) != sha256.Size {
		return ReceivedPresentation{}, false, FailureDatabase
	}
	received := ReceivedPresentation{Size: *size}
	copy(received.SHA256[:], digest)
	return received, true, nil
}

func presentationColumns(received *ReceivedPresentation) (*int64, []byte) {
	if received == nil {
		return nil, nil
	}
	return &received.Size, received.SHA256[:]
}
