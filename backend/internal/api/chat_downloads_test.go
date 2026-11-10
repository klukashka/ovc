package api_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

func anotherTutorsAttachment(t *testing.T, h *authHarness) (response, response) {
	t.Helper()
	_, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	intruder, intruderToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, "another-tutor@example.org")
	student, studentToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, studentEmail)
	card := h.linkedSignedIn(t, tutorToken, studentToken, cardName, studentEmail)
	h.shareStudentWith(t, intruder, student)
	attachment := fixtureID(8, 1)
	h.boundFixture(t, attachment, h.directChannel(t, card.ID).ID, messageID(1))
	outOfScope, _ := h.addressOf(t, intruderToken, attachment, "")
	random, _ := h.addressOf(t, intruderToken, fixtureID(8, 2), "")
	return outOfScope, random
}

func anotherAttachmentsAddress(t *testing.T, h *authHarness) (response, response) {
	t.Helper()
	_, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	_, studentToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 3, studentEmail)
	card := h.linkedSignedIn(t, tutorToken, studentToken, cardName, studentEmail)
	channel := h.directChannel(t, card.ID).ID
	minted, other := fixtureID(8, 3), fixtureID(8, 4)
	h.boundFixture(t, minted, channel, messageID(1))
	h.boundFixture(t, other, channel, messageID(2))
	_, address := h.addressOf(t, tutorToken, minted, "")
	outOfScope := h.fetch(t, strings.Replace(address.URL, minted, other, 1), nil)
	random := h.fetch(t, "/api/v1/files/"+fixtureID(8, 5)+"?t=x", nil)
	return outOfScope, random
}

func TestDownloadsAreAuthorizedThroughReferences(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	world := h.sharedChatWorld(t)
	parties := world.parties
	_, otherToken := h.signedInAs(t, domain.RoleStudent, domain.AccountActive, 6, requestingEmail)
	otherCard := h.linkedSignedIn(t, parties.tutorToken, otherToken, "Another card", requestingEmail)
	h.mustAnswer(t, h.groupCall(t, parties.tutorToken, http.MethodPatch, parties.group.ID, "", fmt.Sprintf(`{"add":[%q]}`, otherCard.ID)), http.StatusOK)
	document := h.boundUpload(t, parties.studentToken, world.direct.ID, "essay.txt", plainText(256), 1)
	photo := h.boundUpload(t, parties.tutorToken, world.group.ID, "board.jpg", samplePicture(t, 40, 30), 2)
	pending := h.chatUpload(t, parties.studentToken, world.direct.ID, "draft.txt", plainText(64))
	h.mustAnswer(t, pending, http.StatusCreated)
	h.mustAnswer(t, h.groupCall(t, parties.tutorToken, http.MethodPatch, parties.group.ID, "", fmt.Sprintf(`{"remove":[%q]}`, otherCard.ID)), http.StatusOK)
	random, _ := h.addressOf(t, world.secondToken, fixtureID(7, 1), "")

	byStudent, _ := h.addressOf(t, parties.studentToken, document, "")
	byTutor, _ := h.addressOf(t, parties.tutorToken, document, "")
	bySecondTutor, _ := h.addressOf(t, world.secondToken, document, "")
	removedMember, _ := h.addressOf(t, otherToken, photo, "")
	ownPending, _ := h.addressOf(t, parties.studentToken, uploadedID(t, pending), "")
	inlinePicture, _ := h.addressOf(t, parties.studentToken, photo, "?disposition=inline")
	inlineDocument, _ := h.addressOf(t, parties.studentToken, document, "?disposition=inline")
	repeated, _ := h.addressOf(t, parties.studentToken, photo, "?disposition=inline&disposition=inline")
	unknown, _ := h.addressOf(t, parties.studentToken, photo, "?disposition=preview")
	outOfScopeAndInvalid, _ := h.addressOf(t, world.secondToken, document, "?disposition=preview")
	h.mustAnswer(t, h.endAsTutor(t, parties.tutorToken, parties.card.ID), http.StatusOK)
	afterEnding, _ := h.addressOf(t, parties.studentToken, document, "")

	for name, result := range map[string]response{"the uploader": byStudent, "the tutor": byTutor, "an inline picture": inlinePicture,
		"the student of an ended relationship": afterEnding} {
		if result.status != http.StatusOK || result.headers.Get("Cache-Control") != "no-store" {
			t.Fatalf("%s got %s", name, describe(result))
		}
	}
	for name, result := range map[string]response{"a second tutor sharing the student": bySecondTutor, "a removed group member": removedMember,
		"the uploader of an unbound upload": ownPending, "an invalid disposition outside the scope": outOfScopeAndInvalid} {
		if result.status != http.StatusNotFound || refusal(t, result) != refusal(t, random) {
			t.Fatalf("%s got %s", name, describe(result))
		}
	}
	for name, result := range map[string]response{"inline for a document": inlineDocument, "a repeated disposition": repeated,
		"an unknown disposition": unknown} {
		if result.status != http.StatusUnprocessableEntity || fieldList(t, result) != "query.disposition:invalid" {
			t.Fatalf("%s got %s", name, describe(result))
		}
	}
}

func TestDownloadAddressesAreStableWithinTheirWindowAndStopWhenUnreferenced(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID).ID
	content := plainText(300)
	document := h.boundUpload(t, parties.studentToken, direct, "essay.txt", content, 1)
	other := h.boundUpload(t, parties.studentToken, direct, "other.txt", plainText(20), 2)
	deleted := h.boundUpload(t, parties.studentToken, direct, "deleted.txt", plainText(30), 3)
	h.at(t, authStart.Add(90*time.Second))

	_, first := h.addressOf(t, parties.studentToken, document, "")
	_, again := h.addressOf(t, parties.tutorToken, document, "")
	_, deletedAddress := h.addressOf(t, parties.studentToken, deleted, "")
	h.at(t, authStart.Add(5*time.Minute+30*time.Second))
	_, later := h.addressOf(t, parties.studentToken, document, "")
	stillValid := h.fetch(t, first.URL, nil)
	swapped := h.fetch(t, strings.Replace(first.URL, document, other, 1), nil)
	h.exec(t, "DELETE FROM attachment_references WHERE attachment_id = $1::uuid", deleted)
	unreferenced := h.fetch(t, deletedAddress.URL, nil)
	h.at(t, first.ExpiresAt)
	expired := h.fetch(t, first.URL, nil)
	_, fresh := h.addressOf(t, parties.studentToken, other, "")
	otherID, _ := domain.ParseUUID[domain.AttachmentID](other)
	otherMessage, _ := domain.ParseUUID[domain.EntityID](messageID(2))
	inlineDocument := h.fetch(t, h.files.MintAddress(otherID, otherMessage, domain.DispositionInline, first.ExpiresAt).URL, nil)
	h.exec(t, "UPDATE channels SET discarded_at = now(), erase_pending_since = now() WHERE id = $1::uuid", direct)
	erasing := h.fetch(t, fresh.URL, nil)
	random := h.fetch(t, "/api/v1/files/"+fixtureID(7, 2)+"?t=x", nil)

	if first.URL != again.URL || !first.ExpiresAt.Equal(authStart.Add(10*time.Minute)) || later.URL == first.URL ||
		!later.ExpiresAt.Equal(authStart.Add(15*time.Minute)) || !strings.HasPrefix(first.URL, "/api/v1/files/"+document+"?t=") {
		t.Fatalf("addresses %+v, %+v and %+v", first, again, later)
	}
	if stillValid.status != http.StatusOK || stillValid.body != string(content) {
		t.Fatalf("a valid address answered %d", stillValid.status)
	}
	for name, result := range map[string]response{"another attachment's path": swapped, "a deleted message": unreferenced,
		"a channel pending erasure": erasing, "an expired address": expired, "an inline address for a document": inlineDocument} {
		if result.status != http.StatusNotFound || refusal(t, result) != refusal(t, random) {
			t.Fatalf("%s answered %s", name, describe(result))
		}
	}
}

func TestDownloadResponsesCarryTheirHeadersAndIgnoreRanges(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID).ID
	content := []byte("Домашнее задание: страница 12.\n")
	document := h.boundUpload(t, parties.studentToken, direct, "задание 12.txt", content, 1)
	photo := h.boundUpload(t, parties.studentToken, direct, "board.jpg", pictureWithLocation(t), 2)
	_, documentAddress := h.addressOf(t, parties.tutorToken, document, "")
	_, photoAddress := h.addressOf(t, parties.tutorToken, photo, "?disposition=inline")

	text := h.fetch(t, documentAddress.URL, map[string]string{"Range": "bytes=0-3"})
	picture := h.fetch(t, photoAddress.URL, nil)

	expected := map[string]string{
		"Content-Type":                 "text/plain; charset=utf-8",
		"X-Content-Type-Options":       "nosniff",
		"Content-Disposition":          `attachment; filename="_______ 12.txt"; filename*=UTF-8''%D0%B7%D0%B0%D0%B4%D0%B0%D0%BD%D0%B8%D0%B5%2012.txt`,
		"Content-Security-Policy":      "default-src 'none'; sandbox; frame-ancestors 'none'",
		"Cross-Origin-Resource-Policy": "same-origin",
		"Referrer-Policy":              "no-referrer",
		"Cache-Control":                "private, no-store",
		"Content-Length":               fmt.Sprint(len(content)),
	}
	if text.status != http.StatusOK || text.body != string(content) {
		t.Fatalf("a ranged request answered %d with %d bytes", text.status, len(text.body))
	}
	for name, want := range expected {
		if got := text.headers.Get(name); got != want {
			t.Errorf("%s is %q, want %q", name, got, want)
		}
	}
	if text.headers.Get("Content-Range") != "" || text.headers.Get("Accept-Ranges") != "" {
		t.Fatalf("the download answered a range: %v", text.headers)
	}
	if picture.status != http.StatusOK || picture.headers.Get("Content-Type") != "image/jpeg" || picture.headers.Get("Content-Disposition") != "inline" {
		t.Fatalf("the picture answered %d with %v", picture.status, picture.headers)
	}
	if strings.Contains(picture.body, "GPS") || !bytes.HasPrefix([]byte(picture.body), []byte{0xFF, 0xD8}) {
		t.Fatal("the stored picture kept its location metadata")
	}
}

func TestPicturesGetTheirDimensionsAndPreviewAddresses(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID).ID

	result := h.chatUpload(t, parties.studentToken, direct, "board.jpeg", samplePicture(t, 64, 48))
	stored := h.onlyUploadBy(t, parties.student)

	if result.status != http.StatusCreated || stored.Width == nil || *stored.Width != 64 || stored.Height == nil || *stored.Height != 48 ||
		stored.MediaType == nil || *stored.MediaType != "image/jpeg" {
		t.Fatalf("the picture answered %s and is stored as %+v", describe(result), stored)
	}
}

func TestUploadAndDownloadRecordsCarryNoFileNamesOrAddresses(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID).ID
	const name = "distinctive-homework-title"
	document := h.boundUpload(t, parties.studentToken, direct, name+".txt", plainText(100), 1)
	_, address := h.addressOf(t, parties.tutorToken, document, "")
	refusedName := h.chatUpload(t, parties.studentToken, direct, name+"\u202e.txt", plainText(10))
	disguised := h.chatUpload(t, parties.studentToken, direct, name+".html", plainText(10))

	downloaded := h.fetch(t, address.URL, nil)
	forged := h.fetch(t, strings.TrimSuffix(address.URL, address.URL[len(address.URL)-4:])+"AAAA", nil)
	logs := h.logs.String()
	stored := h.uploadsBy(t, parties.student)

	if refusedName.status != http.StatusUnprocessableEntity || disguised.status != http.StatusUnsupportedMediaType {
		t.Fatalf("the refusals answered %s and %s", describe(refusedName), describe(disguised))
	}
	if downloaded.status != http.StatusOK || forged.status != http.StatusNotFound {
		t.Fatalf("the downloads answered %d and %d", downloaded.status, forged.status)
	}
	token := address.URL[strings.Index(address.URL, "?t=")+3:]
	for _, forbidden := range []string{name, token, address.URL, "?t="} {
		if strings.Contains(logs, forbidden) {
			t.Fatalf("a record carries %q", forbidden)
		}
	}
	for _, upload := range stored {
		if strings.Contains(logs, upload.Key) {
			t.Fatal("a record carries a storage key")
		}
	}
	for _, event := range []string{`"msg":"upload.accepted"`, `"reason":"filename"`, `"reason":"unsupported"`, `"msg":"download.refused"`,
		`"route":"GET /api/v1/files/{id}"`} {
		if !strings.Contains(logs, event) {
			t.Fatalf("no record carries %s", event)
		}
	}
}

func TestAMemberRemovedMidStreamLeavesNoObject(t *testing.T) {
	t.Parallel()
	t.Run("the second session refuses a member who left", func(t *testing.T) {
		t.Parallel()
		h := newUploadHarness(t)
		parties := h.activeStudentInAGroup(t)
		upload := h.startChatUpload(t, parties.studentToken, h.groupChannel(t, parties.group.ID).ID)
		upload.begin(t, "notes.txt", plainText(1<<10))
		started := h.awaitUploading(t, parties.student)

		h.mustAnswer(t, h.groupCall(t, parties.tutorToken, http.MethodPatch, parties.group.ID, "", fmt.Sprintf(`{"remove":[%q]}`, parties.card.ID)), http.StatusOK)
		result := upload.finish(t, plainText(1<<10))
		h.at(t, authStart.Add(domain.PendingUploadLifetime+time.Hour))
		h.tick(t)

		if result.status != http.StatusNotFound || h.uploadState(t, started.ID) != "gone" {
			t.Fatalf("the upload answered %s and its row is %s", describe(result), h.uploadState(t, started.ID))
		}
		h.assertLeftNothing(t, started)
	})
	t.Run("the age rule runs before the stored object reaches its second session", func(t *testing.T) {
		t.Parallel()
		h, held := newHeldStorageHarness(t)
		parties := h.activeStudentInAGroup(t)
		held.holdNext(rangedRead)
		upload := h.startChatUpload(t, parties.studentToken, h.groupChannel(t, parties.group.ID).ID)
		upload.begin(t, "essay.docx", wordDocument(t, ""))
		written := upload.completeInBackground(nil)
		held.awaitReached(t)
		started := h.onlyUploadBy(t, parties.student)

		h.mustAnswer(t, h.groupCall(t, parties.tutorToken, http.MethodPatch, parties.group.ID, "", fmt.Sprintf(`{"remove":[%q]}`, parties.card.ID)), http.StatusOK)
		h.at(t, authStart.Add(domain.UploadDeadlineCeiling+domain.UploadAgeAfterDeadline+time.Minute))
		expired, err := h.files.ExpirePending(context.Background(), authStart.Add(domain.UploadDeadlineCeiling+domain.UploadAgeAfterDeadline+time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		deletedByAge := h.uploadState(t, started.ID)
		_, storedWhileHeld := h.storedObject(t, started.Key)
		held.resume()
		result := upload.answer(t)
		h.tick(t)

		if err := <-written; err != nil {
			t.Fatal(err)
		}
		if expired != 1 || deletedByAge != string(domain.AttachmentDeleted) || !storedWhileHeld {
			t.Fatalf("the age rule removed %d rows, the row is %s and the object stored %t", expired, deletedByAge, storedWhileHeld)
		}
		if result.status != http.StatusNotFound || h.countRows(t, "SELECT count(id) FROM outbox WHERE idempotency_key = $1",
			string(domain.KindStorageDelete)+":"+started.ID+":intent:1") != 1 {
			t.Fatalf("the second session answered %s without queueing the completed object's deletion", describe(result))
		}
		h.assertLeftNothing(t, started)
		if h.countRows(t, "SELECT count(*) FROM upload_intents WHERE attachment_id = $1::uuid", started.ID) != 0 ||
			h.countRows(t, "SELECT used FROM upload_intent_capacity WHERE capacity_class = $1", "chat") != 0 {
			t.Fatal("confirmed final cleanup retained its intent or capacity reservation")
		}
	})
}

func TestADownloadNeedsALiveMessageOfItsOwnChannel(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.activeStudentInAGroup(t)
	direct := h.directChannel(t, parties.card.ID)
	group := h.groupChannel(t, parties.group.ID)
	elsewhere := h.sent(t, parties.tutorToken, group.ID, "A message of another channel")
	crossed := h.pendingUpload(t, parties.tutorToken, direct.ID, "crossed.txt", 16)
	h.exec(t, "UPDATE attachments SET state = 'bound', expires_at = NULL WHERE id = $1::uuid", crossed)
	h.exec2(t, "INSERT INTO attachment_references (attachment_id, ref_kind, ref_id, created_at) VALUES ($1::uuid, 'message', $2::uuid, now())",
		crossed, elsewhere.ID)
	stale := h.pendingUpload(t, parties.tutorToken, direct.ID, "stale.txt", 16)
	staleMessage := h.sent(t, parties.tutorToken, direct.ID, "Soon marked deleted", stale)
	live := h.pendingUpload(t, parties.tutorToken, direct.ID, "live.txt", 16)
	liveMessage := h.sent(t, parties.tutorToken, direct.ID, "Still here", live)
	crossedMessage, valid := domain.ParseUUID[domain.MessageID](elsewhere.ID)
	crossedID, crossedValid := domain.ParseUUID[domain.AttachmentID](crossed)
	if !valid || !crossedValid {
		t.Fatal("the fixture identifiers do not parse")
	}
	crossedURL, _ := h.files.ChatAddress(crossedID, crossedMessage, domain.DispositionAttachment, h.now(t))
	h.exec(t, "UPDATE messages SET deleted_at = now(), body = NULL WHERE id = $1::uuid", staleMessage.ID)

	crossedAddress, _ := h.addressOf(t, parties.tutorToken, crossed, "")
	crossedFile := h.fetch(t, crossedURL, nil)
	staleAddress, _ := h.addressOf(t, parties.tutorToken, stale, "")
	staleFile := h.fetch(t, staleMessage.Attachments[0].URL, nil)
	liveAddress, _ := h.addressOf(t, parties.tutorToken, live, "")
	liveFile := h.fetch(t, liveMessage.Attachments[0].URL, nil)

	for name, refused := range map[string]response{
		"an address for a message of another channel": crossedAddress, "a file named with a message of another channel": crossedFile,
		"an address for a deleted message": staleAddress, "a file of a deleted message": staleFile,
	} {
		if refused.status != http.StatusNotFound {
			t.Fatalf("%s answered %s", name, describe(refused))
		}
	}
	h.mustAnswer(t, liveAddress, http.StatusOK)
	h.mustAnswer(t, liveFile, http.StatusOK)
}
