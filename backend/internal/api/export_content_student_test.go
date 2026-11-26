package api_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/schemas"
	"ovc/backend/internal/services/export"
)

const (
	studentTutorsRoute      = "/api/v1/tutors"
	studentAssignmentsRoute = "/api/v1/assignments"
	notAvailableText        = "not available"
	removedMessageText      = "Message removed"
	sentinelNotes           = "Sentinel tutor notes"
	exportMessagesSheet     = "Messages"
)

func incompressiblePDF(size int, seed byte) []byte {
	data := append([]byte("%PDF-1.7\n"), make([]byte, 0, size)...)
	block := sha256.Sum256([]byte{seed})
	for len(data) < size {
		data = append(data, block[:]...)
		block = sha256.Sum256(block[:])
	}
	return data[:size]
}

func attachmentOfEntry(path string) string {
	parts := strings.SplitN(path, "/", 3)
	if len(parts) < 3 || len(parts[2]) < 36 {
		return ""
	}
	return parts[2][:36]
}

func (a studentArchive) archivedAttachments() []string {
	var ids []string
	for _, entry := range a.data.Files {
		ids = append(ids, attachmentOfEntry(entry.Path))
	}
	return ids
}

func (a studentArchive) historyOf(channel string) (exportHistory, bool) {
	for _, history := range a.data.Messages {
		var identified struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(history.Channel, &identified) == nil && identified.ID == channel {
			return history, true
		}
	}
	return exportHistory{}, false
}

func (a studentArchive) messageRow(body string) []string {
	for _, row := range a.rows(exportMessagesSheet) {
		if len(row) > 3 && row[3] == body {
			return row
		}
	}
	return nil
}

func (h *authHarness) exportRecords(t *testing.T, message string) []map[string]json.RawMessage {
	t.Helper()
	var records []map[string]json.RawMessage
	for _, line := range strings.Split(h.logs.String(), "\n") {
		if !strings.Contains(line, `"msg":"`+message+`"`) {
			continue
		}
		records = append(records, decodedAs[map[string]json.RawMessage](t, json.RawMessage(line)))
	}
	return records
}

func shortTextQuestion(id string) templateBlockJSON {
	return templateBlockJSON{ID: id, Type: string(domain.BlockShortText), Prompt: text("A word"), AcceptedAnswers: textList("word"), Points: points(1)}
}

func (h *authHarness) discardChannel(t *testing.T, channel string) {
	t.Helper()
	h.exec2(t, "UPDATE channels SET discarded_at = $2::timestamptz, erase_pending_since = $2::timestamptz, updated_at = $2::timestamptz WHERE id = $1::uuid",
		channel, rfc3339(authStart))
}

func TestStudentExportScope(t *testing.T) {
	t.Parallel()
	t.Run("own scope", func(t *testing.T) {
		t.Parallel()
		h := newUploadHarness(t)
		world := h.richStudentWorld(t)
		h.withNotes(t, world.tutor.token, world.firstCard.ID, sentinelNotes)
		erased := h.workTemplate(t, world.tutor.token, "Erased work", instructionBlock(blockID(89, 1), "Read"))
		erasedIssue, _ := h.issueTemplate(t, world.tutor.token, erased, toCard(world.firstCard.ID), studentExportPolicy, nil)
		h.markIssueForErasure(t, erasedIssue)
		h.reviewSaved(t, world.tutor.token, world.submission, reviewJSON(t, 2, 0, saveID(3), "Later feedback",
			commented(scored(blockID(exportWorkGroup, exportFileQuestion), 3), "Second look", world.corrected)))
		h.published(t, world.tutor.token, world.submission, domain.ReviewCheck, 2)
		removed := h.sent(t, world.first.token, world.groupChat, "Soon removed")
		h.mustAnswer(t, h.deleteMessage(t, world.first.token, removed.ID), http.StatusNoContent)
		h.mustAnswer(t, h.groupCall(t, world.tutor.token, http.MethodPatch, world.group.ID, "", fmt.Sprintf(`{"remove":[%q]}`, world.secondCard.ID)), http.StatusOK)

		_, first := h.studentArchiveOf(t, world.first)
		_, mate := h.studentArchiveOf(t, h.signedInAgain(t, world.second))

		for _, leaked := range []string{"First student", "Second student", sentinelNotes, secondExportEmail, "Erased work", "Overall good", "Soon removed"} {
			if first.holdsText(leaked) {
				t.Errorf("the student's export holds %q", leaked)
			}
		}
		if !first.holdsText("Later feedback") || first.messageRow(removedMessageText) == nil {
			t.Error("the student's export lacks the latest published feedback or the placeholder of a deleted message")
		}
		if _, found := mate.historyOf(world.groupChat); found {
			t.Error("a removed member's export holds the group chat")
		}
	})
	t.Run("former member, removed channels and an erased body", func(t *testing.T) {
		t.Parallel()
		h := newUploadHarness(t)
		world := h.exportWorld(t)
		groupLesson := h.scheduled(t, world.tutor.token, groupLessonFor(world.group.ID, "2030-03-13", "11:00")).Lessons[0].ID
		h.cancelWithRoster(t, groupLesson, world.first.id)
		h.mustAnswer(t, h.groupCall(t, world.tutor.token, http.MethodPatch, world.group.ID, "", fmt.Sprintf(`{"remove":[%q]}`, world.firstCard.ID)), http.StatusOK)
		erasedMessage := h.sent(t, world.tutor.token, h.directChannel(t, world.firstCard.ID).ID, "A body erased later")
		h.exec2(t, "UPDATE messages SET body = NULL, erased_at = $2::timestamptz, sender_id = NULL, client_key = NULL, updated_at = $2::timestamptz WHERE id = $1::uuid",
			erasedMessage.ID, rfc3339(authStart))
		discardedCard := h.linkedSignedIn(t, world.outsider.token, world.first.token, "Outsider card", firstExportEmail)
		discarded := h.directChannel(t, discardedCard.ID).ID
		h.sent(t, world.outsider.token, discarded, "A discarded chat message")
		h.setRelationshipState(t, discardedCard.ID, domain.RelationshipDiscarded)
		h.discardChannel(t, discarded)
		eraser := h.exportAccountAs(t, domain.RoleTutor, 8, "eraser-tutor@example.org")
		erasedCard := h.linkedSignedIn(t, eraser.token, h.signedInAgain(t, world.first).token, "Eraser card", firstExportEmail)
		pending := h.directChannel(t, erasedCard.ID).ID
		h.sent(t, eraser.token, pending, "An erase-pending chat message")
		h.setRelationshipState(t, erasedCard.ID, domain.RelationshipDiscarded)
		h.discardChannel(t, pending)

		_, archive := h.studentArchiveOf(t, h.signedInAgain(t, world.first))

		var former *schemas.StudentExportLesson
		for _, raw := range archive.data.Lessons {
			lesson := decodedAs[schemas.StudentExportLesson](t, raw)
			if lesson.ID == groupLesson {
				former = &lesson
			}
		}
		if former == nil || former.Audience.Label != nil || former.Audience.LabelKind == nil || *former.Audience.LabelKind != domain.LabelGroup {
			t.Fatalf("the former member's group lesson is %+v", former)
		}
		if lessons := archive.rows("Lessons"); len(lessons) != 1 || len(lessons[0]) < 7 || lessons[0][6] != "Group lesson" || archive.holdsText(exportGroupName) {
			t.Errorf("the former member's lessons sheet is %q", lessons)
		}
		for _, channel := range []string{discarded, pending} {
			if _, found := archive.historyOf(channel); found {
				t.Errorf("the removed channel %s is in the export", channel)
			}
		}
		if archive.holdsText("A discarded chat message") || archive.holdsText("An erase-pending chat message") || archive.holdsText("A body erased later") {
			t.Error("the export holds a removed channel's message or an erased body")
		}
		if archive.messageRow(removedMessageText) == nil {
			t.Error("the erased message has no placeholder row")
		}
	})
	t.Run("a reclaimed link while pending and after it is kept", func(t *testing.T) {
		t.Parallel()
		h := newUploadHarness(t)
		world := h.exportWorld(t)
		card, student := h.squat(t, world.tutor.token, newcomerEmail, newcomerEmail)
		squatter := accessToken(t, h.login(t, newcomerEmail, newcomerPassword))
		h.setAccount(t, student, "Earlier account holder sentinel", domain.LanguageEnglish)
		h.scheduled(t, world.tutor.token, oneOffFor(card.ID, "2030-03-20", "10:00"))
		template := h.workTemplate(t, world.tutor.token, "Reclaimed work", fileBlock(blockID(84, 1), string(domain.AnswerFilePDF)),
			shortTextQuestion(blockID(84, 2)))
		_, submissions := h.issueTemplate(t, world.tutor.token, template, toCard(card.ID), studentExportPolicy, nil)
		h.saved(t, squatter, submissions[0].id, answersJSON(t, 1, saveID(1), typedAnswer(blockID(84, 2), "An earlier holder's answer")))
		direct := h.directChannel(t, card.ID).ID
		h.sent(t, squatter, direct, "An earlier holder's message")
		tutorFile := h.chatUpload(t, world.tutor.token, direct, "tutor-notes.txt", plainText(64))
		h.mustAnswer(t, tutorFile, http.StatusCreated)
		h.sent(t, world.tutor.token, direct, "A tutor's file", uploadedID(t, tutorFile))
		owner := exportAccount{id: student, token: h.reclaimed(t, newcomerEmail), email: newcomerEmail, password: ownerPassword}

		_, pendingArchive := h.studentArchiveOf(t, owner)
		pendingWindows := h.calendarIDs(t, owner.token, authStart, authStart.Add(60*24*time.Hour))
		h.mustAnswer(t, h.decideReclaim(t, owner.token, reclaimChoice{KeepLinks: []string{card.ID}}), http.StatusOK)
		h.setAccount(t, student, "Current account holder", domain.LanguageEnglish)
		h.at(t, h.now(t).Add(25*time.Hour))
		owner.token = h.ownerSignIn(t, newcomerEmail)
		_, keptArchive := h.studentArchiveOf(t, owner)
		routed := h.submissionAnswer(t, owner.token, submissions[0].id)

		if len(pendingArchive.data.Tutors) != 0 || len(pendingArchive.data.Lessons) != 0 || len(pendingArchive.data.Homework) != 0 ||
			len(pendingArchive.data.Submissions) != 0 || len(pendingArchive.data.Messages) != 0 || len(pendingArchive.data.Files) != 0 || len(pendingWindows) != 0 {
			t.Fatalf("the reclaim-pending owner's export holds the link's data: %d tutors, %d lessons, %d homework, %d chats, %d files",
				len(pendingArchive.data.Tutors), len(pendingArchive.data.Lessons), len(pendingArchive.data.Homework), len(pendingArchive.data.Messages),
				len(pendingArchive.data.Files))
		}
		if pendingArchive.holdsText("An earlier holder's") || keptArchive.holdsText("Earlier account holder sentinel") {
			t.Error("an export holds the earlier holder's data or name")
		}
		if len(keptArchive.data.Homework) != 1 || !decodedAs[schemas.StudentAssignmentItem](t, keptArchive.data.Homework[0]).IssuedBeforeReclaim {
			t.Errorf("the kept link's export holds %d homework items or none with the reclaim cue", len(keptArchive.data.Homework))
		}
		earlier := false
		for _, message := range keptArchive.messages(t) {
			if message.Body != nil && *message.Body == "An earlier holder's message" {
				earlier = message.SentBeforeReclaim
			}
		}
		if !earlier {
			t.Error("the earlier holder's message does not carry sent_before_reclaim")
		}
		h.mustAnswer(t, routed, http.StatusOK)
		assertSameItems(t, "kept submission", keptArchive.data.Submissions, []json.RawMessage{json.RawMessage(routed.body)})
	})
}

func TestStudentExportLabelsGroupChat(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	world := h.richStudentWorld(t)
	h.mustAnswer(t, h.deleteMe(t, world.second.token, world.second.password), http.StatusNoContent)

	_, archive := h.studentArchiveOf(t, h.signedInAgain(t, world.first))

	own, mate, tutor := archive.messageRow("My own message"), archive.messageRow("A mate's message"), archive.messageRow("The tutor's message")
	if own == nil || own[2] != "You" {
		t.Errorf("the student's own message row is %q", own)
	}
	if mate == nil || mate[2] != "Deleted user" {
		t.Errorf("a deleted mate's message row is %q", mate)
	}
	if tutor == nil || tutor[2] != "A language tutor" {
		t.Errorf("the tutor's message row is %q", tutor)
	}
	for _, message := range archive.messages(t) {
		for _, reaction := range message.Reactions {
			if reaction.Members != nil {
				t.Errorf("a reaction in the student's export carries members: %+v", reaction)
			}
		}
		if message.Body != nil && *message.Body == "A mate's message" && (message.SenderLabelKind == nil || *message.SenderLabelKind != domain.LabelDeletedUser) {
			t.Error("the data copy does not label the deleted mate as deleted_user")
		}
	}
}

func TestStudentExportFormats(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	world := h.exportWorld(t)
	direct := h.directChannel(t, world.firstCard.ID).ID
	h.sent(t, world.tutor.token, direct, "=1+2")
	h.sent(t, world.tutor.token, direct, "<b>&amp;</t>")
	description := "Line one\nATTACH:http://example.org/x"
	lesson := oneOffFor(world.firstCard.ID, "2030-03-12", "10:00")
	lesson.Title, lesson.Description = "Plan; ATTACH:x", &description
	h.scheduled(t, world.tutor.token, lesson)
	template := h.workTemplate(t, world.tutor.token, "Files", fileBlock(blockID(87, 1), string(domain.AnswerFilePDF)))
	_, submissions := h.issueTemplate(t, world.tutor.token, template, toCard(world.firstCard.ID), studentExportPolicy, nil)
	first := h.answerFile(t, world.first.token, submissions[0].id, blockID(87, 1), "../x.pdf", 64)
	second := h.answerFile(t, world.first.token, submissions[0].id, blockID(87, 1), "../x.pdf", 65)
	h.saved(t, world.first.token, submissions[0].id, answersJSON(t, 1, saveID(1), attached(blockID(87, 1), first, second)))

	_, archive := h.studentArchiveOf(t, h.signedInAgain(t, world.first))

	formula := archive.messageRow("=1+2")
	markup := archive.messageRow("<b>&amp;</t>")
	if formula == nil || markup == nil {
		t.Fatalf("the messages sheet lost formula-like or markup text: %q %q", formula, markup)
	}
	for name, rows := range archive.sheets {
		for _, row := range rows {
			for _, cell := range row {
				if cell.Formula != nil {
					t.Errorf("the sheet %s holds a formula", name)
				}
			}
		}
	}
	for part := range archive.parts {
		if strings.Contains(part, "hyperlink") || strings.Contains(part, "externalLink") {
			t.Errorf("the workbook holds the part %s", part)
		}
	}
	for _, line := range strings.Split(archive.calendar, "\r\n") {
		if strings.HasPrefix(line, "ATTACH") {
			t.Errorf("the calendar holds the property line %q", line)
		}
	}
	if !strings.Contains(archive.calendar, "SUMMARY:Plan\\; ATTACH:x") {
		t.Errorf("the lesson title did not stay inside SUMMARY:\n%s", archive.calendar)
	}
	for _, id := range []string{first, second} {
		if archive.entries["files/answers/"+id+"-x.pdf"] == nil {
			t.Errorf("the answer file %s is not stored under its safe name: %v", id, archive.entryNames())
		}
	}
}

func TestStudentExportHoldsNoSignedAddress(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	world := h.richStudentWorld(t)

	_, archive := h.studentArchiveOf(t, world.first)

	for _, forbidden := range []string{"X-Amz-Signature", "X-Amz-Credential", "/api/v1/files/", "?t=", `"url":`, `"expires_at":`} {
		if archive.holdsText(forbidden) {
			t.Errorf("the student's export holds %q", forbidden)
		}
	}
	if len(archive.data.Files) == 0 {
		t.Fatal("the rich world exported no files")
	}
}

func (h *authHarness) attachToLesson(t *testing.T, token string, lesson string, material string) {
	t.Helper()
	h.mustAnswer(t, h.send(t, http.MethodPost, lessonsRoute+"/"+lesson+"/materials", fmt.Sprintf(`{"material_ids":[%q],"version":1}`, material),
		jsonCall(token)), http.StatusOK)
}

func TestExportStoresEachFileOnceAndLeavesOutUnreadableOnes(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	world := h.richStudentWorld(t)
	h.attachToLesson(t, world.tutor.token, world.lesson, world.material)
	if err := h.objects.Delete(context.Background(), h.storageKeyOf(t, world.chatFile)); err != nil {
		t.Fatal(err)
	}

	row, archive := h.studentArchiveOf(t, world.first)

	var shared []schemas.StudentExportFileEntry
	for _, entry := range archive.data.Files {
		if attachmentOfEntry(entry.Path) == world.tutorFile {
			shared = append(shared, entry)
		}
	}
	if len(shared) != 1 || len(shared[0].References) != 2 {
		t.Fatalf("the shared worksheet is stored as %+v", shared)
	}
	if slices.Contains(archive.archivedAttachments(), world.chatFile) {
		t.Error("an attachment whose object is gone is in the archive")
	}
	if mate := archive.messageRow("A mate's message"); mate == nil || !strings.Contains(mate[6], notAvailableText) {
		t.Errorf("the row of the unreadable file says %q", mate)
	}
	built := h.exportRecords(t, "export.built")
	unreadable := h.exportRecords(t, "export.file.unreadable")
	if len(built) != 1 || string(built[0]["files_unreadable"]) != "1" || string(built[0]["items_messages"]) == "" || string(built[0]["files"]) == "0" {
		t.Errorf("export.built carries %v", built)
	}
	if len(unreadable) != 1 || string(unreadable[0]["attachment_id"]) != `"`+world.chatFile+`"` || string(unreadable[0]["kind"]) != `"chat"` ||
		string(unreadable[0]["export_id"]) != `"`+row.ID.String()+`"` {
		t.Errorf("export.file.unreadable carries %v", unreadable)
	}
}

func TestExportBoundFollowsTheSelectedFiles(t *testing.T) {
	t.Parallel()
	t.Run("within the selected files plus the allowance", func(t *testing.T) {
		t.Parallel()
		h := newExportAllowanceHarness(t, 1<<20)
		world := h.exportWorld(t)
		direct := h.directChannel(t, world.firstCard.ID).ID
		for index := range 3 {
			upload := h.chatUpload(t, world.first.token, direct, fmt.Sprintf("part-%d.pdf", index), incompressiblePDF(400<<10, byte(index)))
			h.mustAnswer(t, upload, http.StatusCreated)
			h.sent(t, world.first.token, direct, "A part", uploadedID(t, upload))
		}

		ready := h.readyExportOf(t, h.signedInAgain(t, world.first))

		if ready.SizeBytes == nil || *ready.SizeBytes <= 1<<20 {
			t.Fatalf("the archive holds %v bytes, want more than the allowance", ready.SizeBytes)
		}
	})
	t.Run("over the selected files plus the allowance", func(t *testing.T) {
		t.Parallel()
		h := newExportAllowanceHarness(t, 1<<10)
		world := h.exportWorld(t)
		direct := h.directChannel(t, world.firstCard.ID).ID
		selected := h.chatUpload(t, world.tutor.token, direct, "selected.pdf", incompressiblePDF(64<<10, 9))
		h.mustAnswer(t, selected, http.StatusCreated)
		h.sent(t, world.tutor.token, direct, "A message with a selected file", uploadedID(t, selected))

		requested := h.requestExport(t, h.signedInAgain(t, world.first).token, world.first.password)
		h.tick(t)
		failed, _ := h.exportOf(t, world.first.id)

		if requested.status != http.StatusAccepted || failed.State != domain.ExportFailed || failed.Failure == nil || *failed.Failure != domain.ExportFailureTooLarge {
			t.Fatalf("an archive over its bound is %s", failed.State)
		}
	})
}

func TestExportDeleteFreesSlotWithinOnePart(t *testing.T) {
	t.Parallel()
	h, held := newHeldStorageHarness(t)
	world := h.exportWorld(t)
	direct := h.directChannel(t, world.firstCard.ID).ID
	large := h.chatUpload(t, world.first.token, direct, "large.pdf", incompressiblePDF(20<<20, 7))
	h.mustAnswer(t, large, http.StatusCreated)
	h.sent(t, world.first.token, direct, "A large file", uploadedID(t, large))
	seededParts, seededCompletes := held.multipartWrites()
	first := h.signedInAgain(t, world.first)
	h.mustAnswer(t, h.requestExport(t, first.token, first.password), http.StatusAccepted)
	held.holdNext(multipartPart)
	building := h.tickInBackground(t)
	held.awaitReached(t)

	deleted := h.deleteExport(t, first.token)
	other := h.signedInAgain(t, world.outsider)
	requested := h.requestExport(t, other.token, other.password)
	held.resume()
	awaitTick(t, building)

	parts, completes := held.multipartWrites()
	_, kept := h.exportOf(t, world.first.id)
	built, _ := h.exportOf(t, world.outsider.id)
	if deleted.status != http.StatusNoContent || requested.status != http.StatusAccepted || kept {
		t.Fatalf("the deletion answered %d, the other request %d, and the deleted export is kept %v", deleted.status, requested.status, kept)
	}
	if parts-seededParts != 2 || completes-seededCompletes != 1 {
		t.Fatalf("the exports sent %d part uploads and %d completions, want the held part and the other user's single part",
			parts-seededParts, completes-seededCompletes)
	}
	if built.State != domain.ExportReady {
		t.Fatalf("the other user's export is %s after the same tick", built.State)
	}
}

func TestABuildRecordsTheDeletedAccountsItReads(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	world := h.exportWorld(t)
	group := h.groupChannel(t, world.group.ID).ID
	h.sent(t, world.second.token, group, "Kept until the purge")
	h.mustAnswer(t, h.deleteMe(t, world.second.token, world.second.password), http.StatusNoContent)
	due := authStart.Add(domain.DeletedAccountRetention)
	h.at(t, due.Add(-24*time.Hour))
	row := h.readyExportOf(t, h.signedInAgain(t, world.first))

	h.at(t, due.Add(-time.Minute))
	early, _ := h.mintExport(t, h.signedInAgain(t, world.first).token)
	h.at(t, due)
	late, _ := h.mintExport(t, h.signedInAgain(t, world.first).token)
	h.tick(t)
	h.tick(t)

	if row.ExpiresAt == nil || !row.ExpiresAt.Equal(due) {
		t.Fatalf("the export expires at %v, want the mate's purge due time %v", row.ExpiresAt, due)
	}
	if early.status != http.StatusOK || late.status != http.StatusNotFound {
		t.Fatalf("minting answered %d a minute before the due time and %d at it", early.status, late.status)
	}
	expired, _ := h.exportOf(t, world.first.id)
	if expired.State != domain.ExportExpired || h.objectExists(t, *row.StorageKey) {
		t.Fatalf("at the due time the export is %s with its object kept %v", expired.State, h.objectExists(t, *row.StorageKey))
	}
}

func (h *authHarness) buildDirectly(t *testing.T, owner exportAccount) repositories.Export {
	t.Helper()
	h.mustAnswer(t, h.requestExport(t, owner.token, owner.password), http.StatusAccepted)
	build, err := h.exports.ClaimBuild(context.Background(), h.now(t), 0)
	if err != nil || build == nil {
		t.Fatalf("no export could be claimed: %v", err)
	}
	if err := build(context.Background()); err != nil {
		t.Fatal(err)
	}
	row, _ := h.exportOf(t, owner.id)
	return row
}

func TestABuildThatReadADeletedTutorKeepsItsClosingAfterThePurge(t *testing.T) {
	t.Parallel()
	h, held := newHeldStorageHarness(t)
	world := h.exportWorld(t)
	h.mustAnswer(t, h.deleteMe(t, world.tutor.token, world.tutor.password), http.StatusNoContent)
	closing := authStart.Add(domain.DeletedAccountRetention)
	h.at(t, closing.Add(-time.Hour))
	student := h.signedInAgain(t, world.first)
	build := h.holdBuildOf(t, held, student)

	h.at(t, closing)
	if _, err := h.purger.PurgeDeleted(context.Background(), closing); err != nil {
		t.Fatal(err)
	}
	purged := h.countRows(t, "SELECT count(id) FROM users WHERE id = $1::uuid", world.tutor.id.String())
	build.finish(t, h)

	withdrawn, _ := h.exportOf(t, world.first.id)
	records := h.invalidationsOf(t, build.row.ID)
	if purged != 0 {
		t.Fatal("the purge kept the deleted tutor")
	}
	if withdrawn.State != domain.ExportFailed || withdrawn.Failure == nil || *withdrawn.Failure != domain.ExportFailureWithdrawn {
		t.Fatalf("a build that read the deleted tutor is %s after the purge", withdrawn.State)
	}
	if h.exportLimitRows(t, world.first.id) != 0 || h.objectExists(t, *build.row.StorageKey) {
		t.Fatal("the withdrawn build kept a limit row or its object")
	}
	if len(records) != 1 || records[0].Cause != string(domain.ExportCausePurgeDue) {
		t.Fatalf("the withdrawal was recorded as %+v", records)
	}
}

func (h *authHarness) channelsOfDeletedTutorReadable(t *testing.T, token string) bool {
	t.Helper()
	result, page := h.channelList(t, token, "")
	h.mustAnswer(t, result, http.StatusOK)
	return len(page.Items) > 0
}

func TestAStudentsBuildAfterItsTutorsClosingIsWithdrawnWhileItsChatIsListed(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	world := h.exportWorld(t)
	h.mustAnswer(t, h.deleteMe(t, world.tutor.token, world.tutor.password), http.StatusNoContent)
	closing := authStart.Add(domain.DeletedAccountRetention)
	h.at(t, closing.Add(time.Minute))
	student := h.signedInAgain(t, world.first)
	chatReadable := h.channelsOfDeletedTutorReadable(t, student.token)

	row := h.buildDirectly(t, student)

	failed := h.exportNotifications(t, world.first.id, domain.NotificationExportFailed)
	t.Logf("the deleted tutor's chat readable after the closing: %t", chatReadable)
	if !chatReadable {
		t.Fatal("the deleted tutor's chat is no longer listed after the closing, so T-48's wording of this window no longer holds")
	}
	if row.State != domain.ExportFailed || row.Failure == nil || *row.Failure != domain.ExportFailureWithdrawn || failed != 1 {
		t.Fatalf("with the deleted tutor's chat still readable the build is %s with %d failure notices, want it withdrawn", row.State, failed)
	}
}

func TestADeletedTutorsLessonsBeforeThePurgeBoundTheExport(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	world := h.exportWorld(t)
	lesson := h.scheduled(t, world.tutor.token, oneOffFor(world.firstCard.ID, "2030-03-12", "10:00")).Lessons[0].ID
	h.cancelWithRoster(t, lesson, world.first.id)
	h.mustAnswer(t, h.groupCall(t, world.tutor.token, http.MethodPatch, world.group.ID, "", fmt.Sprintf(`{"remove":[%q]}`, world.firstCard.ID)), http.StatusOK)
	h.mustAnswer(t, h.deleteMe(t, world.tutor.token, world.tutor.password), http.StatusNoContent)
	h.discardChannel(t, h.directChannel(t, world.firstCard.ID).ID)
	closing := authStart.Add(domain.DeletedAccountRetention)
	h.at(t, closing.Add(time.Minute))
	student := h.signedInAgain(t, world.first)
	listed := h.calendarIDs(t, student.token, authStart, authStart.Add(30*24*time.Hour))
	chatListed := h.channelsOfDeletedTutorReadable(t, student.token)
	tutors := h.routeArray(t, student.token, studentTutorsRoute, "tutors")

	row := h.buildDirectly(t, student)

	t.Logf("the deleted tutor's lessons listed after the closing: %d", len(listed))
	if chatListed || len(tutors) != 0 {
		t.Fatalf("the lesson is not the only source of the deleted tutor: chat listed %v, %d tutors listed", chatListed, len(tutors))
	}
	if !slices.Contains(listed, lesson) {
		t.Fatal("the deleted tutor's lesson is no longer listed after the closing, so T-48's wording of this window no longer holds")
	}
	if row.State != domain.ExportFailed || row.Failure == nil || *row.Failure != domain.ExportFailureWithdrawn {
		t.Fatalf("a build that read the deleted tutor's lessons after the closing is %s, want it withdrawn", row.State)
	}
}

func TestTheExportRecordsADeletedAccountFromEverySource(t *testing.T) {
	t.Parallel()
	deleteSecond := func(t *testing.T, h *authHarness, world exportWorld) exportAccount {
		h.mustAnswer(t, h.deleteMe(t, world.second.token, world.second.password), http.StatusNoContent)
		return world.second
	}
	studentOwns := func(world exportWorld) exportAccount { return world.first }
	tutorOwns := func(world exportWorld) exportAccount { return world.tutor }
	sources := map[string]struct {
		owner func(exportWorld) exportAccount
		seed  func(*testing.T, *authHarness, exportWorld) exportAccount
	}{
		"tutor": {studentOwns, func(t *testing.T, h *authHarness, world exportWorld) exportAccount {
			h.mustAnswer(t, h.deleteMe(t, world.tutor.token, world.tutor.password), http.StatusNoContent)
			return world.tutor
		}},
		"message sender": {studentOwns, func(t *testing.T, h *authHarness, world exportWorld) exportAccount {
			h.sent(t, world.second.token, h.groupChannel(t, world.group.ID).ID, "A sender's message")
			return deleteSecond(t, h, world)
		}},
		"reaction member": {studentOwns, func(t *testing.T, h *authHarness, world exportWorld) exportAccount {
			reacted := h.sent(t, world.first.token, h.groupChannel(t, world.group.ID).ID, "A message a mate reacts to")
			h.mustAnswer(t, h.react(t, world.second.token, reacted.ID, string(domain.ReactionHeart)), http.StatusNoContent)
			return deleteSecond(t, h, world)
		}},
		"issuing tutor": {studentOwns, func(t *testing.T, h *authHarness, world exportWorld) exportAccount {
			template := h.workTemplate(t, world.tutor.token, "Issued work", instructionBlock(blockID(81, 1), "Read"))
			h.issueTemplate(t, world.tutor.token, template, toCard(world.firstCard.ID), studentExportPolicy, nil)
			h.mustAnswer(t, h.deleteMe(t, world.tutor.token, world.tutor.password), http.StatusNoContent)
			return world.tutor
		}},
		"quoted sender": {studentOwns, func(t *testing.T, h *authHarness, world exportWorld) exportAccount {
			group := h.groupChannel(t, world.group.ID).ID
			quoted := h.sent(t, world.second.token, group, "A quoted message")
			h.mustAnswer(t, h.reply(t, world.first.token, group, "A reply", quoted.ID, ""), http.StatusCreated)
			return deleteSecond(t, h, world)
		}},
		"tutor's card account": {tutorOwns, deleteSecond},
		"tutor's lesson audience": {tutorOwns, func(t *testing.T, h *authHarness, world exportWorld) exportAccount {
			h.scheduled(t, world.tutor.token, oneOffFor(world.secondCard.ID, "2030-03-12", "10:00"))
			return deleteSecond(t, h, world)
		}},
		"tutor's lesson roster": {tutorOwns, func(t *testing.T, h *authHarness, world exportWorld) exportAccount {
			lesson := h.scheduled(t, world.tutor.token, groupLessonFor(world.group.ID, "2030-03-13", "11:00")).Lessons[0].ID
			h.cancelWithRoster(t, lesson, world.second.id)
			return deleteSecond(t, h, world)
		}},
		"tutor's submission student": {tutorOwns, func(t *testing.T, h *authHarness, world exportWorld) exportAccount {
			template := h.workTemplate(t, world.tutor.token, "Deleted student's work", instructionBlock(blockID(91, 1), "Read"))
			h.issueTemplate(t, world.tutor.token, template, toCard(world.secondCard.ID), studentExportPolicy, nil)
			return deleteSecond(t, h, world)
		}},
		"tutor's message sender": {tutorOwns, func(t *testing.T, h *authHarness, world exportWorld) exportAccount {
			h.sent(t, world.second.token, h.groupChannel(t, world.group.ID).ID, "A sender's message")
			return deleteSecond(t, h, world)
		}},
		"tutor's reaction member": {tutorOwns, func(t *testing.T, h *authHarness, world exportWorld) exportAccount {
			message := h.sent(t, world.tutor.token, h.groupChannel(t, world.group.ID).ID, "A message to react to")
			h.mustAnswer(t, h.react(t, world.second.token, message.ID, string(domain.ReactionHeart)), http.StatusNoContent)
			return deleteSecond(t, h, world)
		}},
	}
	for name, source := range sources {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newAuthHarness(t)
			world := h.exportWorld(t)
			deleted := source.seed(t, h, world)
			owner := source.owner(world)
			due := h.deletedAtOf(t, deleted.id).Add(domain.DeletedAccountRetention)
			h.at(t, due.Add(-time.Hour))

			row := h.readyExportOf(t, h.signedInAgain(t, owner))
			h.at(t, due)
			late, _ := h.mintExport(t, h.signedInAgain(t, owner).token)

			if row.ExpiresAt == nil || !row.ExpiresAt.Equal(due) || late.status != http.StatusNotFound {
				t.Fatalf("the export expires at %v and a mint at the due time answered %d", row.ExpiresAt, late.status)
			}
		})
	}
}

func (h *authHarness) routeArray(t *testing.T, token string, path string, key string) []json.RawMessage {
	t.Helper()
	return decodedAs[[]json.RawMessage](t, h.routeBody(t, token, path)[key])
}

var fileObjectKeys = []string{"url", "expires_at", "path"}

func canonicalItem(t *testing.T, raw json.RawMessage, dropped ...string) string {
	t.Helper()
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		decoded := decodedAs[map[string]json.RawMessage](t, trimmed)
		_, isFile := decoded["filename"]
		canonical := map[string]json.RawMessage{}
		for key, value := range decoded {
			if slices.Contains(dropped, key) || (isFile && slices.Contains(fileObjectKeys, key)) {
				continue
			}
			canonical[key] = json.RawMessage(canonicalItem(t, value, dropped...))
		}
		encoded, err := json.Marshal(canonical)
		if err != nil {
			t.Fatal(err)
		}
		return string(encoded)
	}
	if len(trimmed) > 0 && trimmed[0] == '[' {
		decoded := decodedAs[[]json.RawMessage](t, trimmed)
		parts := make([]string, 0, len(decoded))
		for _, value := range decoded {
			parts = append(parts, canonicalItem(t, value, dropped...))
		}
		return "[" + strings.Join(parts, ",") + "]"
	}
	return canonicalJSON(t, trimmed)
}

func (h *authHarness) routePaged(t *testing.T, token string, path string, key string) []json.RawMessage {
	t.Helper()
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	var items []json.RawMessage
	cursor := ""
	for {
		query := path + separator + "limit=2"
		if cursor != "" {
			query += "&cursor=" + url.QueryEscape(cursor)
		}
		body := h.routeBody(t, token, query)
		items = append(items, decodedAs[[]json.RawMessage](t, body[key])...)
		next := decodedAs[*string](t, body["next_cursor"])
		if next == nil {
			return items
		}
		cursor = *next
	}
}

func (h *authHarness) windowIDs(t *testing.T, token string, from time.Time, until time.Time) []string {
	t.Helper()
	var ids []string
	for start := from; start.Before(until); start = start.Add(60 * 24 * time.Hour) {
		for _, id := range h.calendarIDs(t, token, start, start.Add(60*24*time.Hour)) {
			if !slices.Contains(ids, id) {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

func assertSameItems(t *testing.T, label string, exported []json.RawMessage, routed []json.RawMessage, dropped ...string) {
	t.Helper()
	if len(exported) != len(routed) {
		t.Errorf("%s: the export holds %d items and the routes %d", label, len(exported), len(routed))
		return
	}
	for index := 0; index < min(len(exported), len(routed)); index++ {
		left := canonicalItem(t, exported[index], dropped...)
		right := canonicalItem(t, routed[index], dropped...)
		if left != right {
			t.Errorf("%s item %d differs:\nexport %s\nroute  %s", label, index, left, right)
		}
	}
}

func (h *authHarness) routedHistory(t *testing.T, token string, channel string) []json.RawMessage {
	t.Helper()
	return h.routePaged(t, token, channelsRoute+"/"+channel+"/messages", "items")
}

func TestStudentExportMatchesTheRoutes(t *testing.T) {
	t.Parallel()
	h := newExportPagedHarness(t, 1)
	world := h.richStudentWorld(t)
	another := h.workTemplate(t, world.tutor.token, "Another work", instructionBlock(blockID(82, 1), "Read"))
	h.issueTemplate(t, world.tutor.token, another, toCard(world.firstCard.ID), studentExportPolicy, nil)
	if err := h.objects.Delete(context.Background(), h.storageKeyOf(t, world.chatFile)); err != nil {
		t.Fatal(err)
	}

	_, archive := h.studentArchiveOf(t, world.first)

	token := world.first.token
	assertSameItems(t, "tutors", archive.data.Tutors, h.routeArray(t, token, studentTutorsRoute, "tutors"))
	var homework []json.RawMessage
	for _, group := range []string{"todo", "submitted", "checked", "closed"} {
		homework = append(homework, h.routePaged(t, token, studentAssignmentsRoute+"?status="+group, "items")...)
	}
	assertSameItems(t, "homework", archive.data.Homework, homework)
	submissions := make([]json.RawMessage, 0, len(homework))
	for _, item := range homework {
		identified := decodedAs[struct {
			SubmissionID string `json:"submission_id"`
		}](t, item)
		submissions = append(submissions, json.RawMessage(h.submissionAnswer(t, token, identified.SubmissionID).body))
	}
	assertSameItems(t, "submissions", archive.data.Submissions, submissions)
	listed := h.windowIDs(t, token, authStart.Add(-24*time.Hour), authStart.Add(400*24*time.Hour))
	lessons := make([]json.RawMessage, 0, len(listed))
	for _, id := range listed {
		lessons = append(lessons, json.RawMessage(h.lessonPage(t, token, id).body))
	}
	assertSameItems(t, "lessons", archive.data.Lessons, lessons, "issues")
	channels := h.routePaged(t, token, channelsRoute, "items")
	exportedChannels := make([]json.RawMessage, 0, len(archive.data.Messages))
	for _, history := range archive.data.Messages {
		exportedChannels = append(exportedChannels, history.Channel)
	}
	assertSameItems(t, "channels", exportedChannels, channels)
	for _, channel := range channels {
		identified := decodedAs[struct {
			ID string `json:"id"`
		}](t, channel)
		history, _ := archive.historyOf(identified.ID)
		assertSameItems(t, "messages of "+identified.ID, history.Messages, h.routedHistory(t, token, identified.ID))
	}
	unavailable := 0
	for _, message := range archive.messages(t) {
		for _, attachment := range message.Attachments {
			if attachment.Path != nil {
				continue
			}
			unavailable++
			minted, address := h.addressOf(t, token, attachment.ID, "")
			if minted.status != http.StatusOK {
				continue
			}
			if served := h.fetch(t, address.URL, nil); served.status != http.StatusNotFound {
				t.Errorf("the route served the file %s the export marks not available: %d", attachment.ID, served.status)
			}
		}
	}
	if unavailable != 1 {
		t.Errorf("the export marks %d files not available, want the one whose object is gone", unavailable)
	}
}

func (h *authHarness) calendarIDs(t *testing.T, token string, from time.Time, to time.Time) []string {
	t.Helper()
	listed, _ := h.calendarItems(t, token, from, to, "")
	ids := make([]string, 0, len(listed))
	for _, item := range listed {
		identified := decodedAs[struct {
			ID string `json:"id"`
		}](t, json.RawMessage(item))
		ids = append(ids, identified.ID)
	}
	return ids
}

func digestOf(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func listedAndStoredFiles(archive studentArchive) ([]string, []string) {
	var listed, entries []string
	for _, entry := range archive.data.Files {
		listed = append(listed, entry.Path)
	}
	for _, name := range archive.entryNames() {
		if strings.HasPrefix(name, "files/") {
			entries = append(entries, name)
		}
	}
	slices.Sort(listed)
	return listed, entries
}

func (h *authHarness) undownloadableFiles(t *testing.T, token string, archive studentArchive) []string {
	t.Helper()
	var refused []string
	for _, entry := range archive.data.Files {
		attachment := attachmentOfEntry(entry.Path)
		for _, reference := range entry.References {
			query := "?ref=" + reference
			if strings.HasPrefix(reference, string(domain.ReferenceMessage)+":") {
				query = ""
			}
			minted, address := h.addressOf(t, token, attachment, query)
			served := h.fetch(t, address.URL, nil)
			if minted.status != http.StatusOK || served.status != http.StatusOK || digestOf([]byte(served.body)) != entry.SHA256 ||
				!bytes.Equal(archive.entries[entry.Path], []byte(served.body)) {
				refused = append(refused, fmt.Sprintf("%s through %s answered %d and %d", entry.Path, reference, minted.status, served.status))
			}
		}
	}
	return refused
}

func TestEveryArchivedFileIsDownloadableThroughItsRoute(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	world := h.richStudentWorld(t)
	h.attachToLesson(t, world.tutor.token, world.lesson, world.material)
	tutor := h.signedInAgain(t, world.tutor)

	_, archive := h.studentArchiveOf(t, world.first)
	_, tutorExport := h.tutorArchiveOf(t, tutor)

	if listed, entries := listedAndStoredFiles(archive); !slices.Equal(listed, entries) {
		t.Errorf("the archive's file entries %v differ from the listed files %v", entries, listed)
	}
	if len(archive.data.Files) < 4 {
		t.Fatalf("the rich world exported %d files, want the prompt image, the worksheet, the answer, the corrected file and the chat file", len(archive.data.Files))
	}
	if len(tutorExport.tutor.Files) < 5 {
		t.Fatalf("the rich world exported %d files to the tutor, want the prompt image, the worksheet, the answer, the corrected file and the chat file",
			len(tutorExport.tutor.Files))
	}
	if refused := h.undownloadableFiles(t, world.first.token, archive); len(refused) != 0 {
		t.Errorf("the student's archived files are not served through their routes: %v", refused)
	}
	if refused := h.undownloadableFiles(t, tutor.token, tutorExport.studentArchive); len(refused) != 0 {
		t.Errorf("the tutor's archived files are not served through their routes: %v", refused)
	}
}

func TestARefusedFileIsLeftOutOfTheExport(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	world := h.richStudentWorld(t)
	firstRevision := h.revisionID(t, world.submission, 1)
	later := h.correctedFile(t, world.tutor.token, world.submission, blockID(exportWorkGroup, exportFileQuestion), "later.pdf", 90)
	h.reviewSaved(t, world.tutor.token, world.submission, reviewJSON(t, 2, 0, saveID(3), "Overall good",
		commented(scored(blockID(exportWorkGroup, exportFileQuestion), 4), "See the new marks", later)))
	h.published(t, world.tutor.token, world.submission, domain.ReviewCheck, 2)
	h.mustAnswer(t, h.groupCall(t, world.tutor.token, http.MethodPatch, world.group.ID, "", fmt.Sprintf(`{"remove":[%q]}`, world.firstCard.ID)), http.StatusOK)
	first := h.signedInAgain(t, world.first)
	third := h.exportAccountAs(t, domain.RoleStudent, 4, thirdExportEmail)
	thirdCard := h.linkedSignedIn(t, world.tutor.token, third.token, "Third student", thirdExportEmail)
	reclaimSubmission, reclaimAnswer := h.answeredFileWork(t, world.tutor.token, h.signedInAgain(t, world.second), world.secondCard.ID,
		"Reclaimed work", "reclaimed-answer.pdf")
	discardedSubmission, discardedAnswer := h.answeredFileWork(t, world.tutor.token, third, thirdCard.ID, "Discarded work", "discarded-answer.pdf")
	h.setRelationshipState(t, world.secondCard.ID, domain.RelationshipReclaimPending)
	h.setRelationshipState(t, thirdCard.ID, domain.RelationshipDiscarded)
	tutor := h.signedInAgain(t, world.tutor)

	_, archive := h.studentArchiveOf(t, first)
	_, tutorExport := h.tutorArchiveOf(t, tutor)
	staleMint, _ := h.addressOf(t, first.token, world.corrected, "?ref=review_item:"+firstRevision)
	chatMint, _ := h.addressOf(t, first.token, world.chatFile, "")
	reclaimMint, _ := h.addressOf(t, tutor.token, reclaimAnswer, "?ref=answer:"+reclaimSubmission)
	discardedMint, _ := h.addressOf(t, tutor.token, discardedAnswer, "?ref=answer:"+discardedSubmission)

	tutorArchived := tutorExport.archivedAttachments()
	if slices.Contains(tutorArchived, reclaimAnswer) || reclaimMint.status != http.StatusNotFound {
		t.Errorf("an answer file of a reclaim-pending link is archived %v for the tutor and its mint answered %d", slices.Contains(tutorArchived, reclaimAnswer),
			reclaimMint.status)
	}
	if slices.Contains(tutorArchived, discardedAnswer) || discardedMint.status != http.StatusNotFound {
		t.Errorf("an answer file of a discarded link is archived %v for the tutor and its mint answered %d", slices.Contains(tutorArchived, discardedAnswer),
			discardedMint.status)
	}
	if slices.Contains(tutorArchived, world.corrected) || !slices.Contains(tutorArchived, later) {
		t.Error("the tutor's export does not hold exactly the corrected files of the latest published revision")
	}
	archived := archive.archivedAttachments()
	if slices.Contains(archived, world.corrected) || staleMint.status != http.StatusNotFound {
		t.Errorf("the corrected file of an earlier revision is archived %v and its mint answered %d", slices.Contains(archived, world.corrected), staleMint.status)
	}
	if slices.Contains(archived, world.chatFile) || chatMint.status != http.StatusNotFound {
		t.Errorf("a chat file of a channel the student left is archived %v and its mint answered %d", slices.Contains(archived, world.chatFile), chatMint.status)
	}
	if !slices.Contains(archived, later) {
		t.Error("the corrected file of the latest revision is missing")
	}
}

type pointsOnlyStudent struct {
	account    exportAccount
	submission string
	card       string
}

func identifierFree(text string, students pointsOnlyStudent, identifiers ...string) string {
	for index, identifier := range append([]string{students.submission, students.card, students.account.id.String()}, identifiers...) {
		text = strings.ReplaceAll(text, identifier, fmt.Sprintf("identifier-%d", index))
	}
	return text
}

func TestAPointsOnlyPublicationChangesNothingInTheStudentsExport(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	world := h.exportWorld(t)
	template := h.workTemplate(t, world.tutor.token, "Points", fileBlock(blockID(86, 1), string(domain.AnswerFilePDF)))
	students := []pointsOnlyStudent{{account: world.first, card: world.firstCard.ID}, {account: world.second, card: world.secondCard.ID}}
	steps := []func(student *pointsOnlyStudent){
		func(student *pointsOnlyStudent) {
			_, submissions := h.issueTemplate(t, world.tutor.token, template, toCard(student.card), studentExportPolicy, nil)
			student.submission = submissions[0].id
		},
		func(student *pointsOnlyStudent) {
			draft := h.saved(t, student.account.token, student.submission, answersJSON(t, 1, saveID(1)))
			h.mustAnswer(t, h.submitWork(t, student.account.token, student.submission, draft.Version), http.StatusOK)
		},
		func(student *pointsOnlyStudent) {
			h.reviewSaved(t, world.tutor.token, student.submission, reviewJSON(t, 1, 0, saveID(2), "Same feedback",
				commented(scored(blockID(86, 1), 4), "Same comment")))
		},
		func(student *pointsOnlyStudent) {
			h.published(t, world.tutor.token, student.submission, domain.ReviewCheck, 1)
		},
	}
	for index, step := range steps {
		for student := range students {
			h.at(t, authStart.Add(time.Duration(index+1)*10*time.Second))
			step(&students[student])
		}
	}
	h.at(t, authStart.Add(time.Minute))
	h.reviewSaved(t, world.tutor.token, students[0].submission, reviewJSON(t, 2, 0, saveID(3), "Same feedback",
		commented(scored(blockID(86, 1), 1), "Same comment")))
	h.published(t, world.tutor.token, students[0].submission, domain.ReviewCheck, 2)

	_, published := h.studentArchiveOf(t, h.signedInAgain(t, students[0].account))
	_, untouched := h.studentArchiveOf(t, h.signedInAgain(t, students[1].account))

	parts := func(archive studentArchive, student pointsOnlyStudent) string {
		var text strings.Builder
		for _, raw := range append(append([]json.RawMessage{}, archive.data.Homework...), archive.data.Submissions...) {
			text.WriteString(canonicalJSON(t, raw))
		}
		for _, sheet := range []string{"Homework", "Answers", "Feedback"} {
			for _, row := range archive.rows(sheet) {
				text.WriteString(strings.Join(row, "|"))
			}
		}
		return identifierFree(text.String(), student)
	}
	if left, right := parts(published, students[0]), parts(untouched, students[1]); left != right {
		t.Fatalf("a points-only publication changed the student's export:\n%s\n%s", left, right)
	}
}

func TestAChannelThatMovesDuringTheBuildIsStillExported(t *testing.T) {
	t.Parallel()
	h, held := newHeldStorageHarnessTimed(t, export.Timing{Complete: exportCompleteTimeout, ReadPage: 1})
	world := h.exportWorld(t)
	group := h.groupChannel(t, world.group.ID).ID
	direct := h.directChannel(t, world.firstCard.ID).ID
	outsiderCard := h.linkedSignedIn(t, world.outsider.token, world.first.token, "Outsider card", firstExportEmail)
	outsider := h.directChannel(t, outsiderCard.ID).ID
	h.sent(t, world.tutor.token, group, "An old group message")
	h.sent(t, world.outsider.token, outsider, "An outsider's message")
	upload := h.chatUpload(t, world.tutor.token, direct, "notes.txt", plainText(64))
	h.mustAnswer(t, upload, http.StatusCreated)
	h.sent(t, world.tutor.token, direct, "A newer direct message", uploadedID(t, upload))
	key := h.storageKeyOf(t, uploadedID(t, upload))
	first := h.signedInAgain(t, world.first)
	h.mustAnswer(t, h.requestExport(t, first.token, first.password), http.StatusAccepted)
	held.holdNext(func(request *http.Request) bool {
		return request.Method == http.MethodGet && strings.Contains(request.URL.Path, key)
	})
	building := h.tickInBackground(t)
	held.awaitReached(t)

	h.sent(t, world.tutor.token, group, "A message that moves the channel")
	held.resume()
	awaitTick(t, building)
	h.tick(t)
	minted, address := h.mintExport(t, first.token)
	h.mustAnswer(t, minted, http.StatusOK)
	archive := openStudentArchive(t, downloadArchive(t, address.URL))

	if _, found := archive.historyOf(group); !found {
		t.Fatal("the group channel is missing from the archive")
	}
	for _, channel := range []string{direct, outsider} {
		if _, found := archive.historyOf(channel); !found {
			t.Fatalf("the channel %s is missing from the archive", channel)
		}
	}
	if len(archive.data.Messages) != 3 {
		t.Fatalf("the archive holds %d channels, want 3", len(archive.data.Messages))
	}
	if archive.messageRow("An old group message") == nil || archive.messageRow("A newer direct message") == nil {
		t.Fatal("a channel's history is missing from the messages sheet")
	}
}

func TestASubmissionThatMovesDuringTheBuildIsExportedOnce(t *testing.T) {
	t.Parallel()
	h, held := newHeldStorageHarness(t)
	world := h.exportWorld(t)
	direct := h.directChannel(t, world.firstCard.ID).ID
	upload := h.chatUpload(t, world.tutor.token, direct, "notes.txt", plainText(64))
	h.mustAnswer(t, upload, http.StatusCreated)
	h.sent(t, world.tutor.token, direct, "A file to hold the build on", uploadedID(t, upload))
	key := h.storageKeyOf(t, uploadedID(t, upload))
	template := h.workTemplate(t, world.tutor.token, "Moving work", fileBlock(blockID(83, 1), string(domain.AnswerFilePDF)))
	_, submissions := h.issueTemplate(t, world.tutor.token, template, toCard(world.firstCard.ID), studentExportPolicy, nil)
	first := h.signedInAgain(t, world.first)
	h.mustAnswer(t, h.requestExport(t, first.token, first.password), http.StatusAccepted)
	held.holdNext(func(request *http.Request) bool {
		return request.Method == http.MethodGet && strings.Contains(request.URL.Path, key)
	})
	building := h.tickInBackground(t)
	held.awaitReached(t)

	draft := h.saved(t, first.token, submissions[0].id, answersJSON(t, 1, saveID(1)))
	h.mustAnswer(t, h.submitWork(t, first.token, submissions[0].id, draft.Version), http.StatusOK)
	held.resume()
	awaitTick(t, building)
	minted, address := h.mintExport(t, first.token)
	h.mustAnswer(t, minted, http.StatusOK)
	archive := openStudentArchive(t, downloadArchive(t, address.URL))

	if len(archive.data.Homework) != 1 || len(archive.data.Submissions) != 1 || len(archive.rows("Homework")) != 1 {
		t.Fatalf("a submission that moved between groups is exported %d times in homework, %d in submissions and %d in the sheet",
			len(archive.data.Homework), len(archive.data.Submissions), len(archive.rows("Homework")))
	}
}

func TestCalendarKeysetMatchesTheWindows(t *testing.T) {
	t.Parallel()
	h := newExportPagedHarness(t, 1)
	world := h.exportWorld(t)
	var seeded []string
	for _, slot := range [][2]string{
		{"2030-03-11", "10:00"}, {"2030-05-08", "12:00"}, {"2030-05-10", "10:00"}, {"2030-05-11", "10:00"}, {"2030-11-20", "10:00"}, {"2031-10-14", "10:00"},
	} {
		seeded = append(seeded, h.scheduled(t, world.tutor.token, oneOffFor(world.firstCard.ID, slot[0], slot[1])).Lessons[0].ID)
	}
	seeded = append(seeded, h.scheduled(t, world.tutor.token, groupLessonFor(world.group.ID, "2030-07-01", "12:00")).Lessons[0].ID)
	cancelled := h.scheduled(t, world.tutor.token, oneOffFor(world.firstCard.ID, "2030-04-02", "09:00")).Lessons[0].ID
	h.cancelWithRoster(t, cancelled, world.first.id)
	formerGroup := h.newGroup(t, world.tutor.token, "Former group", []string{world.firstCard.ID})
	former := h.scheduled(t, world.tutor.token, groupLessonFor(formerGroup.ID, "2030-03-14", "11:00")).Lessons[0].ID
	h.cancelWithRoster(t, former, world.first.id)
	h.mustAnswer(t, h.groupCall(t, world.tutor.token, http.MethodPatch, formerGroup.ID, "", fmt.Sprintf(`{"remove":[%q]}`, world.firstCard.ID)), http.StatusOK)
	seeded = append(seeded, cancelled, former)
	outsiderCard := h.linkedSignedIn(t, world.outsider.token, world.first.token, "Outsider card", firstExportEmail)
	h.scheduled(t, world.outsider.token, oneOffFor(outsiderCard.ID, "2030-06-01", "10:00"))
	h.mustAnswer(t, h.deleteMe(t, world.outsider.token, world.outsider.password), http.StatusNoContent)
	student := h.signedInAgain(t, world.first)
	var windows []string
	for from := authStart.Add(-24 * time.Hour); from.Before(time.Date(2032, time.January, 1, 0, 0, 0, 0, time.UTC)); from = from.Add(60 * 24 * time.Hour) {
		for _, id := range h.calendarIDs(t, student.token, from, from.Add(60*24*time.Hour)) {
			if !slices.Contains(windows, id) {
				windows = append(windows, id)
			}
		}
	}

	_, archive := h.studentArchiveOf(t, student)

	var exported []string
	for _, raw := range archive.data.Lessons {
		identified := decodedAs[struct {
			ID string `json:"id"`
		}](t, raw)
		exported = append(exported, identified.ID)
	}
	slices.Sort(windows)
	slices.Sort(exported)
	if !slices.Equal(windows, exported) {
		t.Fatalf("the export lists the lessons %v, the calendar windows %v", exported, windows)
	}
	for _, id := range seeded {
		if !slices.Contains(exported, id) {
			t.Errorf("the seeded lesson %s is in neither the windows nor the export", id)
		}
	}
}

func TestExportRecordsCarryNoContentKeyOrAddress(t *testing.T) {
	t.Parallel()
	h, held := newHeldStorageHarness(t)
	world := h.richStudentWorld(t)
	chatKey := h.storageKeyOf(t, world.chatFile)
	if err := h.objects.Delete(context.Background(), chatKey); err != nil {
		t.Fatal(err)
	}
	h.mustAnswer(t, h.requestExport(t, world.first.token, world.first.password), http.StatusAccepted)
	held.holdNext(multipartComplete)
	building := h.tickInBackground(t)
	held.awaitReached(t)
	row, _ := h.exportOf(t, world.first.id)
	var upload string
	if err := h.database.Pool.QueryRow(context.Background(), "SELECT multipart_upload_id FROM upload_intents WHERE attachment_id = $1::uuid",
		row.BuildID.String()).Scan(&upload); err != nil {
		t.Fatal(err)
	}

	held.resume()
	awaitTick(t, building)
	h.tick(t)
	h.tutorArchiveOf(t, h.signedInAgain(t, world.tutor))

	sentinels := []string{
		"A mate's message", "My own message", "Export work", "Worksheet", "notes.txt", "answer.pdf", "corrected.pdf", "prompt.png", "Overall good",
		"First student", "Second student", exportGroupName, lessonTitle, "The tutor's message", "A direct message",
		h.storageKeyOf(t, world.answerFile), chatKey, h.storageKeyOf(t, world.tutorFile), h.storageKeyOf(t, world.corrected),
		*row.StorageKey, upload, "X-Amz", "uploadId",
	}
	unreadable := 0
	for _, line := range strings.Split(h.logs.String(), "\n") {
		if !strings.Contains(line, `"msg":"export.`) {
			continue
		}
		if strings.Contains(line, `"msg":"export.file.unreadable"`) {
			unreadable++
		}
		for _, sentinel := range sentinels {
			if strings.Contains(line, sentinel) {
				t.Errorf("an export record carries %q: %s", sentinel, line)
			}
		}
	}
	if unreadable != 2 {
		t.Errorf("the scan saw %d export.file.unreadable records, want one for the student's export and one for the tutor's", unreadable)
	}
}
