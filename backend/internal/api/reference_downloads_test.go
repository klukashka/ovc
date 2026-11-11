package api_test

import (
	"net/http"
	"testing"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/services/files"
)

const insertReferencedLibraryFile = `WITH stored AS (
    INSERT INTO attachments (id, uploader_id, context, quota_tutor_id, storage_key, filename, media_type, size_bytes, sha256, state,
        width, height, created_at, updated_at)
    VALUES ($1::uuid, $2::uuid, 'library', $2::uuid, 'library/' || $1, 'diagram.png', 'image/png', 10, sha256('x'), 'bound', 4, 4, now(), now())
    RETURNING id
)
INSERT INTO attachment_references (attachment_id, ref_kind, ref_id, created_at) SELECT id, $3, $4::uuid, now() FROM stored`

func (h *authHarness) referencedLibraryFile(t *testing.T, id string, tutor domain.UserID, kind domain.ReferenceKind, item domain.EntityID) {
	t.Helper()
	if _, err := h.database.Pool.Exec(t.Context(), insertReferencedLibraryFile, id, tutor.String(), string(kind), item.String()); err != nil {
		t.Fatal(err)
	}
}

func TestDownloadTokenV2IsRefusedWithoutALiveReferenceOfItsKind(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.activeStudentInAGroup(t)
	channel := h.directChannel(t, parties.card.ID).ID
	chatFile := h.boundUpload(t, parties.studentToken, channel, "essay.txt", plainText(64), 31)
	otherLesson, lesson := domain.EntityID{0x74, 0x65, 1}, domain.EntityID{0x6c, 0x65, 1}
	otherLessonFile, lessonFile := fixtureID(24, 1), fixtureID(24, 2)
	h.referencedLibraryFile(t, otherLessonFile, parties.tutor, domain.ReferenceLesson, otherLesson)
	h.referencedLibraryFile(t, lessonFile, parties.tutor, domain.ReferenceLesson, lesson)
	otherLessonID, valid := domain.ParseUUID[domain.AttachmentID](otherLessonFile)
	if !valid {
		t.Fatal("the other lesson file's identifier does not parse")
	}
	lessonID, valid := domain.ParseUUID[domain.AttachmentID](lessonFile)
	if !valid {
		t.Fatal("the lesson file's identifier does not parse")
	}
	versionTwo, err := h.files.ReferenceAddress(otherLessonID, files.DownloadReference{Kind: domain.ReferenceLesson, Item: lesson},
		domain.DispositionInline, h.now(t))
	if err != nil {
		t.Fatal(err)
	}
	versionOneForLesson := h.files.MintAddress(lessonID, lesson, domain.DispositionAttachment, h.now(t))

	servedVersionTwo := h.fetch(t, versionTwo.URL, nil)
	servedVersionOneForLesson := h.fetch(t, versionOneForLesson.URL, nil)
	forged := h.fetch(t, "/api/v1/files/"+lessonFile+"?t=forged", nil)
	chatAddress, chatBody := h.addressOf(t, parties.studentToken, chatFile, "")
	servedChat := h.fetch(t, chatBody.URL, nil)
	chatWithReference, _ := h.addressOf(t, parties.studentToken, chatFile, "?ref=lesson:"+otherLesson.String())
	chatWithMalformedReference, _ := h.addressOf(t, parties.studentToken, chatFile, "?ref=nonsense")
	otherLessonWithReference, _ := h.addressOf(t, parties.tutorToken, otherLessonFile, "?ref=lesson:"+otherLesson.String())
	otherLessonWithMalformedReference, _ := h.addressOf(t, parties.tutorToken, otherLessonFile, "?ref=nonsense")
	otherLessonWithoutReference, _ := h.addressOf(t, parties.tutorToken, otherLessonFile, "")

	for name, refused := range map[string]response{
		"a version 2 token naming a lesson without the file": servedVersionTwo,
		"a version 1 token for a lesson-referenced file":     servedVersionOneForLesson,
		"a forged token": forged,
	} {
		if refused.status != http.StatusNotFound || code(t, refused) != "not_found" {
			t.Fatalf("%s answered %s", name, describe(refused))
		}
	}
	if chatAddress.status != http.StatusOK || servedChat.status != http.StatusOK || servedChat.body != string(plainText(64)) {
		t.Fatalf("a version 1 token for a message answered %s and %s", describe(chatAddress), describe(servedChat))
	}
	for name, refused := range map[string]response{
		"a reference on a chat attachment":               chatWithReference,
		"a malformed reference on a chat attachment":     chatWithMalformedReference,
		"a reference to a lesson the caller cannot see":  otherLessonWithReference,
		"a malformed reference without a visible lesson": otherLessonWithMalformedReference,
		"a lesson file without a reference":              otherLessonWithoutReference,
	} {
		if refused.status != http.StatusNotFound || code(t, refused) != "not_found" {
			t.Fatalf("%s answered %s", name, describe(refused))
		}
	}
	if h.countLogs(`"msg":"download.refused"`, `"reason":"token"`) != 1 || h.countLogs(`"msg":"download.refused"`, `"reason":"unreferenced"`) != 2 {
		t.Fatal("the refused downloads were not recorded with their reasons")
	}
}

func TestDownloadRecordsCarryTheReferenceKindAndTheContext(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.activeStudentInAGroup(t)
	channel := h.directChannel(t, parties.card.ID).ID
	inlineFile := h.boundUpload(t, parties.studentToken, channel, "essay.txt", plainText(64), 33)
	missingFile := h.boundUpload(t, parties.studentToken, channel, "draft.txt", plainText(64), 34)
	inlineID, inlineValid := domain.ParseUUID[domain.AttachmentID](inlineFile)
	missingID, missingValid := domain.ParseUUID[domain.AttachmentID](missingFile)
	message, messageValid := domain.ParseUUID[domain.EntityID](messageID(33))
	if !inlineValid || !missingValid || !messageValid {
		t.Fatal("a fixture identifier does not parse")
	}
	for _, upload := range h.uploadsBy(t, parties.student) {
		if upload.ID == missingFile {
			if err := h.objects.Delete(t.Context(), upload.Key); err != nil {
				t.Fatal(err)
			}
		}
	}
	inlineText := h.files.MintAddress(inlineID, message, domain.DispositionInline, h.now(t))
	missing := h.files.MintAddress(missingID, domain.EntityID{0x4d, 0x53, 34}, domain.DispositionAttachment, h.now(t))
	unreferenced, err := h.files.ReferenceAddress(inlineID, files.DownloadReference{Kind: domain.ReferenceLesson, Item: domain.EntityID{0x6c, 0x65, 3}},
		domain.DispositionAttachment, h.now(t))
	if err != nil {
		t.Fatal(err)
	}

	refusedInline := h.fetch(t, inlineText.URL, nil)
	refusedMissing := h.fetch(t, missing.URL, nil)
	refusedUnreferenced := h.fetch(t, unreferenced.URL, nil)

	for name, refused := range map[string]response{
		"an inline token for a text file": refusedInline,
		"a file whose object is missing":  refusedMissing,
		"a lesson token without the file": refusedUnreferenced,
	} {
		if refused.status != http.StatusNotFound {
			t.Fatalf("%s answered %s", name, describe(refused))
		}
	}
	chat := []string{`"ref_kind":"message"`, `"context":"chat"`}
	if h.countLogs(append([]string{`"msg":"download.refused"`, `"reason":"token"`}, chat...)...) != 1 {
		t.Fatal("the inline refusal was not recorded with its reference kind and context")
	}
	if h.countLogs(append([]string{`"msg":"download.object_missing"`}, chat...)...) != 1 {
		t.Fatal("the missing object was not recorded with its reference kind and context")
	}
	if h.countLogs(`"msg":"download.refused"`, `"reason":"unreferenced"`, `"ref_kind":"lesson"`) != 1 {
		t.Fatal("the unreferenced refusal was not recorded with its reference kind")
	}
}
