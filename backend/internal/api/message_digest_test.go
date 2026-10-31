package api_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/adapters/mail"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/services/chat"
)

func digestFixture(t *testing.T) (*authHarness, endingParties, string) {
	t.Helper()
	h := newFlowHarness(t)
	parties := h.activeStudentInAGroup(t)
	channel := h.groupChannel(t, parties.group.ID).ID
	h.sent(t, parties.tutorToken, channel, "Private digest body must never leave chat")
	return h, parties, channel
}

func (h *authHarness) digestService(t *testing.T, settings chat.DigestSettings) *chat.Service {
	t.Helper()
	return chat.New(chat.Dependencies{Store: h.store, Clock: h.clock, Random: randomPerTest(t), Logger: slog.New(slog.DiscardHandler), Digest: settings})
}

func (h *authHarness) queueDigests(t *testing.T) {
	t.Helper()
	if _, err := h.digestService(t, chat.DefaultDigestSettings()).QueueDigests(context.Background(), h.now(t)); err != nil {
		t.Fatal(err)
	}
}

func (h *authHarness) digestsFor(t *testing.T, recipient string) []mail.SinkMessage {
	t.Helper()
	var found []mail.SinkMessage
	for _, message := range h.mailTo(t, recipient) {
		if message.Subject == "Unread messages" || message.Subject == "Непрочитанные сообщения" {
			found = append(found, message)
		}
	}
	return found
}

func TestADigestIsSentAfterTenMinutesOffline(t *testing.T) {
	t.Parallel()
	h, parties, _ := digestFixture(t)
	h.tickAt(t, authStart.Add(9*time.Minute))
	before := len(h.digestsFor(t, studentEmail))

	h.tickAt(t, authStart.Add(10*time.Minute))

	messages := h.digestsFor(t, studentEmail)
	if before != 0 || len(messages) != 1 || !strings.Contains(messages[0].Text, groupName+": 1") || h.digestPending(t, parties.student) {
		t.Fatalf("before %d, digests %v, pending %t", before, subjects(messages), h.digestPending(t, parties.student))
	}
}

func TestNoSecondDigestForTheSameMessages(t *testing.T) {
	t.Parallel()
	h, _, _ := digestFixture(t)
	h.tickAt(t, authStart.Add(10*time.Minute))

	h.tickAt(t, authStart.Add(41*time.Minute))

	if messages := h.digestsFor(t, studentEmail); len(messages) != 1 {
		t.Fatalf("got %d digests", len(messages))
	}
}

func TestDigestsAreAtLeastThirtyMinutesApart(t *testing.T) {
	t.Parallel()
	h, parties, channel := digestFixture(t)
	h.tickAt(t, authStart.Add(10*time.Minute))
	token := accessToken(t, h.login(t, tutorEmail, accountPassword))
	h.sent(t, token, channel, "Another message")
	h.tickAt(t, authStart.Add(39*time.Minute))
	before := len(h.digestsFor(t, studentEmail))

	h.tickAt(t, authStart.Add(40*time.Minute))

	messages := h.digestsFor(t, studentEmail)
	if before != 1 || len(messages) != 2 || !strings.Contains(messages[1].Text, groupName+": 1") || h.digestPending(t, parties.student) {
		t.Fatalf("before %d, digests %v", before, subjects(messages))
	}
}

func TestReadingClearsTheDigestFlag(t *testing.T) {
	t.Parallel()
	h, parties, channel := digestFixture(t)
	direct := h.directChannel(t, parties.card.ID).ID
	h.sent(t, parties.tutorToken, direct, "Unread elsewhere")
	first, _ := h.readChannel(t, parties.studentToken, channel, 1)
	h.mustAnswer(t, first, http.StatusOK)
	afterFirst := h.digestPending(t, parties.student)

	second, _ := h.readChannel(t, parties.studentToken, direct, 1)

	h.mustAnswer(t, second, http.StatusOK)
	if !afterFirst || h.digestPending(t, parties.student) {
		t.Fatal("reading did not preserve another unread channel or clear the last one")
	}
}

func TestDigestCountsOnlyChannelsTheMemberStillReachesAtDispatch(t *testing.T) {
	t.Parallel()
	h, parties, channel := digestFixture(t)
	direct := h.directChannel(t, parties.card.ID).ID
	h.sent(t, parties.tutorToken, direct, "Direct unread")
	h.at(t, authStart.Add(11*time.Minute))
	h.queueDigests(t)
	token := accessToken(t, h.login(t, tutorEmail, accountPassword))
	h.mustAnswer(t, h.groupCall(t, token, http.MethodPatch, parties.group.ID, "", fmt.Sprintf(`{"remove":[%q]}`, parties.card.ID)), http.StatusOK)

	h.tick(t)

	messages := h.digestsFor(t, studentEmail)
	if len(messages) != 1 || strings.Contains(messages[0].Text, groupName) || !strings.Contains(messages[0].Text, endingTutorAccountName+": 1") || h.messageCount(t, channel) != 1 {
		t.Fatalf("dispatch did not enforce current membership: %v", subjects(messages))
	}
}

func TestDigestCountsOnlyMessagesThatStillHaveABody(t *testing.T) {
	t.Parallel()
	h, parties, channel := digestFixture(t)
	second := h.sent(t, parties.tutorToken, channel, "Deleted before dispatch")
	h.at(t, authStart.Add(11*time.Minute))
	h.queueDigests(t)
	token := accessToken(t, h.login(t, tutorEmail, accountPassword))
	h.mustAnswer(t, h.deleteMessage(t, token, second.ID), http.StatusNoContent)

	h.tick(t)

	messages := h.digestsFor(t, studentEmail)
	if len(messages) != 1 || !strings.Contains(messages[0].Text, groupName+": 1") {
		t.Fatalf("wrong remaining count: %v", subjects(messages))
	}
}

func TestADigestWithNoRemainingMessagesIsDropped(t *testing.T) {
	t.Parallel()
	h, parties, channel := digestFixture(t)
	h.at(t, authStart.Add(11*time.Minute))
	h.queueDigests(t)
	token := accessToken(t, h.login(t, studentEmail, accountPassword))
	read, _ := h.readChannel(t, token, channel, 1)
	h.mustAnswer(t, read, http.StatusOK)

	h.tick(t)

	if len(h.digestsFor(t, studentEmail)) != 0 || h.outboxOf(t, domain.EventMessagesDigest, "done:obsolete") != 1 || h.digestPending(t, parties.student) {
		t.Fatal("a fully read digest was delivered")
	}
	if h.countLogs(`"msg":"email.dropped"`, `"reason":"obsolete_digest"`) != 1 {
		t.Fatalf("missing obsolete diagnostic: %s", h.logs.String())
	}
}

func TestDigestNamesChannelsByTheRecipientsLabelsWithoutText(t *testing.T) {
	t.Parallel()
	h, parties, _ := digestFixture(t)
	direct := h.directChannel(t, parties.card.ID).ID
	h.sent(t, parties.studentToken, direct, "Student secret body")

	h.tickAt(t, authStart.Add(11*time.Minute))

	tutor := h.digestsFor(t, tutorEmail)
	student := h.digestsFor(t, studentEmail)
	if len(tutor) != 1 || len(student) != 1 || !strings.Contains(tutor[0].Text, cardName+": 1") || !strings.Contains(student[0].Text, groupName+": 1") {
		t.Fatal("recipient labels differ")
	}
	for _, message := range append(tutor, student...) {
		for _, forbidden := range []string{"Private digest body", "Student secret body", endingStudentName} {
			if strings.Contains(message.Text, forbidden) || strings.Contains(message.HTML, forbidden) {
				t.Fatalf("digest leaks %q", forbidden)
			}
		}
	}
}

func TestDigestCarriesOneClickUnsubscribe(t *testing.T) {
	t.Parallel()
	h, _, _ := digestFixture(t)
	h.tickAt(t, authStart.Add(11*time.Minute))
	messages := h.digestsFor(t, studentEmail)
	if len(messages) != 1 {
		t.Fatalf("got %d digests", len(messages))
	}
	token := headerToken(t, messages[0])

	answer := h.unsubscribe(t, token, formType, oneClickForm)

	h.mustAnswer(t, answer, http.StatusNoContent)
	page := publicOrigin + "/unsubscribe?token=" + url.QueryEscape(token)
	if messages[0].Headers["List-Unsubscribe-Post"] != "List-Unsubscribe=One-Click" || !strings.Contains(messages[0].Text, page) || !containsLink(messages[0].Links, page) {
		t.Fatal("digest unsubscribe missing")
	}
	preferences := h.storedPreferences(t, domain.UserID{2})
	if len(preferences) != 1 || preferences[0] != "message_digest:false" {
		t.Fatalf("wrong preference: %v", preferences)
	}
}

func TestATurnedOffDigestIsDropped(t *testing.T) {
	t.Parallel()
	h, parties, _ := digestFixture(t)
	h.at(t, authStart.Add(11*time.Minute))
	h.queueDigests(t)
	token := accessToken(t, h.login(t, studentEmail, accountPassword))
	h.mustAnswer(t, h.savePreferences(t, token, `{"groups":[{"group":"message_digest","enabled":false}]}`), http.StatusOK)

	h.tick(t)

	if len(h.digestsFor(t, studentEmail)) != 0 || h.digestPending(t, parties.student) {
		t.Fatal("disabled digest delivered or retained")
	}
	if h.countLogs(`"msg":"email.dropped"`, `"reason":"preference"`, `"group":"message_digest"`) != 1 {
		t.Fatal("preference drop diagnostic missing")
	}
}

func TestADigestToAnUnconfirmedAccountIsDropped(t *testing.T) {
	t.Parallel()
	h, parties, _ := digestFixture(t)
	if _, err := h.database.Pool.Exec(context.Background(), "UPDATE users SET state = 'unverified', email_verified_at = NULL WHERE id = $1", parties.student); err != nil {
		t.Fatal(err)
	}

	h.tickAt(t, authStart.Add(11*time.Minute))

	if len(h.digestsFor(t, studentEmail)) != 0 || h.outboxOf(t, domain.EventMessagesDigest, "done:obsolete") != 1 {
		t.Fatal("unconfirmed address received digest")
	}
}

func TestDigestEmailsUseTheRecipientsLanguage(t *testing.T) {
	t.Parallel()
	h, parties, _ := digestFixture(t)
	h.setAccount(t, parties.student, endingStudentName, domain.LanguageRussian)

	h.tickAt(t, authStart.Add(11*time.Minute))

	messages := h.digestsFor(t, studentEmail)
	if len(messages) != 1 || messages[0].Subject != "Непрочитанные сообщения" || !strings.Contains(messages[0].Text, "Открыть сообщения") || !strings.Contains(messages[0].HTML, `lang="ru"`) {
		t.Fatalf("incorrect localized digest: %v", subjects(messages))
	}
}

func TestDigestListsAtMostTwentyChannelsAndEscapesTheirLabels(t *testing.T) {
	t.Parallel()
	h, parties, _ := digestFixture(t)
	for index := 0; index < 21; index++ {
		group := h.newGroup(t, parties.tutorToken, fmt.Sprintf("<b>Channel %02d</b>", index), []string{parties.card.ID})
		h.sent(t, parties.tutorToken, h.groupChannel(t, group.ID).ID, "Secret text")
	}

	h.tickAt(t, authStart.Add(11*time.Minute))

	messages := h.digestsFor(t, studentEmail)
	if len(messages) != 1 || strings.Count(messages[0].HTML, "<li>") != 20 || !strings.Contains(messages[0].Text, "and 2 more") || strings.Contains(messages[0].HTML, "<b>") || strings.Contains(messages[0].Text, "Secret text") {
		t.Fatalf("digest limit or escaping failed: %v", subjects(messages))
	}
}

func TestDigestStateIsRemovedByStudentAndTutorPurge(t *testing.T) {
	for _, role := range []domain.Role{domain.RoleTutor, domain.RoleStudent} {
		t.Run(string(role), func(t *testing.T) {
			t.Parallel()
			h, parties, _ := digestFixture(t)
			h.sent(t, parties.studentToken, h.directChannel(t, parties.card.ID).ID, "Tutor unread")
			h.tickAt(t, authStart.Add(11*time.Minute))
			email, user := studentEmail, parties.student
			if role == domain.RoleTutor {
				email, user = tutorEmail, parties.tutor
			}
			if h.countRows(t, "SELECT count(user_id) FROM digest_state WHERE user_id = $1", user.String()) != 1 {
				t.Fatal("digest state was not created")
			}
			token := h.relogin(t, email)
			h.mustAnswer(t, h.deleteMe(t, token, accountPassword), http.StatusNoContent)
			h.advance(t, domain.DeletedAccountRetention+time.Hour)

			h.tick(t)

			if h.userExists(t, email) || h.countRows(t, "SELECT count(user_id) FROM digest_state WHERE user_id = $1", user.String()) != 0 || h.digestPending(t, user) {
				t.Fatal("purge left digest bookkeeping")
			}
		})
	}
}

func TestDigestReadsAggregatePresenceAcrossAPIInstances(t *testing.T) {
	t.Parallel()
	h := newLiveHarness(t, liveTuning{})
	other := h.anotherInstance(t)
	parties := h.groupOfTwoStudents(t)
	h.sent(t, parties.tutorToken, parties.groupChannel, "Offline message before connecting")
	first := h.connect(t, parties.firstToken)
	last := other.connect(t, parties.firstToken)
	h.at(t, authStart.Add(11*time.Minute))
	if err := h.gateway.ExtendPresence(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := other.gateway.ExtendPresence(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := first.conn.CloseNow(); err != nil {
		t.Fatal(err)
	}
	h.awaitRegistry(t, parties.first, 0)

	h.tick(t)

	if len(h.digestsFor(t, firstPresenceEmail)) != 0 || len(h.digestsFor(t, secondPresenceEmail)) != 1 {
		t.Fatal("digest used instance-local presence")
	}
	if err := last.conn.CloseNow(); err != nil {
		t.Fatal(err)
	}
	other.awaitRegistry(t, parties.first, 0)
}

func TestQueuedProvisionalDigestsDoNotCrossReclaimDecisions(t *testing.T) {
	for _, choice := range []string{"suspended", "kept", "dropped"} {
		t.Run(choice, func(t *testing.T) {
			t.Parallel()
			h := newFlowHarness(t)
			_, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
			card, _ := h.squat(t, tutor, newcomerEmail, newcomerEmail)
			h.tick(t)
			confirmation := latestToken(t, h.mailTo(t, newcomerEmail), confirmSubject)
			channel := h.directChannel(t, card.ID).ID
			h.sent(t, tutor, channel, "Provisional message before reclaim")
			h.at(t, authStart.Add(11*time.Minute))
			h.queueDigests(t)
			queued := h.outboxOf(t, domain.EventMessagesDigest, "pending")

			confirmed := h.verify(t, fmt.Sprintf(`{"token":%q,"new_password":%q}`, confirmation, ownerPassword), "")
			h.mustAnswer(t, confirmed, http.StatusOK)
			owner := accessToken(t, confirmed)
			if choice == "kept" {
				h.mustAnswer(t, h.decideReclaim(t, owner, reclaimChoice{KeepLinks: []string{card.ID}}), http.StatusOK)
			}
			if choice == "dropped" {
				h.mustAnswer(t, h.decideReclaim(t, owner, reclaimChoice{DropLinks: []string{card.ID}}), http.StatusOK)
			}
			h.tick(t)

			if queued != 1 || len(h.digestsFor(t, newcomerEmail)) != 0 || h.outboxOf(t, domain.EventMessagesDigest, "done:obsolete") != 1 {
				t.Fatal("queued provisional digest crossed the reclaim boundary")
			}
		})
	}
}

func TestKeptReclaimStillDigestsNewMessages(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	_, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card, _ := h.squat(t, tutor, newcomerEmail, newcomerEmail)
	h.tick(t)
	confirmation := latestToken(t, h.mailTo(t, newcomerEmail), confirmSubject)
	channel := h.directChannel(t, card.ID).ID
	h.sent(t, tutor, channel, "Old provisional text")
	h.at(t, authStart.Add(11*time.Minute))
	h.queueDigests(t)
	confirmed := h.verify(t, fmt.Sprintf(`{"token":%q,"new_password":%q}`, confirmation, ownerPassword), "")
	h.mustAnswer(t, confirmed, http.StatusOK)
	h.mustAnswer(t, h.decideReclaim(t, accessToken(t, confirmed), reclaimChoice{KeepLinks: []string{card.ID}}), http.StatusOK)
	h.tick(t)
	h.at(t, authStart.Add(41*time.Minute))
	tutor = h.relogin(t, tutorEmail)
	h.sent(t, tutor, channel, "New message after reclaim")

	h.tickAt(t, authStart.Add(52*time.Minute))

	messages := h.digestsFor(t, newcomerEmail)
	if len(messages) != 1 || !strings.Contains(messages[0].Text, ": 1") || strings.Contains(messages[0].Text, ": 2") {
		t.Fatalf("kept reclaim suppressed new messages or included history: %v", subjects(messages))
	}
}

func TestReclaimDoesNotDiscardTheTutorsQueuedDigest(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	_, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card, _ := h.squat(t, tutor, newcomerEmail, newcomerEmail)
	h.tick(t)
	confirmation := latestToken(t, h.mailTo(t, newcomerEmail), confirmSubject)
	channel := h.directChannel(t, card.ID).ID
	squatter := accessToken(t, h.login(t, newcomerEmail, newcomerPassword))
	h.sent(t, squatter, channel, "Provisional message to tutor")
	h.at(t, authStart.Add(11*time.Minute))
	h.queueDigests(t)
	confirmed := h.verify(t, fmt.Sprintf(`{"token":%q,"new_password":%q}`, confirmation, ownerPassword), "")
	h.mustAnswer(t, confirmed, http.StatusOK)
	h.mustAnswer(t, h.decideReclaim(t, accessToken(t, confirmed), reclaimChoice{KeepLinks: []string{card.ID}}), http.StatusOK)

	h.tick(t)

	messages := h.digestsFor(t, tutorEmail)
	if len(messages) != 1 || !strings.Contains(messages[0].Text, cardName+": 1") {
		t.Fatal("student reclaim removed the tutor's legitimate digest")
	}
}

func TestDigestQueueDiagnosticsContainOnlyCommittedCounts(t *testing.T) {
	t.Parallel()
	h, parties, _ := digestFixture(t)
	h.sent(t, parties.tutorToken, h.directChannel(t, parties.card.ID).ID, "Private direct message")

	h.tickAt(t, authStart.Add(11*time.Minute))

	if h.countLogs(`"msg":"digest.queued"`, `"level":"INFO"`, `"members":1`, `"channels":2`) != 1 {
		t.Fatalf("missing digest summary: %s", h.logs.String())
	}
	for _, record := range strings.Split(h.logs.String(), "\n") {
		if !strings.Contains(record, `"msg":"digest.queued"`) {
			continue
		}
		for _, forbidden := range []string{tutorEmail, studentEmail, groupName, cardName, "Private direct message", "recipient", "token", "channel_id", "user_id"} {
			if strings.Contains(record, forbidden) {
				t.Fatalf("digest summary leaks %q", forbidden)
			}
		}
	}
}
