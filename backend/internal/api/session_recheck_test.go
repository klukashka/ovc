package api_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/netip"
	"net/url"
	"sync"
	"testing"
	"time"

	"ovc/backend/internal/adapters/storage"
	"ovc/backend/internal/api"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/teststorage"
)

type squatterWrite struct {
	role domain.Role
	act  func(t *testing.T, h *authHarness, user domain.UserID, token string) (squatterRequest, unchangedCheck)
}

func authorised(token string) map[string]string {
	headers := signUpHeaders()
	headers["Authorization"] = "Bearer " + token
	return headers
}

func uploadForm(t *testing.T, filename string, data []byte) (string, string) {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	return body.String(), form.FormDataContentType()
}

func (h *authHarness) sendUpload(t *testing.T, token string) <-chan pendingAnswer {
	t.Helper()
	body, contentType := uploadForm(t, "me.png", pngBytes(t, 2, 2))
	return h.sendInBackground(http.MethodPost, "/api/v1/uploads?context=avatar", body, map[string]string{
		"Authorization": "Bearer " + token, "Content-Type": contentType,
	})
}

func (h *authHarness) storedName(t *testing.T, user domain.UserID) string {
	t.Helper()
	account, err := h.store.Repositories().Users.ByID(context.Background(), user)
	if err != nil {
		t.Fatal(err)
	}
	return account.Name
}

func (h *authHarness) hasAvatar(t *testing.T, user domain.UserID) bool {
	t.Helper()
	_, err := h.store.Repositories().Attachments.AvatarOf(context.Background(), user)
	if err != nil && !errors.Is(err, repositories.ErrNotFound) {
		t.Fatal(err)
	}
	return err == nil
}

func (h *authHarness) uploadsOf(t *testing.T, user domain.UserID, state domain.AttachmentState) int64 {
	t.Helper()
	return h.countRows(t, "SELECT count(id) FROM attachments WHERE uploader_id = $1::uuid AND state = '"+string(state)+"'", user.String())
}

type squatterRequest = func() <-chan pendingAnswer

type unchangedCheck = func(t *testing.T)

var squatterWrites = map[string]squatterWrite{
	"a signed-in acceptance": {role: domain.RoleStudent, act: func(t *testing.T, h *authHarness, user domain.UserID, token string) (squatterRequest, unchangedCheck) {
		t.Helper()
		_, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 9, secondTutorEmail)
		card := h.newCard(t, tutor, cardName, "")
		link := linkToken(t, card.Invitation)
		send := func() <-chan pendingAnswer { return h.trySignedInAcceptance(token, link) }
		unchanged := func(t *testing.T) {
			t.Helper()
			if h.storedRelationship(t, card.ID).State != "invited" || h.invitationPage(t, link).status != http.StatusOK || h.linksOf(t, user) != 0 {
				t.Fatal("the acceptance linked the card")
			}
		}
		return send, unchanged
	}},
	"a name edit": {role: domain.RoleStudent, act: func(t *testing.T, h *authHarness, user domain.UserID, token string) (squatterRequest, unchangedCheck) {
		t.Helper()
		send := func() <-chan pendingAnswer {
			return h.sendInBackground(http.MethodPatch, "/api/v1/users/me", `{"name":"A squatter's name"}`, authorised(token))
		}
		unchanged := func(t *testing.T) {
			t.Helper()
			if h.storedName(t, user) != "A language tutor" {
				t.Fatal("the squatter's name landed")
			}
		}
		return send, unchanged
	}},
	"an avatar binding": {role: domain.RoleStudent, act: func(t *testing.T, h *authHarness, user domain.UserID, token string) (squatterRequest, unchangedCheck) {
		t.Helper()
		avatar := uploadedID(t, h.upload(t, token, "me.png", pngBytes(t, 2, 2)))
		send := func() <-chan pendingAnswer {
			return h.sendInBackground(http.MethodPatch, "/api/v1/users/me", fmt.Sprintf(`{"avatar_id":%q}`, avatar), authorised(token))
		}
		unchanged := func(t *testing.T) {
			t.Helper()
			if h.hasAvatar(t, user) {
				t.Fatal("the squatter's avatar was bound")
			}
		}
		return send, unchanged
	}},
	"an avatar upload": {role: domain.RoleStudent, act: func(t *testing.T, h *authHarness, user domain.UserID, token string) (squatterRequest, unchangedCheck) {
		t.Helper()
		send := func() <-chan pendingAnswer { return h.sendUpload(t, token) }
		unchanged := func(t *testing.T) {
			t.Helper()
			if h.countRows(t, "SELECT count(id) FROM attachments WHERE uploader_id = $1::uuid", user.String()) != 0 {
				t.Fatal("the squatter's upload created an attachment")
			}
		}
		return send, unchanged
	}},
	"an unverified tutor's profile": {role: domain.RoleTutor, act: func(t *testing.T, h *authHarness, user domain.UserID, token string) (squatterRequest, unchangedCheck) {
		t.Helper()
		send := func() <-chan pendingAnswer {
			return h.sendInBackground(http.MethodPatch, "/api/v1/users/me", `{"bio":"A squatter's bio","subjects":["Spam"]}`, authorised(token))
		}
		unchanged := func(t *testing.T) {
			t.Helper()
			profile, err := h.store.Repositories().TutorProfiles.ByUser(context.Background(), user)
			if err != nil {
				t.Fatal(err)
			}
			if profile.Bio != "" || len(profile.Subjects) != 0 {
				t.Fatal("the squatter's tutor profile landed")
			}
		}
		return send, unchanged
	}},
	"a sign-out everywhere": {role: domain.RoleStudent, act: func(t *testing.T, h *authHarness, user domain.UserID, token string) (squatterRequest, unchangedCheck) {
		t.Helper()
		send := func() <-chan pendingAnswer {
			return h.sendInBackground(http.MethodPost, "/api/v1/auth/logout-all", "", authorised(token))
		}
		unchanged := func(t *testing.T) {
			t.Helper()
			if live := h.countRows(t, "SELECT count(id) FROM sessions WHERE user_id = $1::uuid AND revoked_at IS NULL", user.String()); live != 1 {
				t.Fatalf("the owner's session was revoked: %d live sessions", live)
			}
		}
		return send, unchanged
	}},
	"a code entry": {role: domain.RoleStudent, act: func(t *testing.T, h *authHarness, user domain.UserID, token string) (squatterRequest, unchangedCheck) {
		t.Helper()
		tutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 9, secondTutorEmail)
		code := h.storedCode(t, tutor).Display()
		send := func() <-chan pendingAnswer {
			return h.sendInBackground(http.MethodPost, joinRequestsPath, codeBody(code), authorised(token))
		}
		unchanged := func(t *testing.T) {
			t.Helper()
			if h.countRows(t, "SELECT count(id) FROM join_requests WHERE student_id = $1::uuid", user.String()) != 0 {
				t.Fatal("the squatter's code entry created a join request")
			}
		}
		return send, unchanged
	}},
	"an ending": {role: domain.RoleStudent, act: func(t *testing.T, h *authHarness, user domain.UserID, token string) (squatterRequest, unchangedCheck) {
		t.Helper()
		tutor, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 9, secondTutorEmail)
		card := h.seedCard(t, tutor, 1, domain.RelationshipProvisional, &user)
		send := func() <-chan pendingAnswer {
			return h.sendInBackground(http.MethodPost, tutorsPath+card+"/end", "", authorised(token))
		}
		unchanged := func(t *testing.T) {
			t.Helper()
			if state := h.storedRelationship(t, card).State; state != string(domain.RelationshipReclaimPending) || h.cardCount(t, tutor) != 1 {
				t.Fatalf("the squatter's ending changed the link to %s", state)
			}
		}
		return send, unchanged
	}},
	"a confirmation resend": {role: domain.RoleStudent, act: func(t *testing.T, h *authHarness, user domain.UserID, token string) (squatterRequest, unchangedCheck) {
		t.Helper()
		before := h.countRows(t, "SELECT count(id) FROM email_requests WHERE user_id = $1::uuid", user.String())
		send := func() <-chan pendingAnswer {
			return h.sendInBackground(http.MethodPost, "/api/v1/auth/verify-email/resend", "{}", authorised(token))
		}
		unchanged := func(t *testing.T) {
			t.Helper()
			if after := h.countRows(t, "SELECT count(id) FROM email_requests WHERE user_id = $1::uuid", user.String()); after != before {
				t.Fatalf("the squatter's resend queued %d requests", after-before)
			}
		}
		return send, unchanged
	}},
}

func TestRequestAuthenticatedBeforeReclaimCannotActAfterIt(t *testing.T) {
	t.Parallel()
	for name, write := range squatterWrites {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newFlowHarness(t)
			user, squatter := h.signedInAs(t, write.role, domain.AccountUnverified, 2, newcomerEmail)
			h.resend(t, newcomerEmail)
			h.tick(t)
			confirmation := latestToken(t, h.mailTo(t, newcomerEmail), confirmSubject)
			send, unchanged := write.act(t, h, user, squatter)
			release := h.holdUserRow(t, user)

			reclaim := h.sendInBackground(http.MethodPost, "/api/v1/auth/verify-email",
				fmt.Sprintf(`{"token":%q,"new_password":%q}`, confirmation, ownerPassword), signUpHeaders())
			h.waitForLockWaiters(t, 1)
			squatted := send()
			h.waitForLockWaiters(t, 2)
			release()
			reclaimed, refused := awaitAnswer(t, reclaim), awaitAnswer(t, squatted)

			if reclaimed.status != http.StatusOK {
				t.Fatalf("the reclaim answered %d %s", reclaimed.status, reclaimed.body)
			}
			if refused.status != http.StatusUnauthorized || code(t, refused) != "unauthenticated" {
				t.Fatalf("the squatter's request answered %d %s", refused.status, refused.body)
			}
			unchanged(t)
			if h.me(t, accessToken(t, reclaimed)).status != http.StatusOK {
				t.Fatal("the owner's session does not work after the squatter's request")
			}
		})
	}
	t.Run("the reclaim choice", func(t *testing.T) {
		t.Parallel()
		h := newFlowHarness(t)
		_, tutor := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
		card, student := h.squat(t, tutor, newcomerEmail, newcomerEmail)
		owner := h.reclaimed(t, newcomerEmail)
		reset := h.resetToken(t, newcomerEmail)
		release := h.holdUserRow(t, student)

		resetAnswer := h.sendReset(reset)
		h.waitForLockWaiters(t, 1)
		choice := h.sendInBackground(http.MethodPost, reclaimPath, reclaimChoice{KeepLinks: []string{card.ID}}.body(t), jsonCall(owner))
		h.waitForLockWaiters(t, 2)
		release()
		resetDone, chosen := awaitAnswer(t, resetAnswer), awaitAnswer(t, choice)

		if resetDone.status != http.StatusOK || outcomeOf(t, resetDone) != "" {
			t.Fatalf("the reset answered %d %s", resetDone.status, resetDone.body)
		}
		if chosen.status != http.StatusUnauthorized || code(t, chosen) != "unauthenticated" {
			t.Fatalf("the choice with a revoked session answered %d %s", chosen.status, chosen.body)
		}
		if h.storedRelationship(t, card.ID).State != "reclaim_pending" || !h.meReclaimPending(t, h.ownerSignIn(t, newcomerEmail)) {
			t.Fatal("the choice with a revoked session decided the link")
		}
	})
	t.Run("an avatar upload's settling", func(t *testing.T) {
		held := &heldWrites{}
		h := newAuthHarnessStoring(t, flowProxies(), api.NewHandler, held.wrap(t))
		user, squatter := h.signedInAs(t, domain.RoleStudent, domain.AccountUnverified, 2, newcomerEmail)
		h.resend(t, newcomerEmail)
		h.tick(t)
		confirmation := latestToken(t, h.mailTo(t, newcomerEmail), confirmSubject)
		held.holdNext()

		upload := h.sendUpload(t, squatter)
		held.awaitReached(t)
		reclaimed := h.verify(t, fmt.Sprintf(`{"token":%q,"new_password":%q}`, confirmation, ownerPassword), "")
		held.resume()
		refused := awaitAnswer(t, upload)

		if reclaimed.status != http.StatusOK {
			t.Fatalf("the reclaim answered %d %s", reclaimed.status, reclaimed.body)
		}
		if refused.status != http.StatusUnauthorized || code(t, refused) != "unauthenticated" {
			t.Fatalf("the settled upload answered %d %s", refused.status, refused.body)
		}
		if h.uploadsOf(t, user, domain.AttachmentPending) != 0 || h.uploadsOf(t, user, domain.AttachmentDeleted) != 1 || h.hasAvatar(t, user) {
			t.Fatal("the squatter's upload stays usable after the reclaim")
		}
		if queued := h.countRows(t, "SELECT count(id) FROM outbox WHERE kind = $1", string(domain.KindStorageDelete)); queued != 1 {
			t.Fatalf("%d object deletions were queued", queued)
		}
	})
}

type heldWrites struct {
	mutex   sync.Mutex
	armed   bool
	reached chan struct{}
	release chan struct{}
	once    sync.Once
}

func (w *heldWrites) holdNext() {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	w.armed = true
}

func (w *heldWrites) take() bool {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	taken := w.armed
	w.armed = false
	return taken
}

func (w *heldWrites) resume() {
	w.once.Do(func() { close(w.release) })
}

func (w *heldWrites) awaitReached(t *testing.T) {
	t.Helper()
	select {
	case <-w.reached:
	case <-time.After(pauseDeadline):
		t.Fatal("the upload never reached its object write")
	}
}

func (w *heldWrites) wrap(t *testing.T) func(*teststorage.Storage) storage.Store {
	t.Helper()
	w.reached, w.release = make(chan struct{}), make(chan struct{})
	return func(local *teststorage.Storage) storage.Store {
		target, err := url.Parse(local.Settings.Endpoint)
		if err != nil {
			t.Fatal(err)
		}
		forward := httputil.NewSingleHostReverseProxy(target)
		proxy := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.Method == http.MethodPut && w.take() {
				close(w.reached)
				<-w.release
			}
			forward.ServeHTTP(writer, request)
		}))
		t.Cleanup(proxy.Close)
		t.Cleanup(w.resume)
		settings := local.Settings
		settings.Endpoint = proxy.URL
		return storage.NewS3(settings)
	}
}

func flowProxies() []netip.Prefix {
	return []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}
}
