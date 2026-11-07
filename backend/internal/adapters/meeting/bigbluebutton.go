package meeting

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
)

type ChecksumAlgorithm string

const (
	ChecksumSHA256 ChecksumAlgorithm = "sha256"
	ChecksumSHA384 ChecksumAlgorithm = "sha384"
	ChecksumSHA512 ChecksumAlgorithm = "sha512"
)

const (
	bbbMaxResponseBytes         = 1 << 20
	bbbMaxResponseHeaderBytes   = 64 << 10
	bbbOperationDeadline        = 15 * time.Second
	bbbEndConfirmInterval       = time.Second
	bbbEndConfirmReads          = 5
	bbbMinSecretBytes           = 32
	bbbRoomNameMinRunes         = 2
	bbbRoomNameMaxRunes         = 64
	bbbExpireWithoutJoinMinutes = "10"
	bbbEndWithoutModeratorDelay = "15"
)

const (
	bbbCallCreate = "create"
	bbbCallInfo   = "getMeetingInfo"
	bbbCallEnd    = "end"
	bbbCallJoin   = "join"
)

const (
	bbbReturnSuccess     = "SUCCESS"
	bbbReturnFailed      = "FAILED"
	bbbKeyNotFound       = "notFound"
	bbbKeyForciblyEnded  = "meetingForciblyEnded"
	bbbKeyIDNotUnique    = "idNotUnique"
	bbbKeyEndRequestSent = "sentEndMeetingRequest"
)

type BigBlueButtonConfig struct {
	Endpoint  string
	PublicURL string
	Secret    core.Secret
	Algorithm ChecksumAlgorithm
	Roots     *x509.CertPool
	Logger    *slog.Logger
}

type bigBlueButtonTiming struct {
	bound           func(context.Context) (context.Context, context.CancelFunc)
	confirmInterval time.Duration
	confirmReads    int
}

func defaultBigBlueButtonTiming() bigBlueButtonTiming {
	return bigBlueButtonTiming{
		bound: func(ctx context.Context) (context.Context, context.CancelFunc) {
			return context.WithTimeout(ctx, bbbOperationDeadline)
		},
		confirmInterval: bbbEndConfirmInterval, confirmReads: bbbEndConfirmReads,
	}
}

type BigBlueButton struct {
	endpoint  string
	public    string
	secret    core.Secret
	algorithm ChecksumAlgorithm
	client    *http.Client
	logger    *slog.Logger
	timing    bigBlueButtonTiming
}

func NewBigBlueButton(config BigBlueButtonConfig) (*BigBlueButton, error) {
	return newBigBlueButton(config, defaultBigBlueButtonTiming())
}

func newBigBlueButton(config BigBlueButtonConfig, timing bigBlueButtonTiming) (*BigBlueButton, error) {
	endpoint, endpointValid := canonicalHTTPSBase(config.Endpoint)
	public, publicValid := canonicalHTTPSBase(config.PublicURL)
	if !endpointValid || !publicValid || len(config.Secret.Reveal()) < bbbMinSecretBytes || !config.Algorithm.valid() || config.Roots == nil || config.Logger == nil {
		return nil, FailureInvalidInput
	}
	return &BigBlueButton{
		endpoint: endpoint, public: public, secret: config.Secret, algorithm: config.Algorithm,
		client: newBigBlueButtonClient(config.Roots), logger: config.Logger, timing: timing,
	}, nil
}

func newBigBlueButtonClient(roots *x509.CertPool) *http.Client {
	return &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport: &http.Transport{
			TLSClientConfig:        &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots},
			DisableKeepAlives:      true,
			DisableCompression:     true,
			MaxResponseHeaderBytes: bbbMaxResponseHeaderBytes,
		},
	}
}

func canonicalHTTPSBase(raw string) (string, bool) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || parsed.Opaque != "" {
		return "", false
	}
	return strings.TrimRight(parsed.String(), "/"), true
}

func (algorithm ChecksumAlgorithm) valid() bool {
	return algorithm == ChecksumSHA256 || algorithm == ChecksumSHA384 || algorithm == ChecksumSHA512
}

func (algorithm ChecksumAlgorithm) digest(payload string) string {
	switch algorithm {
	case ChecksumSHA256:
		sum := sha256.Sum256([]byte(payload))
		return hex.EncodeToString(sum[:])
	case ChecksumSHA384:
		sum := sha512.Sum384([]byte(payload))
		return hex.EncodeToString(sum[:])
	case ChecksumSHA512:
		sum := sha512.Sum512([]byte(payload))
		return hex.EncodeToString(sum[:])
	}
	panic("the constructor admits only supported checksum algorithms")
}

func encodeBigBlueButtonQuery(values url.Values) string {
	return strings.NewReplacer("~", "%7E", "%2A", "*").Replace(values.Encode())
}

func (b *BigBlueButton) signedURL(call string, values url.Values) string {
	query := encodeBigBlueButtonQuery(values)
	checksum := b.algorithm.digest(call + query + b.secret.Reveal())
	return b.endpoint + "/" + call + "?" + query + "&checksum=" + checksum
}

type bbbDocument struct {
	XMLName              xml.Name `xml:"response"`
	ReturnCode           []string `xml:"returncode"`
	MessageKey           []string `xml:"messageKey"`
	MeetingID            []string `xml:"meetingID"`
	CreateTime           []int64  `xml:"createTime"`
	Running              []bool   `xml:"running"`
	ParticipantCount     []int    `xml:"participantCount"`
	EndTime              []int64  `xml:"endTime"`
	HasBeenForciblyEnded []bool   `xml:"hasBeenForciblyEnded"`
}

func (document bbbDocument) response() (bbbResponse, bool) {
	unique := len(document.ReturnCode) <= 1 && len(document.MessageKey) <= 1 && len(document.MeetingID) <= 1 && len(document.CreateTime) <= 1 &&
		len(document.Running) <= 1 && len(document.ParticipantCount) <= 1 && len(document.EndTime) <= 1 && len(document.HasBeenForciblyEnded) <= 1
	if !unique {
		return bbbResponse{}, false
	}
	return bbbResponse{
		ReturnCode: firstOrZero(document.ReturnCode), MessageKey: firstOrZero(document.MessageKey), MeetingID: firstOrZero(document.MeetingID),
		CreateTime: firstOrNil(document.CreateTime), Running: firstOrNil(document.Running), ParticipantCount: firstOrNil(document.ParticipantCount),
		EndTime: firstOrNil(document.EndTime), HasBeenForciblyEnded: firstOrNil(document.HasBeenForciblyEnded),
	}, true
}

func firstOrZero(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func firstOrNil[T int | int64 | bool](values []T) *T {
	if len(values) == 0 {
		return nil
	}
	return &values[0]
}

type bbbResponse struct {
	ReturnCode           string
	MessageKey           string
	MeetingID            string
	CreateTime           *int64
	Running              *bool
	ParticipantCount     *int
	EndTime              *int64
	HasBeenForciblyEnded *bool
}

func (response bbbResponse) ended() bool {
	return (response.EndTime != nil && *response.EndTime > 0) || (response.HasBeenForciblyEnded != nil && *response.HasBeenForciblyEnded)
}

func (b *BigBlueButton) call(ctx context.Context, name string, values url.Values) (bbbResponse, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, b.signedURL(name, values), http.NoBody)
	if err != nil {
		return bbbResponse{}, FailureInvalidInput
	}
	answer, err := b.client.Do(request)
	if err != nil {
		return bbbResponse{}, FailureUnavailable
	}
	defer closeBigBlueButtonBody(ctx, b.logger, answer.Body)
	if answer.StatusCode >= http.StatusMultipleChoices && answer.StatusCode < http.StatusBadRequest {
		return bbbResponse{}, FailureRedirected
	}
	if answer.StatusCode != http.StatusOK {
		return bbbResponse{}, FailureUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(answer.Body, bbbMaxResponseBytes+1))
	if err != nil {
		return bbbResponse{}, FailureUnavailable
	}
	if len(body) > bbbMaxResponseBytes {
		return bbbResponse{}, FailureOversized
	}
	return parseBigBlueButtonResponse(body)
}

func closeBigBlueButtonBody(ctx context.Context, logger *slog.Logger, body io.Closer) {
	if err := body.Close(); err != nil {
		logger.LogAttrs(ctx, slog.LevelDebug, "meeting.provider.body_close_failed", slog.String("kind", "transport"))
	}
}

func parseBigBlueButtonResponse(body []byte) (bbbResponse, error) {
	decoder := xml.NewDecoder(bytes.NewReader(body))
	root, found := bigBlueButtonRoot(decoder)
	if !found {
		return bbbResponse{}, FailureMalformed
	}
	var document bbbDocument
	if err := decoder.DecodeElement(&document, &root); err != nil {
		return bbbResponse{}, FailureMalformed
	}
	if !bigBlueButtonDocumentEnded(decoder) {
		return bbbResponse{}, FailureMalformed
	}
	response, unique := document.response()
	if !unique || (response.ReturnCode != bbbReturnSuccess && response.ReturnCode != bbbReturnFailed) {
		return bbbResponse{}, FailureMalformed
	}
	return response, nil
}

func bigBlueButtonRoot(decoder *xml.Decoder) (xml.StartElement, bool) {
	for {
		token, err := decoder.Token()
		if err != nil {
			return xml.StartElement{}, false
		}
		switch typed := token.(type) {
		case xml.StartElement:
			return typed, true
		case xml.ProcInst:
			if typed.Target != "xml" {
				return xml.StartElement{}, false
			}
		case xml.CharData:
			if len(bytes.TrimSpace(typed)) > 0 {
				return xml.StartElement{}, false
			}
		default:
			return xml.StartElement{}, false
		}
	}
}

func bigBlueButtonDocumentEnded(decoder *xml.Decoder) bool {
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return true
		}
		if err != nil {
			return false
		}
		text, isText := token.(xml.CharData)
		if !isText || len(bytes.TrimSpace(text)) > 0 {
			return false
		}
	}
}

func (b *BigBlueButton) bounded(ctx context.Context, operation func(context.Context) error) error {
	bounded, cancel := b.timing.bound(ctx)
	defer cancel()
	err := operation(bounded)
	if err == nil {
		return nil
	}
	if cause := ctx.Err(); cause != nil {
		return cause
	}
	return err
}

func (b *BigBlueButton) CreateMeeting(ctx context.Context, requested domain.Meeting) (created domain.ProviderCreateTime, err error) {
	defer reportProvider(ctx, b.logger, operationCreate, &err)
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if !b.validCreate(requested) {
		return 0, FailureInvalidInput
	}
	err = b.bounded(ctx, func(bounded context.Context) error {
		var createErr error
		created, createErr = b.create(bounded, requested)
		return createErr
	})
	if err != nil {
		return 0, err
	}
	return created, nil
}

func (b *BigBlueButton) validCreate(requested domain.Meeting) bool {
	if requested.ID == (domain.ProviderMeetingID{}) || requested.RoutingKey == (domain.MeetingRoutingKey{}) || requested.LessonID == (domain.LessonID{}) {
		return false
	}
	return validName(requested.Name, bbbRoomNameMaxRunes) && utf8.RuneCountInString(requested.Name) >= bbbRoomNameMinRunes
}

func (b *BigBlueButton) create(ctx context.Context, requested domain.Meeting) (domain.ProviderCreateTime, error) {
	if requested.CreateTime != 0 {
		return b.existingGeneration(ctx, requested)
	}
	response, err := b.call(ctx, bbbCallCreate, b.createValues(requested))
	if err != nil {
		return 0, err
	}
	if response.ReturnCode == bbbReturnFailed {
		if response.MessageKey == bbbKeyIDNotUnique {
			return 0, FailureConflict
		}
		return 0, FailureRejected
	}
	if response.MeetingID != requested.ID.String() {
		return 0, FailureMismatch
	}
	if response.CreateTime == nil || *response.CreateTime <= 0 {
		return 0, FailureMalformed
	}
	if response.ended() {
		return 0, FailureConflict
	}
	return domain.ProviderCreateTime(*response.CreateTime), nil
}

func (b *BigBlueButton) existingGeneration(ctx context.Context, requested domain.Meeting) (domain.ProviderCreateTime, error) {
	info, err := b.inspect(ctx, requested.ID)
	if err != nil {
		return 0, err
	}
	if info.Status == domain.MeetingNotFound {
		return 0, FailureInvalidInput
	}
	if info.CreateTime != requested.CreateTime {
		return 0, FailureConflict
	}
	return info.CreateTime, nil
}

func (b *BigBlueButton) createValues(requested domain.Meeting) url.Values {
	return url.Values{
		"meetingID":                            {requested.ID.String()},
		"name":                                 {requested.Name},
		"record":                               {"false"},
		"autoStartRecording":                   {"false"},
		"allowStartStopRecording":              {"false"},
		"meetingExpireIfNoUserJoinedInMinutes": {bbbExpireWithoutJoinMinutes},
		"endWhenNoModerator":                   {"true"},
		"endWhenNoModeratorDelayInMinutes":     {bbbEndWithoutModeratorDelay},
		"logoutURL":                            {b.public + "/lessons/" + requested.LessonID.String()},
		"meetingEndedURL":                      {b.public + "/api/v1/integrations/bbb/" + requested.RoutingKey.String() + "/ended"},
	}
}

func (b *BigBlueButton) inspect(ctx context.Context, id domain.ProviderMeetingID) (domain.MeetingInfo, error) {
	if id == (domain.ProviderMeetingID{}) {
		return domain.MeetingInfo{}, FailureInvalidInput
	}
	response, err := b.call(ctx, bbbCallInfo, url.Values{"meetingID": {id.String()}})
	if err != nil {
		return domain.MeetingInfo{}, err
	}
	if response.ReturnCode == bbbReturnFailed {
		if response.MessageKey == bbbKeyNotFound {
			return domain.MeetingInfo{Status: domain.MeetingNotFound}, nil
		}
		return domain.MeetingInfo{}, FailureRejected
	}
	if response.MeetingID != id.String() {
		return domain.MeetingInfo{}, FailureMismatch
	}
	if response.ended() {
		return domain.MeetingInfo{Status: domain.MeetingNotFound}, nil
	}
	if response.CreateTime == nil || *response.CreateTime <= 0 || response.Running == nil || response.ParticipantCount == nil || *response.ParticipantCount < 0 {
		return domain.MeetingInfo{}, FailureMalformed
	}
	info := domain.MeetingInfo{Status: domain.MeetingCreated, CreateTime: domain.ProviderCreateTime(*response.CreateTime), Participants: *response.ParticipantCount}
	if *response.Running || info.Participants > 0 {
		info.Status = domain.MeetingRunning
	}
	return info, nil
}

func (b *BigBlueButton) InspectMeeting(ctx context.Context, id domain.ProviderMeetingID) (info domain.MeetingInfo, err error) {
	defer reportProvider(ctx, b.logger, operationInspect, &err)
	if err := ctx.Err(); err != nil {
		return domain.MeetingInfo{}, err
	}
	err = b.bounded(ctx, func(bounded context.Context) error {
		var inspectErr error
		info, inspectErr = b.inspect(bounded, id)
		return inspectErr
	})
	if err != nil {
		return domain.MeetingInfo{}, err
	}
	return info, nil
}

func (b *BigBlueButton) MeetingInfo(ctx context.Context, requested domain.Meeting) (info domain.MeetingInfo, err error) {
	defer reportProvider(ctx, b.logger, operationInfo, &err)
	if err := ctx.Err(); err != nil {
		return domain.MeetingInfo{}, err
	}
	err = b.bounded(ctx, func(bounded context.Context) error {
		var inspectErr error
		info, inspectErr = b.inspect(bounded, requested.ID)
		return inspectErr
	})
	if err != nil {
		return domain.MeetingInfo{}, err
	}
	if info.CreateTime != requested.CreateTime {
		return domain.MeetingInfo{Status: domain.MeetingNotFound}, nil
	}
	return info, nil
}

func (b *BigBlueButton) JoinLink(ctx context.Context, requested domain.Meeting, participant domain.Participant) (link JoinLink, err error) {
	defer reportProvider(ctx, b.logger, operationJoin, &err)
	if err := ctx.Err(); err != nil {
		return JoinLink{}, err
	}
	role, valid := bigBlueButtonRole(participant)
	if !valid {
		return JoinLink{}, FailureInvalidInput
	}
	err = b.bounded(ctx, func(bounded context.Context) error {
		info, inspectErr := b.inspect(bounded, requested.ID)
		if inspectErr != nil {
			return inspectErr
		}
		if info.Status == domain.MeetingNotFound || info.CreateTime != requested.CreateTime {
			return FailureNotFound
		}
		link = NewJoinLink(b.signedURL(bbbCallJoin, url.Values{
			"meetingID":  {requested.ID.String()},
			"createTime": {strconv.FormatInt(int64(requested.CreateTime), 10)},
			"fullName":   {participant.Name},
			"role":       {role},
			"userID":     {participant.ID.String()},
		}))
		return nil
	})
	if err != nil {
		return JoinLink{}, err
	}
	return link, nil
}

func bigBlueButtonRole(participant domain.Participant) (string, bool) {
	if participant.ID == (domain.ParticipantID{}) || !validName(participant.Name, domain.ParticipantNameMaxRunes) {
		return "", false
	}
	switch participant.Role {
	case domain.ParticipantModerator:
		return "MODERATOR", true
	case domain.ParticipantViewer:
		return "VIEWER", true
	}
	return "", false
}

func (b *BigBlueButton) EndMeeting(ctx context.Context, requested domain.Meeting) (err error) {
	defer reportProvider(ctx, b.logger, operationEnd, &err)
	if err := ctx.Err(); err != nil {
		return err
	}
	return b.bounded(ctx, func(bounded context.Context) error { return b.end(bounded, requested) })
}

func (b *BigBlueButton) end(ctx context.Context, requested domain.Meeting) error {
	current, err := b.inspect(ctx, requested.ID)
	if err != nil {
		return err
	}
	if current.Status == domain.MeetingNotFound {
		return nil
	}
	if current.CreateTime != requested.CreateTime {
		return FailureNotFound
	}
	response, err := b.call(ctx, bbbCallEnd, url.Values{"meetingID": {requested.ID.String()}})
	if err != nil {
		return err
	}
	if response.ReturnCode == bbbReturnFailed {
		if response.MessageKey == bbbKeyNotFound || response.MessageKey == bbbKeyForciblyEnded {
			return b.confirmEnded(ctx, requested)
		}
		return FailureRejected
	}
	if response.MessageKey != bbbKeyEndRequestSent {
		return FailureMalformed
	}
	return b.confirmEnded(ctx, requested)
}

func (b *BigBlueButton) confirmEnded(ctx context.Context, requested domain.Meeting) error {
	for read := range b.timing.confirmReads {
		if read > 0 {
			if err := waitBigBlueButton(ctx, b.timing.confirmInterval); err != nil {
				return err
			}
		}
		current, err := b.inspect(ctx, requested.ID)
		if err != nil {
			return err
		}
		if current.Status == domain.MeetingNotFound {
			return nil
		}
		if current.CreateTime != requested.CreateTime {
			return FailureConflict
		}
	}
	return FailureUnavailable
}

func waitBigBlueButton(ctx context.Context, interval time.Duration) error {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return FailureUnavailable
	case <-timer.C:
		return nil
	}
}
