package api_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

const (
	formerStudent       = "former_student"
	removedStudent      = "removed_student"
	purgedActiveName    = "Anna from the morning group"
	purgedActiveNotes   = "Struggles with the past perfect"
	purgedEndedName     = "Anna who stopped in spring"
	reclaimedCardNotes  = "Met through the reclaimed link"
	thirdTutorEmail     = "third-tutor@example.org"
	tutorOwnMessage     = "See you on Thursday"
	studentOwnMessage   = "Thank you for the lesson"
	purgedChannelSearch = "?q=Anna"
)

type purgedStudentWorld struct {
	activeCard    string
	endedCard     string
	reclaimedCard string
	reclaimTutor  domain.UserID
	lesson        string
	issue         string
	channel       string
}

func (h *authHarness) purgedStudentWorld(t *testing.T) purgedStudentWorld {
	t.Helper()
	_, tokenA := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	_, tokenB := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 3, secondTutorEmail)
	tutorD, tokenD := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 4, thirdTutorEmail)
	reclaimed, _ := h.squat(t, tokenD, newcomerEmail, newcomerEmail)
	h.withNotes(t, tokenD, reclaimed.ID, reclaimedCardNotes)
	owner := h.reclaimed(t, newcomerEmail)
	active := h.linkedSignedIn(t, tokenA, owner, purgedActiveName, newcomerEmail)
	ended := h.linkedSignedIn(t, tokenB, owner, purgedEndedName, newcomerEmail)
	h.withNotes(t, tokenA, active.ID, purgedActiveNotes)
	world := purgedStudentWorld{activeCard: active.ID, endedCard: ended.ID, reclaimedCard: reclaimed.ID, reclaimTutor: tutorD}
	world.lesson = h.lessonWith(t, tokenA, active.ID)
	world.issue, _ = h.issueWork(t, tokenA, toCard(active.ID), "Trip to Paris", string(domain.AnswerPolicyNever), nil, workBlocks()...)
	var channel cardChannel
	if err := json.Unmarshal([]byte(h.cardCall(t, tokenA, http.MethodGet, active.ID, "", "").body), &channel); err != nil || channel.ChannelID == nil {
		t.Fatal("the active card has no direct channel")
	}
	world.channel = *channel.ChannelID
	h.mustAnswer(t, h.sendMessage(t, tokenA, world.channel, tutorOwnMessage, nil, ""), http.StatusCreated)
	h.mustAnswer(t, h.sendMessage(t, owner, world.channel, studentOwnMessage, nil, ""), http.StatusCreated)
	h.mustAnswer(t, h.endAsTutor(t, tokenB, ended.ID), http.StatusOK)
	h.mustAnswer(t, h.deleteMe(t, owner, ownerPassword), http.StatusNoContent)
	return world
}

type purgedTutorView struct {
	lesson        response
	issues        issuePageBody
	issue         issueDetailBody
	channels      response
	channelPage   channelPage
	searched      response
	searchPage    channelPage
	channel       response
	channelDetail channelItem
	messages      []chatMessage
	notifications response
	issuesRaw     response
	issueRaw      response
	home          response
}

type purgedHomeChannel struct {
	Label     *string `json:"label"`
	LabelKind *string `json:"label_kind"`
}

type purgedHome struct {
	UnreadChannels struct {
		Items []purgedHomeChannel `json:"items"`
	} `json:"unread_channels"`
}

func (h *authHarness) purgedTutorView(t *testing.T, token string, world purgedStudentWorld) purgedTutorView {
	t.Helper()
	view := purgedTutorView{
		lesson:        h.send(t, http.MethodGet, lessonsRoute+"/"+world.lesson, "", jsonCall(token)),
		issues:        h.issuesPage(t, token, url.Values{}),
		issue:         h.issueDetail(t, token, world.issue),
		notifications: h.send(t, http.MethodGet, notificationsPath+"?limit=100", "", jsonCall(token)),
		messages:      h.messagesOf(t, token, world.channel),
		issuesRaw:     h.issuesAnswer(t, token, url.Values{}),
		issueRaw:      h.issueDetailAnswer(t, token, world.issue),
		home:          h.homeAnswer(t, token, ""),
	}
	view.channels, view.channelPage = h.channelList(t, token, "")
	view.searched, view.searchPage = h.channelList(t, token, purgedChannelSearch)
	view.channel, view.channelDetail = h.channelCall(t, token, world.channel)
	return view
}

func assertFormerStudentLabel(t *testing.T, surface string, label *string, kind *string) {
	t.Helper()
	if label != nil || kind == nil || *kind != formerStudent {
		t.Fatalf("%s shows label %v with kind %v instead of the former student label", surface, label, kind)
	}
}

func lessonAudienceOf(t *testing.T, answer response) tutorLessonAudience {
	t.Helper()
	var audience tutorLessonAudience
	if err := json.Unmarshal([]byte(answer.body), &audience); err != nil || answer.status != http.StatusOK {
		t.Fatalf("the lesson answered %d %s", answer.status, answer.body)
	}
	return audience
}

func messageBodies(messages []chatMessage) (own []string, others []*string) {
	for _, message := range messages {
		if message.Own && message.Body != nil {
			own = append(own, *message.Body)
		}
		if !message.Own {
			others = append(others, message.Body)
		}
	}
	return own, others
}

func TestStudentPurgeRelabelsEveryRelationshipRow(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	world := h.purgedStudentWorld(t)
	replacement := h.onlyNewCard(t, world.reclaimTutor)
	mailBeforePurge, mailBeforePurgeB := len(h.mailTo(t, tutorEmail)), len(h.mailTo(t, secondTutorEmail))

	h.tickAt(t, h.now(t).Add(domain.DeletedAccountRetention))
	tokenA, tokenB := h.relogin(t, tutorEmail), h.relogin(t, secondTutorEmail)
	view := h.purgedTutorView(t, tokenA, world)
	endedTutorNotifications := h.send(t, http.MethodGet, notificationsPath+"?limit=100", "", jsonCall(tokenB))
	var home purgedHome
	homeErr := json.Unmarshal([]byte(view.home.body), &home)

	if h.userExists(t, newcomerEmail) {
		t.Fatal("the student account was not purged")
	}
	want := map[string]string{world.activeCard: "student_purged", world.endedCard: "student_purged", world.reclaimedCard: "removed"}
	for id, reason := range want {
		row := h.purgedRow(t, id)
		if row.State != "discarded" || row.DiscardReason == nil || *row.DiscardReason != reason || row.StudentID != nil || row.Email != nil || row.DisplayName != nil || row.Notes != "" {
			t.Errorf("the purged student's row %s is %+v", id, row)
		}
	}
	if kept := h.storedRetention(t, replacement); kept.State != "invited" || kept.DisplayName == nil || *kept.DisplayName != cardName ||
		kept.Notes != reclaimedCardNotes || kept.Email == nil || *kept.Email != newcomerEmail {
		t.Fatalf("the reclaiming tutor's replacement card is %+v", kept)
	}
	audience := lessonAudienceOf(t, view.lesson).Audience
	assertFormerStudentLabel(t, "the lesson", audience.Label, audience.LabelKind)
	if len(view.issues.Items) != 1 {
		t.Fatalf("the tutor lists %d issues", len(view.issues.Items))
	}
	assertFormerStudentLabel(t, "the issue list", view.issues.Items[0].Audience.DisplayName, view.issues.Items[0].Audience.LabelKind)
	assertFormerStudentLabel(t, "the issue detail", view.issue.Audience.DisplayName, view.issue.Audience.LabelKind)
	if view.channels.status != http.StatusOK || len(view.channelPage.Items) != 1 || view.channel.status != http.StatusOK {
		t.Fatalf("the read-only channel answered list %d with %d items and detail %d", view.channels.status, len(view.channelPage.Items), view.channel.status)
	}
	assertFormerStudentLabel(t, "the channel list", view.channelPage.Items[0].Label, view.channelPage.Items[0].LabelKind)
	assertFormerStudentLabel(t, "the channel detail", view.channelDetail.Label, view.channelDetail.LabelKind)
	if view.searched.status != http.StatusOK || len(view.searchPage.Items) != 0 {
		t.Fatalf("a search for the erased name found %d channels", len(view.searchPage.Items))
	}
	if view.notifications.status != http.StatusOK || endedTutorNotifications.status != http.StatusOK || view.home.status != http.StatusOK || homeErr != nil {
		t.Fatalf("notifications answered %d and %d, the home answered %d", view.notifications.status, endedTutorNotifications.status, view.home.status)
	}
	if len(home.UnreadChannels.Items) != 1 {
		t.Fatalf("the home lists %d unread channels", len(home.UnreadChannels.Items))
	}
	assertFormerStudentLabel(t, "the home's unread channels", home.UnreadChannels.Items[0].Label, home.UnreadChannels.Items[0].LabelKind)
	own, others := messageBodies(view.messages)
	if len(own) != 1 || own[0] != tutorOwnMessage || len(others) != 1 || others[0] != nil {
		t.Fatalf("the channel keeps own messages %v and other bodies %v", own, others)
	}
	bodies := []string{
		view.lesson.body, view.channels.body, view.channel.body, view.notifications.body, endedTutorNotifications.body,
		view.issuesRaw.body, view.issueRaw.body, view.home.body,
	}
	for _, message := range append(h.mailTo(t, tutorEmail)[mailBeforePurge:], h.mailTo(t, secondTutorEmail)[mailBeforePurgeB:]...) {
		bodies = append(bodies, message.Subject, message.Text, message.HTML)
	}
	for _, body := range bodies {
		for _, private := range []string{studentAccountName, purgedActiveName, purgedEndedName, purgedActiveNotes, studentOwnMessage} {
			if strings.Contains(body, private) {
				t.Fatalf("a tutor-facing answer or email after the purge contains %q: %s", private, body)
			}
		}
	}
	assertNotLogged(t, h.logs.String(), purgedActiveName, purgedEndedName, purgedActiveNotes, reclaimedCardNotes, newcomerEmail)
}

func TestPurgeOfAProvisionalStudentKeepsTheRemovedLabel(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"seven day purge", "immediate purge"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			f := newRemovalFixture(t, path)
			h := f.h
			lesson := h.lessonWith(t, f.tutorToken, f.card)
			removedAt := h.now(t)

			if path == "immediate purge" {
				h.mustAnswer(t, h.deleteMe(t, f.studentToken, accountPassword), http.StatusNoContent)
			}
			h.tickAt(t, removedAt.Add(time.Hour))
			token := h.relogin(t, tutorEmail)
			audience := lessonAudienceOf(t, h.send(t, http.MethodGet, lessonsRoute+"/"+lesson, "", jsonCall(token))).Audience
			fresh := h.storedRetention(t, h.onlyNewCard(t, f.tutor))

			if h.countRows(t, "SELECT count(*) FROM users WHERE id = $1::uuid", f.student.String()) != 0 {
				t.Fatal("the provisional student was not purged")
			}
			if row := h.purgedRow(t, f.card); row.DisplayName != nil || row.Notes != "" || row.StudentID != nil || row.DiscardReason == nil || *row.DiscardReason != "removed" {
				t.Fatalf("the purged provisional row is %+v", row)
			}
			if fresh.DisplayName == nil || *fresh.DisplayName != removedPathCardName || fresh.Notes != removedPathNotes || fresh.Email == nil || *fresh.Email != removedPathEmail {
				t.Fatalf("the replacement card is %+v", fresh)
			}
			if audience.Label != nil || audience.LabelKind == nil || *audience.LabelKind != removedStudent {
				t.Fatalf("the removed link's lesson shows %v with kind %v", audience.Label, audience.LabelKind)
			}
		})
	}
}

func TestBlockedStudentIsNotPurged(t *testing.T) {
	t.Parallel()
	f := newRemovalFixture(t, "seven day purge")
	h := f.h
	if _, err := h.store.Repositories().Users.Block(t.Context(), f.student, h.now(t)); err != nil {
		t.Fatal(err)
	}

	h.tickAt(t, h.now(t).Add(domain.DeletedAccountRetention+24*time.Hour))

	if h.countRows(t, "SELECT count(*) FROM users WHERE id = $1::uuid", f.student.String()) != 1 {
		t.Fatal("the blocked student was purged")
	}
	if row := h.purgedRow(t, f.card); row.State != "provisional" || row.DisplayName == nil || *row.DisplayName != removedPathCardName || row.Notes != removedPathNotes {
		t.Fatalf("the blocked student's card is %+v", row)
	}
}
