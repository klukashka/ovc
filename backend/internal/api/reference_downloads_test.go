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

func TestDownloadTokenV2LayoutIsRefusedUntilItsKindIsAuthorized(t *testing.T) {
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
	versionTwo, err := h.files.ReferenceAddress(otherLessonID, files.DownloadReference{Kind: domain.ReferenceLesson, Item: otherLesson},
		domain.DispositionInline, h.now(t))
	if err != nil {
		t.Fatal(err)
	}
	versionOneForLesson := h.files.MintAddress(lessonID, lesson, domain.DispositionAttachment, h.now(t))

	servedVersionTwo := h.fetch(t, versionTwo.URL, nil)
	servedVersionOneForLesson := h.fetch(t, versionOneForLesson.URL, nil)
	chatAddress, chatBody := h.addressOf(t, parties.studentToken, chatFile, "")
	servedChat := h.fetch(t, chatBody.URL, nil)
	chatWithReference, _ := h.addressOf(t, parties.studentToken, chatFile, "?ref=lesson:"+otherLesson.String())
	chatWithMalformedReference, _ := h.addressOf(t, parties.studentToken, chatFile, "?ref=nonsense")
	otherLessonWithReference, _ := h.addressOf(t, parties.tutorToken, otherLessonFile, "?ref=lesson:"+otherLesson.String())
	otherLessonWithMalformedReference, _ := h.addressOf(t, parties.tutorToken, otherLessonFile, "?ref=nonsense")
	otherLessonWithoutReference, _ := h.addressOf(t, parties.tutorToken, otherLessonFile, "")

	for name, refused := range map[string]response{
		"a version 2 token of a kind without its authorization": servedVersionTwo,
		"a version 1 token for a lesson-referenced file":        servedVersionOneForLesson,
	} {
		if refused.status != http.StatusNotFound || code(t, refused) != "not_found" {
			t.Fatalf("%s answered %s", name, describe(refused))
		}
	}
	if chatAddress.status != http.StatusOK || servedChat.status != http.StatusOK || servedChat.body != string(plainText(64)) {
		t.Fatalf("a version 1 token for a message answered %s and %s", describe(chatAddress), describe(servedChat))
	}
	for name, refused := range map[string]response{
		"a reference on a chat attachment":                  chatWithReference,
		"a malformed reference on a chat attachment":        chatWithMalformedReference,
		"a reference to a lesson without its authorization": otherLessonWithReference,
		"a malformed reference to an unauthorized lesson":   otherLessonWithMalformedReference,
		"a lesson file without a reference":                 otherLessonWithoutReference,
	} {
		if refused.status != http.StatusNotFound || code(t, refused) != "not_found" {
			t.Fatalf("%s answered %s", name, describe(refused))
		}
	}
	if h.countLogs(`"msg":"download.refused"`, `"reason":"token"`) != 1 || h.countLogs(`"msg":"download.refused"`, `"reason":"unreferenced"`) != 1 {
		t.Fatal("the refused downloads were not recorded with their reasons")
	}
}
