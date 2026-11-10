package api_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

const (
	lessonMaterialTitle    = "Lesson material sentinel"
	lessonMaterialFilename = "diagram.png"
)

func TestLessonMaterialsAnyStatus(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.visibilityParties(t)
	ids := materialIDsOfItems(h.libraryMaterials(t, parties.tutor, parties.tutorToken, 2))
	past := h.scheduled(t, parties.tutorToken, oneOffFor(parties.card, "2030-03-11", "10:00")).Lessons[0]
	live := h.scheduled(t, parties.tutorToken, oneOffFor(parties.card, "2030-03-12", "10:00")).Lessons[0]
	ended := h.scheduled(t, parties.tutorToken, oneOffFor(parties.card, "2030-03-13", "10:00")).Lessons[0]
	cancelled := h.scheduled(t, parties.tutorToken, oneOffFor(parties.card, "2030-03-14", "10:00")).Lessons[0]
	h.cancelledThis(t, parties.tutorToken, cancelled)
	h.setStatus(t, live.ID, string(domain.LessonLive))
	h.setStatus(t, ended.ID, string(domain.LessonEnded))
	h.at(t, time.Date(2030, time.March, 20, 0, 0, 0, 0, time.UTC))
	token := h.tutorToken(t)
	lessons := map[string]string{"a past lesson": past.ID, "a live lesson": live.ID, "an ended lesson": ended.ID, "a cancelled lesson": cancelled.ID}
	versions := map[string]int{}
	for name, lesson := range lessons {
		versions[name] = decodeLesson(t, h.lessonPage(t, token, lesson)).Version
	}

	answers := map[string]response{}
	for name, lesson := range lessons {
		answers[name] = h.attachMaterials(t, token, lesson, versions[name], ids[1], ids[0])
	}

	for name, answer := range answers {
		h.mustAnswer(t, answer, http.StatusOK)
		view := decodeLesson(t, answer)
		if !slices.Equal(attachedMaterialIDs(view), []string{ids[1], ids[0]}) || view.Version != versions[name]+1 {
			t.Errorf("%s holds %v at version %d after version %d", name, attachedMaterialIDs(view), view.Version, versions[name])
		}
		for _, material := range view.Materials {
			if !strings.HasPrefix(material.URL, "/api/v1/files/"+material.AttachmentID+"?t=") || material.ExpiresAt.IsZero() {
				t.Errorf("%s lists a material without its address: %+v", name, material)
			}
		}
		if answer.headers.Get("Cache-Control") != "no-store" || len(h.storedLesson(t, lessons[name]).Overridden) != 0 {
			t.Errorf("%s answered without no-store or recorded an override", name)
		}
	}
}

func TestLessonMaterialRefusalsAndTheirOrder(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.visibilityParties(t)
	other, otherToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
	token := parties.tutorToken
	ids := materialIDsOfItems(h.libraryMaterials(t, parties.tutor, token, domain.LessonMaterialsMax))
	foreign := h.createMaterial(t, otherToken, h.pendingLibraryFile(t, other, 71, 1, 1000), "Foreign", nil)
	deleted := h.libraryMaterial(t, parties.tutor, token, 30, "Deleted")
	kept := h.libraryMaterial(t, parties.tutor, token, 31, "Kept")
	lesson := h.scheduled(t, token, oneOffFor(parties.card, "2030-03-12", "18:00")).Lessons[0]
	series := h.scheduled(t, token, oneOffFor(parties.card, "2030-03-13", "18:00").weekly([]int{3}, 3))
	endedCard := h.linkedCard(t, parties.tutor, 13, domain.RelationshipEnded)
	h.attached(t, token, lesson.ID, 1, kept.ID)
	h.mustAnswer(t, h.deleteMaterial(t, token, deleted.ID), http.StatusNoContent)
	h.mustAnswer(t, h.deleteMaterial(t, token, kept.ID), http.StatusNoContent)

	notFound := map[string]response{
		"another tutor's material":            h.attachMaterials(t, token, lesson.ID, 2, foreign.ID),
		"a deleted material not attached":     h.attachMaterials(t, token, lesson.ID, 2, deleted.ID),
		"a stale version with a random id":    h.attachMaterials(t, token, lesson.ID, 1, randomCardID),
		"a series edit with another's":        h.editLesson(t, token, series.Lessons[0].ID, lessonPatch{Scope: "following", Version: 1, MaterialIDs: &[]string{foreign.ID}}),
		"a lesson creation with another's":    h.createLessons(t, token, lessonRequest{RelationshipID: &parties.card, Title: lessonTitle, Date: "2030-04-01", Start: "18:00", Duration: 60, MaterialIDs: []string{foreign.ID}}, ""),
		"another tutor attaching to a lesson": h.attachMaterials(t, otherToken, lesson.ID, 2, foreign.ID),
		"another's material for an ended relationship": h.createLessons(t, token, lessonRequest{
			RelationshipID: &endedCard, Title: lessonTitle, Date: "2030-04-01", Start: "18:00", Duration: 60, MaterialIDs: []string{foreign.ID},
		}, ""),
	}
	random := h.attachMaterials(t, token, lesson.ID, 2, randomCardID)
	staleAndRepeated := h.attachMaterials(t, token, lesson.ID, 1, ids[0], ids[0])
	repeated := h.attachMaterials(t, token, lesson.ID, 2, ids[0], ids[0])
	tooMany := h.attachMaterials(t, token, lesson.ID, 2, append(slices.Clone(ids), ids[0])...)
	unchanged := h.attachMaterials(t, token, lesson.ID, 2, kept.ID)
	keptAfterDeletion := h.attachMaterials(t, token, lesson.ID, 2, kept.ID, ids[0])
	full := h.attachMaterials(t, token, lesson.ID, 3, ids...)
	thisScope := h.editLesson(t, token, series.Lessons[0].ID, lessonPatch{Scope: "this", Version: 1, MaterialIDs: &[]string{ids[0]}})
	created := h.createLessons(t, token, lessonRequest{
		RelationshipID: &parties.card, Title: lessonTitle, Date: "2030-04-02", Start: "18:00", Duration: 60, MaterialIDs: []string{ids[1], ids[0]},
	}, "")
	creationRepeat := h.createLessons(t, token, lessonRequest{
		RelationshipID: &parties.card, Title: lessonTitle, Date: "2030-04-01", Start: "18:00", Duration: 60, MaterialIDs: []string{ids[0], ids[0]},
	}, "")

	for name, answer := range notFound {
		if answer.status != http.StatusNotFound || refusal(t, answer) != refusal(t, random) {
			t.Errorf("%s answered %s", name, describe(answer))
		}
	}
	if staleAndRepeated.status != http.StatusConflict || code(t, staleAndRepeated) != "conflict" {
		t.Errorf("a stale version with a repeat answered %s", describe(staleAndRepeated))
	}
	for name, invalid := range map[string]struct {
		answer response
		code   string
	}{
		"a repeat": {repeated, "invalid"}, "more than 20": {tooMany, "too_long"}, "a repeat on creation": {creationRepeat, "invalid"},
		"materials with the scope this": {thisScope, "not_allowed"},
	} {
		if path, field := fieldOf(t, invalid.answer); path != "body.material_ids" || field != invalid.code {
			t.Errorf("%s answered %s", name, describe(invalid.answer))
		}
	}
	h.mustAnswer(t, unchanged, http.StatusOK)
	if view := decodeLesson(t, unchanged); view.Version != 2 || len(view.Materials) != 1 || view.Materials[0].Title != "Kept" {
		t.Errorf("an unchanged list answered %+v", view)
	}
	h.mustAnswer(t, keptAfterDeletion, http.StatusOK)
	h.mustAnswer(t, full, http.StatusOK)
	if view := decodeLesson(t, full); !slices.Equal(attachedMaterialIDs(view), ids) || view.Version != 4 {
		t.Errorf("twenty materials answered %v at version %d", attachedMaterialIDs(view), view.Version)
	}
	if lessons := decodeCreated(t, created).Lessons; len(lessons) != 1 || !slices.Equal(attachedMaterialIDs(lessons[0]), []string{ids[1], ids[0]}) {
		t.Errorf("a one-off creation with materials answered %s", describe(created))
	}
	if h.storedLesson(t, series.Lessons[0].ID).Version != 1 {
		t.Error("a refused series edit changed the lesson")
	}
}

func TestLessonMaterialAfterLibraryDeleteStillDownloads(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	parties := h.visibilityParties(t)
	token := parties.tutorToken
	upload := h.libraryUpload(t, token, domain.ContextLibrary, "worksheet.pdf", pdfFile(64))
	h.mustAnswer(t, upload, http.StatusCreated)
	material := h.createMaterial(t, token, uploadedID(t, upload), "Worksheet", nil)
	lesson := h.scheduled(t, token, oneOffFor(parties.card, "2030-03-12", "18:00")).Lessons[0]
	h.attached(t, token, lesson.ID, 1, material.ID)
	h.mustAnswer(t, h.patchMaterial(t, token, material.ID, `{"title":"Worksheet, revised"}`), http.StatusOK)
	h.mustAnswer(t, h.deleteMaterial(t, token, material.ID), http.StatusNoContent)

	tutorPage := h.lessonPage(t, token, lesson.ID)
	studentPage := h.lessonPage(t, parties.studentToken, lesson.ID)
	minted, address := h.addressOf(t, parties.studentToken, material.AttachmentID, "?ref=lesson:"+lesson.ID)
	inline, _ := h.addressOf(t, parties.studentToken, material.AttachmentID, "?ref=lesson:"+lesson.ID+"&disposition=inline")
	served := h.fetch(t, address.URL, nil)
	studentView := decodeLesson(t, studentPage)
	fromDetail := h.fetch(t, studentView.Materials[0].URL, nil)

	tutorView := decodeLesson(t, tutorPage)
	if len(tutorView.Materials) != 1 || tutorView.Materials[0].Title != "Worksheet, revised" || tutorView.Materials[0].MaterialID == nil ||
		*tutorView.Materials[0].MaterialID != material.ID || tutorView.Materials[0].Filename != "worksheet.pdf" {
		t.Fatalf("the tutor's lesson lists %+v", tutorView.Materials)
	}
	if strings.Contains(studentPage.body, `"material_id"`) || len(studentView.Materials) != 1 || studentView.Materials[0].Title != "Worksheet, revised" ||
		studentPage.headers.Get("Cache-Control") != "no-store" {
		t.Fatalf("the student's lesson answered %s", describe(studentPage))
	}
	if minted.status != http.StatusOK || minted.headers.Get("Cache-Control") != "no-store" {
		t.Fatalf("minting answered %s", describe(minted))
	}
	if path, field := fieldOf(t, inline); path != "query.disposition" || field != "invalid" {
		t.Fatalf("an inline PDF answered %s", describe(inline))
	}
	for name, fetched := range map[string]response{"the minted address": served, "the detail's address": fromDetail} {
		if fetched.status != http.StatusOK || fetched.body != string(pdfFile(64)) || !strings.HasPrefix(fetched.headers.Get("Content-Disposition"), "attachment") {
			t.Fatalf("%s served %d with %q", name, fetched.status, fetched.headers.Get("Content-Disposition"))
		}
	}
	if h.fileUsage(t, parties.tutor) == 0 || h.storageDeletionsOf(t, material.AttachmentID) != 0 {
		t.Fatal("the lesson's reference did not keep the file's charge")
	}
}

func TestLessonMaterialReleasesLastReference(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.visibilityParties(t)
	token := parties.tutorToken
	material := h.libraryMaterial(t, parties.tutor, token, 1, "Worksheet")
	lesson := h.scheduled(t, token, oneOffFor(parties.card, "2030-03-12", "18:00")).Lessons[0]
	h.attached(t, token, lesson.ID, 1, material.ID)
	h.mustAnswer(t, h.deleteMaterial(t, token, material.ID), http.StatusNoContent)
	usage, deletions := h.fileUsage(t, parties.tutor), h.storageDeletionsOf(t, material.AttachmentID)

	detached := h.attached(t, token, lesson.ID, 2)

	if usage != 1000 || deletions != 0 {
		t.Fatalf("before detaching the usage is %d with %d deletions queued", usage, deletions)
	}
	if len(detached.Materials) != 0 || h.fileUsage(t, parties.tutor) != 0 || h.storageDeletionsOf(t, material.AttachmentID) != 1 ||
		h.attachmentStateOf(t, material.AttachmentID) != string(domain.AttachmentDeleted) || h.referencesOfKind(t, material.AttachmentID, domain.ReferenceLesson) != 0 {
		t.Fatalf("detaching the last reference left usage %d, %d deletions and state %s", h.fileUsage(t, parties.tutor),
			h.storageDeletionsOf(t, material.AttachmentID), h.attachmentStateOf(t, material.AttachmentID))
	}
	if h.countLogs(`"msg":"file.reference.removed"`, `"ref_kind":"lesson"`, material.AttachmentID) != 1 {
		t.Fatal("the removed reference was not recorded")
	}
}

func TestSeriesMaterialReleasesLastReference(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.visibilityParties(t)
	token := parties.tutorToken
	material := h.libraryMaterial(t, parties.tutor, token, 1, "Worksheet")
	request := oneOffFor(parties.card, firstLessonDate, firstLessonLocal).weekly([]int{1}, 4)
	request.MaterialIDs = []string{material.ID}
	created := h.scheduled(t, token, request)
	h.mustAnswer(t, h.deleteMaterial(t, token, material.ID), http.StatusNoContent)
	usage := h.fileUsage(t, parties.tutor)

	split := h.editLesson(t, token, created.Lessons[0].ID, lessonPatch{Scope: "following", Version: 1, MaterialIDs: &[]string{}})

	h.mustAnswer(t, split, http.StatusOK)
	changed := decodeChanged(t, split)
	if usage != 1000 || changed.Series == nil || len(h.seriesMaterialIDs(t, changed.Series.ID)) != 0 || len(h.seriesMaterialIDs(t, created.Series.ID)) != 0 {
		t.Fatalf("the split answered %s with usage %d before it", describe(split), usage)
	}
	for _, lesson := range changed.Lessons {
		if len(lesson.Materials) != 0 {
			t.Fatalf("a covered lesson kept %+v", lesson.Materials)
		}
	}
	if h.fileUsage(t, parties.tutor) != 0 || h.storageDeletionsOf(t, material.AttachmentID) != 1 ||
		h.countRows(t, "SELECT count(ref_id) FROM attachment_references WHERE attachment_id = $1::uuid", material.AttachmentID) != 0 {
		t.Fatalf("the series' last reference left usage %d and %d deletions", h.fileUsage(t, parties.tutor), h.storageDeletionsOf(t, material.AttachmentID))
	}
	if h.countLogs(`"msg":"lesson.materials.changed"`, `"series_id":"`+changed.Series.ID+`"`, `"count":0`, `"scope":"following"`) != 1 {
		t.Fatal("the series' material change was not recorded")
	}
}

func TestSeriesMaterialsCopiedAndOverridden(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.visibilityParties(t)
	token := parties.tutorToken
	items := h.libraryMaterials(t, parties.tutor, token, 3)
	first, second, third := items[0], items[1], items[2]
	request := oneOffFor(parties.card, firstLessonDate, firstLessonLocal).weekly([]int{1}, 4)
	request.MaterialIDs = []string{first.ID, second.ID}
	created := h.scheduled(t, token, request)
	overridden := h.attached(t, token, created.Lessons[1].ID, 1, third.ID)

	split := decodeChanged(t, h.editLesson(t, token, created.Lessons[0].ID, lessonPatch{
		Scope: "following", Version: 1, MaterialIDs: &[]string{second.ID, third.ID},
	}))
	if split.Series == nil {
		t.Fatal("the split answered no series")
	}
	splitMaterials := h.seriesMaterialIDs(t, split.Series.ID)
	originalMaterials := h.seriesMaterialIDs(t, created.Series.ID)
	afterSplit := map[string][]string{}
	for _, lesson := range created.Lessons {
		afterSplit[lesson.ID] = h.lessonMaterialIDs(t, lesson.ID)
	}
	seriesEmails := h.outboxKeysOf(t, domain.EventLessonSeriesChanged)
	later := decodeChanged(t, h.editLesson(t, token, created.Lessons[2].ID, following(h.storedLesson(t, created.Lessons[2].ID).Version).titled(newLessonTitle)))

	for _, lesson := range created.Lessons {
		if !slices.Equal(attachedMaterialIDs(lesson), []string{first.ID, second.ID}) {
			t.Fatalf("a created occurrence holds %v", attachedMaterialIDs(lesson))
		}
	}
	if !slices.Equal(h.storedLesson(t, overridden.ID).Overridden, []string{string(domain.LessonFieldMaterials)}) {
		t.Fatalf("the overridden occurrence records %v", h.storedLesson(t, overridden.ID).Overridden)
	}
	if !slices.Equal(splitMaterials, []string{second.ID, third.ID}) || len(originalMaterials) != 0 {
		t.Fatalf("the split series holds %v and the original %v", splitMaterials, originalMaterials)
	}
	for index, lesson := range created.Lessons {
		want := []string{second.ID, third.ID}
		if index == 1 {
			want = []string{third.ID}
		}
		if !slices.Equal(afterSplit[lesson.ID], want) {
			t.Errorf("occurrence %d holds %v after the split, want %v", index, afterSplit[lesson.ID], want)
		}
	}
	if later.Series == nil || !slices.Equal(h.seriesMaterialIDs(t, later.Series.ID), []string{second.ID, third.ID}) ||
		len(h.seriesMaterialIDs(t, split.Series.ID)) != 0 || !slices.Equal(h.lessonMaterialIDs(t, created.Lessons[3].ID), []string{second.ID, third.ID}) {
		t.Fatalf("a split without materials copied %v", h.seriesMaterialIDs(t, later.Series.ID))
	}
	if h.fileUsage(t, parties.tutor) != 3000 || h.referencesOfKind(t, first.AttachmentID, domain.ReferenceLesson) != 0 ||
		h.referencesOfKind(t, second.AttachmentID, domain.ReferenceSeries) != 1 || h.referencesOfKind(t, third.AttachmentID, domain.ReferenceLesson) != 4 {
		t.Fatalf("the references and charges are wrong: usage %d", h.fileUsage(t, parties.tutor))
	}
	if seriesEmails != 0 {
		t.Fatalf("a change of materials alone queued %d lesson emails", seriesEmails)
	}
}

func TestHorizonExtensionCopiesSeriesMaterials(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
	items := h.libraryMaterials(t, tutor, token, 2)
	request := oneOffFor(card, firstLessonDate, firstLessonLocal).until([]int{1}, mondaysUntilDecember)
	request.AllowOverlap, request.MaterialIDs = true, materialIDsOfItems(items)
	created := h.scheduled(t, token, request)
	usage := h.fileUsage(t, tutor)

	h.tickAt(t, authStart.Add(7*24*time.Hour))

	lessons := h.seriesLessons(t, created.Series.ID)
	newest, _, _ := strings.Cut(lessons[len(lessons)-1], "@")
	if len(lessons) != 13 || !slices.Equal(h.lessonMaterialIDs(t, newest), materialIDsOfItems(items)) {
		t.Fatalf("the new occurrence of %d holds %v", len(lessons), h.lessonMaterialIDs(t, newest))
	}
	for _, item := range items {
		if h.referencesOfKind(t, item.AttachmentID, domain.ReferenceLesson) != 13 || h.referencesOfKind(t, item.AttachmentID, domain.ReferenceSeries) != 1 {
			t.Fatalf("a file has %d lesson references", h.referencesOfKind(t, item.AttachmentID, domain.ReferenceLesson))
		}
	}
	if h.fileUsage(t, tutor) != usage || h.countRows(t, "SELECT count(attachment_id) FROM quota_charges WHERE tutor_id = $1::uuid", tutor.String()) != 2 {
		t.Fatal("the extension changed a charge")
	}
}

func TestSeriesExtensionRacesMaterialRemoval(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	card := h.linkedCard(t, tutor, 10, domain.RelationshipActive)
	removed, kept := h.libraryMaterial(t, tutor, token, 1, "Removed"), h.libraryMaterial(t, tutor, token, 2, "Kept")
	request := oneOffFor(card, firstLessonDate, firstLessonLocal).until([]int{1}, mondaysUntilDecember)
	request.AllowOverlap, request.MaterialIDs = true, []string{removed.ID}
	created := h.scheduled(t, token, request)
	h.mustAnswer(t, h.deleteMaterial(t, token, removed.ID), http.StatusNoContent)
	h.at(t, authStart.Add(24*time.Hour))
	token = h.tutorToken(t)
	release := h.holdRowThen(t, "SELECT "+lockedRowPosition+" FROM lesson_series WHERE id = $1::uuid FOR UPDATE", created.Series.ID, "")

	ticked := h.tickInBackground(t)
	edited := h.sendInBackground(http.MethodPatch, editPath(created.Lessons[0].ID),
		lessonPatch{Scope: "following", Version: 1, MaterialIDs: &[]string{kept.ID}}.body(t), jsonCall(token))
	h.waitForLockWaiters(t, 2)
	release()
	awaitTick(t, ticked)
	split := awaitAnswer(t, edited)

	h.mustAnswer(t, split, http.StatusOK)
	references := h.countRows(t, "SELECT count(ref_id) FROM attachment_references WHERE attachment_id = $1::uuid", removed.AttachmentID)
	if references != 0 || h.storageDeletionsOf(t, removed.AttachmentID) != 1 {
		t.Fatalf("the removed file keeps %d references with %d deletions queued", references, h.storageDeletionsOf(t, removed.AttachmentID))
	}
	if stale := h.countRows(t, "SELECT count(lesson_id) FROM lesson_materials WHERE material_id = $1::uuid", removed.ID); stale != 0 {
		t.Fatalf("%d lessons still show the removed material", stale)
	}
	if h.storageDeletionsOf(t, kept.AttachmentID) != 0 || h.attachmentStateOf(t, kept.AttachmentID) != string(domain.AttachmentBound) {
		t.Fatal("the kept file was queued for deletion")
	}
}

func TestLessonMaterialsVisibility(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.visibilityParties(t)
	token := parties.tutorToken
	second, secondToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
	h.shareStudentWith(t, second, parties.student)
	elsewhere := h.linkedCard(t, parties.tutor, 12, domain.RelationshipActive)
	h.newGroup(t, token, "Another group", []string{elsewhere})
	provisionalAccount, provisional := h.squattedProvisional(t, token, 30)
	provisionalToken := accessToken(t, h.login(t, provisionalAccount.email, provisionalAccount.password))
	material := h.libraryMaterial(t, parties.tutor, token, 1, lessonMaterialTitle)
	oneToOne := h.scheduled(t, token, oneOffFor(parties.card, "2030-03-12", "18:00")).Lessons[0]
	group := h.scheduled(t, token, groupLessonFor(parties.group, "2030-03-13", "18:00")).Lessons[0]
	cancelledGroup := h.scheduled(t, token, groupLessonFor(parties.group, "2030-03-14", "18:00")).Lessons[0]
	ofProvisional := h.scheduled(t, token, oneOffFor(provisional, "2030-03-15", "18:00")).Lessons[0]
	h.cancelledThis(t, token, cancelledGroup)
	for _, lesson := range []string{oneToOne.ID, group.ID, ofProvisional.ID} {
		h.attached(t, token, lesson, 1, material.ID)
	}
	h.attached(t, token, cancelledGroup.ID, 2, material.ID)
	if answer := h.groupCall(t, token, http.MethodPatch, parties.group, "", `{"remove":`+idList([]string{parties.other})+`}`); answer.status != http.StatusOK {
		t.Fatalf("removing the member answered %d %s", answer.status, answer.body)
	}
	elsewhereToken := h.studentToken(t, 12)
	otherToken := h.studentToken(t, 11)

	type visibility struct {
		token  string
		lesson string
	}
	granted := map[string]visibility{
		"the student's one-to-one lesson":            {parties.studentToken, oneToOne.ID},
		"a current member's group lesson":            {parties.studentToken, group.ID},
		"a removed member's cancelled roster lesson": {otherToken, cancelledGroup.ID},
		"a provisional student's own lesson":         {provisionalToken, ofProvisional.ID},
	}
	refused := map[string]visibility{
		"a second tutor sharing the student":         {secondToken, oneToOne.ID},
		"a removed member's current group lesson":    {otherToken, group.ID},
		"another group's student":                    {elsewhereToken, group.ID},
		"another student's one-to-one lesson":        {otherToken, oneToOne.ID},
		"a provisional student on another's lesson":  {provisionalToken, oneToOne.ID},
		"a student of the tutor on another's lesson": {elsewhereToken, ofProvisional.ID},
	}
	random, _ := h.addressOf(t, parties.studentToken, fixtureID(70, 99), "?ref=lesson:"+oneToOne.ID)

	for name, seen := range granted {
		page := h.lessonPage(t, seen.token, seen.lesson)
		minted, _ := h.addressOf(t, seen.token, material.AttachmentID, "?ref=lesson:"+seen.lesson)
		if page.status != http.StatusOK || len(decodeLesson(t, page).Materials) != 1 || minted.status != http.StatusOK {
			t.Errorf("%s: the page answered %s and minting %s", name, describe(page), describe(minted))
		}
	}
	for name, hidden := range refused {
		minted, _ := h.addressOf(t, hidden.token, material.AttachmentID, "?ref=lesson:"+hidden.lesson)
		if minted.status != http.StatusNotFound || refusal(t, minted) != refusal(t, random) {
			t.Errorf("%s: minting answered %s", name, describe(minted))
		}
	}
	tutorMint, _ := h.addressOf(t, token, material.AttachmentID, "?ref=lesson:"+group.ID)
	if tutorMint.status != http.StatusOK {
		t.Fatalf("the tutor's mint answered %s", describe(tutorMint))
	}
}

func TestLessonMaterialFramesReachTheAudienceAndRoster(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.visibilityParties(t)
	token := parties.tutorToken
	material := h.libraryMaterial(t, parties.tutor, token, 1, "Worksheet")
	scheduled := h.scheduled(t, token, groupLessonFor(parties.group, "2030-03-13", "18:00")).Lessons[0]
	cancelled := h.scheduled(t, token, groupLessonFor(parties.group, "2030-03-14", "18:00")).Lessons[0]
	h.cancelledThis(t, token, cancelled)
	if answer := h.groupCall(t, token, http.MethodPatch, parties.group, "", `{"remove":`+idList([]string{parties.other})+`}`); answer.status != http.StatusOK {
		t.Fatalf("removing the member answered %d %s", answer.status, answer.body)
	}
	frames := h.recordFrames(t)

	h.attached(t, token, scheduled.ID, 1, material.ID)
	h.attached(t, token, cancelled.ID, 2, material.ID)
	sent := frames.settle(t, h)

	for name, frame := range map[string]struct {
		user   domain.UserID
		lesson string
		want   bool
	}{
		"the tutor of the scheduled lesson":           {parties.tutor, scheduled.ID, true},
		"a current member of the scheduled lesson":    {parties.student, scheduled.ID, true},
		"a removed member of the scheduled lesson":    {parties.otherStudent, scheduled.ID, false},
		"the tutor of the cancelled lesson":           {parties.tutor, cancelled.ID, true},
		"a roster student of the cancelled lesson":    {parties.otherStudent, cancelled.ID, true},
		"another roster student of the cancelled one": {parties.student, cancelled.ID, true},
	} {
		if hasFrame(sent, frame.user, domain.EntityLesson, frame.lesson) != frame.want {
			t.Errorf("%s: a lesson frame arrived %v, want %v", name, !frame.want, frame.want)
		}
	}
}

func TestLessonMaterialRecordsCarryNoContent(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.visibilityParties(t)
	token := parties.tutorToken
	picture := h.pendingLibraryPicture(t, parties.tutor, 72, 1)
	material := h.createMaterial(t, token, picture, lessonMaterialTitle, nil)
	other := h.libraryMaterial(t, parties.tutor, token, 2, lessonMaterialTitle+" two")
	request := oneOffFor(parties.card, firstLessonDate, firstLessonLocal).weekly([]int{1}, 3)
	request.MaterialIDs = []string{material.ID}
	series := h.scheduled(t, token, request)
	view := h.attached(t, token, series.Lessons[1].ID, 1, other.ID, material.ID)
	minted, address := h.addressOf(t, parties.studentToken, material.AttachmentID, "?ref=lesson:"+series.Lessons[1].ID+"&disposition=inline")
	h.mustAnswer(t, h.editLesson(t, token, series.Lessons[0].ID, lessonPatch{Scope: "following", Version: 1, MaterialIDs: &[]string{other.ID}}), http.StatusOK)

	records := h.logs.String()

	if len(view.Materials) != 2 || view.Materials[1].Width == nil || !strings.Contains(view.Materials[1].URL, "?t=") || minted.status != http.StatusOK {
		t.Fatalf("the lesson lists %+v and minting answered %s", view.Materials, describe(minted))
	}
	_, signed, _ := strings.Cut(address.URL, "?t=")
	for _, forbidden := range []string{lessonMaterialTitle, lessonMaterialFilename, "notes.pdf", "/api/v1/files/", signed, view.Materials[0].URL} {
		if strings.Contains(records, forbidden) {
			t.Errorf("a record carries %q", forbidden)
		}
	}
	if h.countLogs(`"msg":"lesson.materials.changed"`, `"lesson_id":"`+series.Lessons[1].ID+`"`, `"count":2`, `"scope":"this"`) != 1 ||
		h.countLogs(`"msg":"file.reference.added"`, `"ref_kind":"series"`) == 0 || h.countLogs(`"msg":"file.reference.added"`, `"ref_kind":"lesson"`) == 0 {
		t.Fatalf("the lesson material events were not recorded:\n%s", records)
	}
}

func TestTutorPurgeRemovesLessonAndSeriesMaterials(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	parties := h.visibilityParties(t)
	token := parties.tutorToken
	items := h.libraryMaterials(t, parties.tutor, token, 2)
	request := oneOffFor(parties.card, firstLessonDate, firstLessonLocal).weekly([]int{1}, 3)
	request.MaterialIDs = materialIDsOfItems(items)
	h.scheduled(t, token, request)
	h.mustAnswer(t, h.deleteMaterial(t, token, items[0].ID), http.StatusNoContent)
	before := h.countRows(t, "SELECT count(lesson_id) FROM lesson_materials WHERE tutor_id = $1::uuid", parties.tutor.String())
	h.mustAnswer(t, h.deleteMe(t, token, accountPassword), http.StatusNoContent)

	h.at(t, h.now(t).Add(domain.DeletedAccountRetention))
	h.tick(t)
	h.tick(t)

	if before != 6 {
		t.Fatalf("the seed holds %d lesson materials", before)
	}
	for table, query := range map[string]string{
		"lesson materials": "SELECT count(lesson_id) FROM lesson_materials WHERE tutor_id = $1::uuid",
		"series materials": "SELECT count(series_id) FROM series_materials WHERE tutor_id = $1::uuid",
		"charges":          "SELECT count(attachment_id) FROM quota_charges WHERE tutor_id = $1::uuid",
		"lessons":          "SELECT count(id) FROM lessons WHERE tutor_id = $1::uuid",
	} {
		if remaining := h.countRows(t, query, parties.tutor.String()); remaining != 0 {
			t.Errorf("the purge left %d %s", remaining, table)
		}
	}
	for _, item := range items {
		if h.countRows(t, "SELECT count(ref_id) FROM attachment_references WHERE attachment_id = $1::uuid", item.AttachmentID) != 0 ||
			h.storageDeletionsOf(t, item.AttachmentID) != 1 {
			t.Errorf("the purge left references or no deletion of %s", item.AttachmentID)
		}
	}
}

func TestSeriesEditKeepsADeletedSeriesMaterialAndCancellingRetiresThem(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.visibilityParties(t)
	token := parties.tutorToken
	items := h.libraryMaterials(t, parties.tutor, token, 2)
	deleted, kept := items[0], items[1]
	request := oneOffFor(parties.card, firstLessonDate, firstLessonLocal).until([]int{1}, mondaysUntilDecember)
	request.AllowOverlap, request.MaterialIDs = true, []string{deleted.ID, kept.ID}
	created := h.scheduled(t, token, request)
	h.mustAnswer(t, h.deleteMaterial(t, token, deleted.ID), http.StatusNoContent)

	reordered := h.editLesson(t, token, created.Lessons[0].ID, lessonPatch{Scope: "following", Version: 1, MaterialIDs: &[]string{kept.ID, deleted.ID}})
	split := decodeChanged(t, reordered)
	cancelled := h.cancelCall(t, token, split.Lessons[1].ID, "following", split.Lessons[1].Version)

	if split.Series == nil || split.Lessons[0].Version != 2 || !slices.Equal(attachedMaterialIDs(split.Lessons[0]), []string{kept.ID, deleted.ID}) {
		t.Fatalf("the reordering edit answered %s", describe(reordered))
	}
	if h.countLogs(`"msg":"lesson.edited"`, `"fields":"materials"`, `"scope":"following"`) != 1 {
		t.Fatal("the edit did not record materials among its fields")
	}
	h.mustAnswer(t, cancelled, http.StatusOK)
	if len(h.seriesMaterialIDs(t, split.Series.ID)) != 0 || h.referencesOfKind(t, deleted.AttachmentID, domain.ReferenceSeries) != 0 ||
		!slices.Equal(h.lessonMaterialIDs(t, split.Lessons[2].ID), []string{kept.ID, deleted.ID}) {
		t.Fatalf("cancelling following left series materials %v", h.seriesMaterialIDs(t, split.Series.ID))
	}
	if h.storageDeletionsOf(t, deleted.AttachmentID) != 0 || h.fileUsage(t, parties.tutor) != 2000 {
		t.Fatal("a file still shown by lessons was released")
	}
}

func TestFinishedSeriesReleasesItsMaterials(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.visibilityParties(t)
	token := parties.tutorToken
	material := h.libraryMaterial(t, parties.tutor, token, 1, "Worksheet")
	request := oneOffFor(parties.card, firstLessonDate, firstLessonLocal).weekly([]int{1}, 2)
	request.MaterialIDs = []string{material.ID}
	created := h.scheduled(t, token, request)
	h.mustAnswer(t, h.deleteMaterial(t, token, material.ID), http.StatusNoContent)
	for _, lesson := range created.Lessons {
		h.attached(t, token, lesson.ID, lesson.Version)
	}
	usage, deletions := h.fileUsage(t, parties.tutor), h.storageDeletionsOf(t, material.AttachmentID)

	h.tick(t)

	if usage != 1000 || deletions != 0 {
		t.Fatalf("before the run the usage is %d with %d deletions", usage, deletions)
	}
	if len(h.seriesMaterialIDs(t, created.Series.ID)) != 0 || h.fileUsage(t, parties.tutor) != 0 || h.storageDeletionsOf(t, material.AttachmentID) != 1 {
		t.Fatalf("the finished series kept its materials: usage %d", h.fileUsage(t, parties.tutor))
	}
	if h.countLogs(`"msg":"series.extended"`, `"retired":1`) != 1 {
		t.Fatal("the retirement was not recorded")
	}
}

func TestSeriesWithoutAnOccurrenceBeforeItsEndReleasesItsMaterials(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.visibilityParties(t)
	token := parties.tutorToken
	material := h.libraryMaterial(t, parties.tutor, token, 1, "Worksheet")
	request := oneOffFor(parties.card, firstLessonDate, firstLessonLocal).until([]int{1}, "2030-06-09")
	request.AllowOverlap, request.MaterialIDs = true, []string{material.ID}
	created := h.scheduled(t, token, request)
	before := h.seriesMaterialIDs(t, created.Series.ID)

	h.tickAt(t, authStart.Add(24*time.Hour))

	if len(before) != 1 || len(h.seriesLessons(t, created.Series.ID)) != 13 || h.materialisedUntil(t, created.Series.ID) == "2030-06-09" {
		t.Fatalf("the series held %v and was extended to %s", before, h.materialisedUntil(t, created.Series.ID))
	}
	if len(h.seriesMaterialIDs(t, created.Series.ID)) != 0 || h.referencesOfKind(t, material.AttachmentID, domain.ReferenceSeries) != 0 ||
		h.referencesOfKind(t, material.AttachmentID, domain.ReferenceLesson) != 13 {
		t.Fatalf("the finished series kept %v", h.seriesMaterialIDs(t, created.Series.ID))
	}
}

func TestEditOfAFinishedSeriesComparesTheLessonsMaterials(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	parties := h.visibilityParties(t)
	token := parties.tutorToken
	items := h.libraryMaterials(t, parties.tutor, token, 2)
	request := oneOffFor(parties.card, firstLessonDate, firstLessonLocal).weekly([]int{1}, 3)
	request.MaterialIDs = materialIDsOfItems(items)
	created := h.scheduled(t, token, request)
	h.mustAnswer(t, h.deleteMaterial(t, token, items[0].ID), http.StatusNoContent)
	h.tick(t)
	token = h.tutorToken(t)

	echoed := h.editLesson(t, token, created.Lessons[0].ID, lessonPatch{Scope: "following", Version: 1, MaterialIDs: &[]string{items[0].ID, items[1].ID}})

	unchanged := decodeChanged(t, echoed)
	if unchanged.Series == nil || unchanged.Series.ID != created.Series.ID || h.storedLesson(t, created.Lessons[0].ID).Version != 1 {
		t.Fatalf("echoing a finished series' materials answered %s", describe(echoed))
	}
}
