package meeting

import (
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"crypto/x509"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
)

const (
	protocolSecret   = "protocol-fake-shared-value-for-local-tests"
	protocolBasePath = "/bigbluebutton/api/"
)

type protocolRequest struct {
	method string
	call   string
	values url.Values
}

type protocolRoom struct {
	createTime   int64
	participants map[string]string
	ending       bool
	endReads     int
	ended        bool
}

type protocolFault struct {
	applyState bool
	respond    func(http.ResponseWriter, *http.Request)
}

type protocolFake struct {
	server         *httptest.Server
	mutex          sync.Mutex
	rooms          map[string]*protocolRoom
	lastCreateTime int64
	requests       []protocolRequest
	faults         map[string]protocolFault
	endReads       int
	reportEnded    bool
	infoReads      chan struct{}
}

func newProtocolFake(t *testing.T) *protocolFake {
	t.Helper()
	fake := &protocolFake{rooms: make(map[string]*protocolRoom), faults: make(map[string]protocolFault), lastCreateTime: 1_700_000_000_000, infoReads: make(chan struct{}, 64)}
	fake.server = httptest.NewUnstartedServer(http.HandlerFunc(fake.handle))
	fake.server.Config.ErrorLog = log.New(io.Discard, "", 0)
	fake.server.StartTLS()
	t.Cleanup(fake.server.Close)
	return fake
}

func (fake *protocolFake) endpoint() string {
	return fake.server.URL + strings.TrimSuffix(protocolBasePath, "/")
}

func (fake *protocolFake) roots() *x509.CertPool {
	roots := x509.NewCertPool()
	roots.AddCert(fake.server.Certificate())
	return roots
}

func (fake *protocolFake) browser() *http.Client {
	return newBigBlueButtonClient(fake.roots())
}

func (fake *protocolFake) setFault(call string, fault protocolFault) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.faults[call] = fault
}

func (fake *protocolFake) clearFaults() {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.faults = make(map[string]protocolFault)
}

func (fake *protocolFake) setEndReads(reads int) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.endReads = reads
}

func (fake *protocolFake) setReportEnded(report bool) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.reportEnded = report
}

func (fake *protocolFake) captured(call string) []protocolRequest {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	matching := make([]protocolRequest, 0, len(fake.requests))
	for _, request := range fake.requests {
		if request.call == call {
			matching = append(matching, request)
		}
	}
	return matching
}

func (fake *protocolFake) requestCount() int {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	return len(fake.requests)
}

func (fake *protocolFake) participants(meetingID string) map[string]string {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	room, found := fake.rooms[meetingID]
	if !found {
		return nil
	}
	copied := make(map[string]string, len(room.participants))
	for id, role := range room.participants {
		copied[id] = role
	}
	return copied
}

func (fake *protocolFake) handle(writer http.ResponseWriter, request *http.Request) {
	call, inBase := strings.CutPrefix(request.URL.Path, protocolBasePath)
	if !inBase {
		http.NotFound(writer, request)
		return
	}
	values, signed := verifiedProtocolQuery(call, request.URL.RawQuery)
	if !signed {
		writeProtocolAnswer(writer, protocolRefusal("checksumError"))
		return
	}
	fake.mutex.Lock()
	fake.requests = append(fake.requests, protocolRequest{method: request.Method, call: call, values: values})
	fault, faulted := fake.faults[call]
	fake.mutex.Unlock()
	if faulted {
		if fault.applyState {
			fake.apply(httptest.NewRecorder(), call, values)
		}
		fault.respond(writer, request)
		return
	}
	fake.apply(writer, call, values)
}

func (fake *protocolFake) apply(writer http.ResponseWriter, call string, values url.Values) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	switch call {
	case bbbCallCreate:
		fake.create(writer, values)
	case bbbCallInfo:
		fake.info(writer, values)
	case bbbCallEnd:
		fake.end(writer, values)
	case bbbCallJoin:
		fake.join(writer, values)
	default:
		writeProtocolAnswer(writer, protocolRefusal("unknownCall"))
	}
}

type protocolAnswer struct {
	XMLName          xml.Name `xml:"response"`
	ReturnCode       string   `xml:"returncode"`
	MeetingID        string   `xml:"meetingID,omitempty"`
	CreateTime       int64    `xml:"createTime,omitempty"`
	Running          *bool    `xml:"running,omitempty"`
	ParticipantCount *int     `xml:"participantCount,omitempty"`
	EndTime          *int64   `xml:"endTime,omitempty"`
	ForciblyEnded    *bool    `xml:"hasBeenForciblyEnded,omitempty"`
	MessageKey       string   `xml:"messageKey,omitempty"`
}

func protocolSuccess(key string) protocolAnswer {
	return protocolAnswer{ReturnCode: bbbReturnSuccess, MessageKey: key}
}

func protocolRefusal(key string) protocolAnswer {
	return protocolAnswer{ReturnCode: bbbReturnFailed, MessageKey: key}
}

func (fake *protocolFake) create(writer http.ResponseWriter, values url.Values) {
	id, name := values.Get("meetingID"), values.Get("name")
	if id == "" || utf8.RuneCountInString(name) < 2 || utf8.RuneCountInString(name) > 64 {
		writeProtocolAnswer(writer, protocolRefusal("validationError"))
		return
	}
	notEnded := false
	if room, found := fake.rooms[id]; found && !room.ended {
		writeProtocolAnswer(writer, protocolAnswer{ReturnCode: bbbReturnSuccess, MeetingID: id, CreateTime: room.createTime, ForciblyEnded: &notEnded, MessageKey: "duplicateWarning"})
		return
	}
	fake.lastCreateTime++
	fake.rooms[id] = &protocolRoom{createTime: fake.lastCreateTime, participants: make(map[string]string)}
	writeProtocolAnswer(writer, protocolAnswer{ReturnCode: bbbReturnSuccess, MeetingID: id, CreateTime: fake.lastCreateTime, ForciblyEnded: &notEnded})
}

func (fake *protocolFake) info(writer http.ResponseWriter, values url.Values) {
	select {
	case fake.infoReads <- struct{}{}:
	default:
	}
	id := values.Get("meetingID")
	room, found := fake.rooms[id]
	if !found || (room.ended && !fake.reportEnded) {
		writeProtocolAnswer(writer, protocolRefusal(bbbKeyNotFound))
		return
	}
	if room.ending {
		if room.endReads > 0 {
			room.endReads--
		} else {
			room.ended = true
			if !fake.reportEnded {
				writeProtocolAnswer(writer, protocolRefusal(bbbKeyNotFound))
				return
			}
		}
	}
	endTime := int64(0)
	if room.ended {
		endTime = room.createTime + 1
	}
	running, count := len(room.participants) > 0, len(room.participants)
	writeProtocolAnswer(writer, protocolAnswer{ReturnCode: bbbReturnSuccess, MeetingID: id, CreateTime: room.createTime, Running: &running, ParticipantCount: &count, EndTime: &endTime, ForciblyEnded: &room.ended})
}

func (fake *protocolFake) end(writer http.ResponseWriter, values url.Values) {
	room, found := fake.rooms[values.Get("meetingID")]
	if !found || room.ended {
		writeProtocolAnswer(writer, protocolRefusal(bbbKeyNotFound))
		return
	}
	room.ending, room.endReads = true, fake.endReads
	writeProtocolAnswer(writer, protocolSuccess(bbbKeyEndRequestSent))
}

func (fake *protocolFake) join(writer http.ResponseWriter, values url.Values) {
	room, found := fake.rooms[values.Get("meetingID")]
	if !found || room.ended || room.ending {
		writeProtocolAnswer(writer, protocolRefusal(bbbKeyForciblyEnded))
		return
	}
	if values.Get("createTime") != strconv.FormatInt(room.createTime, 10) {
		writeProtocolAnswer(writer, protocolRefusal("mismatchCreateTimeParam"))
		return
	}
	role := values.Get("role")
	if (role != "MODERATOR" && role != "VIEWER") || values.Get("fullName") == "" || values.Get("userID") == "" {
		writeProtocolAnswer(writer, protocolRefusal("validationError"))
		return
	}
	room.participants[values.Get("userID")] = role
	writeProtocolAnswer(writer, protocolSuccess("successfullyJoined"))
}

func protocolFailure(key string) string {
	return `<response><returncode>FAILED</returncode><messageKey>` + key + `</messageKey><message>provider detail text</message></response>`
}

func writeProtocolAnswer(writer http.ResponseWriter, answer protocolAnswer) {
	writer.Header().Set("Content-Type", "text/xml")
	if err := xml.NewEncoder(writer).Encode(answer); err != nil {
		panic("protocol fake response write failed: " + err.Error())
	}
}

func verifiedProtocolQuery(call, raw string) (url.Values, bool) {
	query, checksum, found := strings.Cut(raw, "&checksum=")
	if !found || strings.Contains(checksum, "&") {
		return nil, false
	}
	if !protocolChecksumMatches(call+query+protocolSecret, checksum) {
		return nil, false
	}
	values, err := url.ParseQuery(query)
	if err != nil || protocolCanonicalQuery(values) != query {
		return nil, false
	}
	return values, true
}

func protocolChecksumMatches(payload, checksum string) bool {
	var expected string
	switch len(checksum) {
	case sha256.Size * 2:
		sum := sha256.Sum256([]byte(payload))
		expected = hex.EncodeToString(sum[:])
	case sha512.Size384 * 2:
		sum := sha512.Sum384([]byte(payload))
		expected = hex.EncodeToString(sum[:])
	case sha512.Size * 2:
		sum := sha512.Sum512([]byte(payload))
		expected = hex.EncodeToString(sum[:])
	default:
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(checksum)) == 1
}

func protocolCanonicalQuery(values url.Values) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	pairs := make([]string, 0, len(keys))
	for _, key := range keys {
		pairs = append(pairs, protocolFormEncode(key)+"="+protocolFormEncode(values.Get(key)))
	}
	return strings.Join(pairs, "&")
}

func protocolFormEncode(value string) string {
	var encoded strings.Builder
	for _, unit := range []byte(value) {
		switch {
		case unit >= 'a' && unit <= 'z', unit >= 'A' && unit <= 'Z', unit >= '0' && unit <= '9', strings.IndexByte(".-*_", unit) >= 0:
			encoded.WriteByte(unit)
		case unit == ' ':
			encoded.WriteByte('+')
		default:
			fmt.Fprintf(&encoded, "%%%02X", unit)
		}
	}
	return encoded.String()
}

func protocolWellFormed(body string) bool {
	var document struct {
		XMLName xml.Name `xml:"response"`
	}
	return xml.Unmarshal([]byte(body), &document) == nil
}
