package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"
	"testing"

	"ovc/backend/internal/domain"
)

const contractGroup = 73

var (
	sentinelPoints  = []string{"97", "89", "83", "79", "73", "71", "492", "180"}
	sentinelStrings = []string{"sentinel-accepted-answer", "4321.12345", "0.000917", "sentinel-explanation"}
	answerFileKeys  = []string{"attachment_id", "filename", "media_type", "size_bytes", "width", "height", "url", "expires_at"}
	studentFileKeys = []string{"filename", "media_type", "size_bytes", "width", "height", "url", "expires_at"}
	submissionKeys  = []string{
		"id", "title", "tutor", "status", "late", "issued_before_reclaim", "deadline_at", "version", "blocks", "answers", "editable_question_ids",
		"review",
	}
	assignmentItemKeys = []string{
		"submission_id", "title", "tutor", "status", "late", "issued_before_reclaim", "deadline_at", "issued_at", "submitted_at", "checked_at",
		"feedback_excerpt", "has_corrected_files", "feedback_new",
	}
	tutorRefKeys     = []string{"relationship_id", "name", "colour"}
	notificationKeys = []string{"id", "event", "created_at", "read_at", "label", "label_deleted", "label_kind", "colour", "link"}
	blockKeysByType  = map[string][]string{
		"instruction": {"id", "type", "text"}, "material": {"id", "type", "material"},
		"single_choice": {"id", "type", "prompt", "images", "options"}, "multiple_choice": {"id", "type", "prompt", "images", "options"},
		"short_text": {"id", "type", "prompt", "images"}, "short_number": {"id", "type", "prompt", "images"},
		"long_text": {"id", "type", "prompt", "images", "max_length"}, "file": {"id", "type", "prompt", "images", "allowed_kinds", "max_files"},
	}
	revealedKeysByQuestion = map[string][]string{
		contractQuestion(2): {"question_id", "correct_option_ids", "explanation"}, contractQuestion(3): {"question_id", "correct_option_ids", "explanation"},
		contractQuestion(4): {"question_id", "accepted_answers", "explanation"},
		contractQuestion(5): {"question_id", "correct_value", "tolerance", "explanation"},
		contractQuestion(6): {"question_id", "explanation"}, contractQuestion(7): {"question_id", "explanation"},
	}
)

func contractQuestion(n byte) string {
	return blockID(contractGroup, n)
}

func contractOption(n byte) string {
	return optionID(contractGroup, n)
}

func contractBlocks(image string, material string) []templateBlockJSON {
	explanation := func(kind string) *string { return text("sentinel-explanation-" + kind) }
	single := []templateOptionJSON{{ID: contractOption(1), Text: "Paris", Correct: flag(true)}, {ID: contractOption(2), Text: "Rome", Correct: flag(false)}}
	multiple := []templateOptionJSON{
		{ID: contractOption(3), Text: "Red", Correct: flag(true)}, {ID: contractOption(4), Text: "Blue", Correct: flag(false)},
	}
	return []templateBlockJSON{
		instructionBlock(contractQuestion(1), "Read first"),
		{ID: contractQuestion(2), Type: string(domain.BlockSingleChoice), Prompt: text("Capital"), ImageIDs: textList(image), Options: &single,
			Points: points(97), Explanation: explanation("single")},
		{ID: contractQuestion(3), Type: string(domain.BlockMultipleChoice), Prompt: text("Colours"), Options: &multiple,
			Scoring: text(string(domain.ScoringPartial)), Points: points(89), Explanation: explanation("multiple")},
		{ID: contractQuestion(4), Type: string(domain.BlockShortText), Prompt: text("Word"), AcceptedAnswers: textList("sentinel-accepted-answer"),
			Points: points(83), Explanation: explanation("text")},
		{ID: contractQuestion(5), Type: string(domain.BlockShortNumber), Prompt: text("Number"), Value: text("4321.12345"), Tolerance: text("0.000917"),
			Points: points(79), Explanation: explanation("number")},
		{ID: contractQuestion(6), Type: string(domain.BlockLongText), Prompt: text("Essay"), MaxLength: points(500), Points: points(73),
			Explanation: explanation("long")},
		{ID: contractQuestion(7), Type: string(domain.BlockFile), Prompt: text("Photo"), AllowedKinds: textList(string(domain.AnswerFilePDF)),
			MaxFiles: points(2), Points: points(71), Explanation: explanation("file")},
		materialBlock(contractQuestion(8), material),
	}
}

func contractAutomaticBlocks() []templateBlockJSON {
	blocks := contractBlocks("", "")
	automatic := []templateBlockJSON{blocks[1], blocks[3]}
	automatic[0].ImageIDs = nil
	return automatic
}

type contractCheck struct {
	t *testing.T
}

func (c contractCheck) object(label string, raw json.RawMessage, allowed []string) map[string]json.RawMessage {
	c.t.Helper()
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(raw, &decoded); err != nil {
		c.t.Fatalf("%s is not an object: %s", label, raw)
	}
	keys := make([]string, 0, len(decoded))
	for key := range decoded {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	expected := slices.Clone(allowed)
	sort.Strings(expected)
	if !slices.Equal(keys, expected) {
		c.t.Errorf("%s has the keys %v, want %v", label, keys, expected)
	}
	return decoded
}

func (c contractCheck) array(label string, raw json.RawMessage) []json.RawMessage {
	c.t.Helper()
	var decoded []json.RawMessage
	if err := json.Unmarshal(raw, &decoded); err != nil {
		c.t.Fatalf("%s is not an array: %s", label, raw)
	}
	return decoded
}

func (c contractCheck) noSentinel(label string, raw []byte) {
	c.t.Helper()
	for _, sentinel := range sentinelStrings {
		if bytes.Contains(raw, []byte(sentinel)) {
			c.t.Errorf("%s carries the key sentinel %q", label, sentinel)
		}
	}
	c.noSentinelNumber(label, json.RawMessage(raw))
}

func (c contractCheck) noSentinelInText(label string, text string) {
	c.t.Helper()
	for _, sentinel := range sentinelStrings {
		if strings.Contains(text, sentinel) {
			c.t.Errorf("%s carries the key sentinel %q", label, sentinel)
		}
	}
	for _, found := range pointSentinelsIn(text) {
		c.t.Errorf("%s carries the point sentinel %s", label, found)
	}
}

func pointSentinelsIn(text string) []string {
	var found []string
	for _, word := range strings.FieldsFunc(text, isOutsideSentinelWord) {
		if slices.Contains(sentinelPoints, word) {
			found = append(found, word)
		}
	}
	return found
}

const sentinelWordRunes = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ_-"

func isOutsideSentinelWord(r rune) bool {
	return !strings.ContainsRune(sentinelWordRunes, r)
}

func TestPointSentinelTextScan(t *testing.T) {
	t.Parallel()
	caught := []string{"Score: 492", "492", "<b>180</b>", "Points 97.", "(71)"}
	ignored := []string{"14920", "a492b", "Q492z_", "id 0f492a3c-4920-4b92-8492-c0ffee492000", "#a49210", "order-97", "1971"}

	var missed, flagged []string
	for _, text := range caught {
		if len(pointSentinelsIn(text)) != 1 {
			missed = append(missed, text)
		}
	}
	for _, text := range ignored {
		if len(pointSentinelsIn(text)) != 0 {
			flagged = append(flagged, text)
		}
	}

	if len(missed) > 0 || len(flagged) > 0 {
		t.Fatalf("the point sentinel scan missed %q and flagged %q", missed, flagged)
	}
}

func (c contractCheck) noSentinelNumber(label string, raw json.RawMessage) {
	c.t.Helper()
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return
	}
	switch trimmed[0] {
	case '{':
		var decoded map[string]json.RawMessage
		if json.Unmarshal(trimmed, &decoded) == nil {
			for key, value := range decoded {
				c.noSentinelNumber(label+"."+key, value)
			}
		}
	case '[':
		var decoded []json.RawMessage
		if json.Unmarshal(trimmed, &decoded) == nil {
			for _, value := range decoded {
				c.noSentinelNumber(label, value)
			}
		}
	default:
		var number json.Number
		if json.Unmarshal(trimmed, &number) == nil && slices.Contains(sentinelPoints, number.String()) {
			c.t.Errorf("%s carries the point sentinel %s", label, number)
		}
	}
}

func (c contractCheck) submission(label string, raw []byte, revealed bool) {
	c.t.Helper()
	allowed := slices.Clone(submissionKeys)
	if revealed {
		allowed = append(allowed, "revealed")
	}
	decoded := c.object(label, raw, allowed)
	c.object(label+".tutor", decoded["tutor"], tutorRefKeys)
	for _, block := range c.array(label+".blocks", decoded["blocks"]) {
		var typed struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(block, &typed); err != nil {
			c.t.Fatal(err)
		}
		fields := c.object(label+".blocks."+typed.Type, block, blockKeysByType[typed.Type])
		if options, found := fields["options"]; found {
			for _, option := range c.array(label+".options", options) {
				c.object(label+".option", option, []string{"id", "text"})
			}
		}
		if images, found := fields["images"]; found {
			for _, image := range c.array(label+".images", images) {
				c.object(label+".image", image, studentFileKeys)
			}
		}
		if material, found := fields["material"]; found {
			c.object(label+".material", material, append([]string{"title"}, studentFileKeys...))
		}
	}
	for _, answer := range c.array(label+".answers", decoded["answers"]) {
		fields := c.object(label+".answer", answer, []string{"question_id", "value", "files"})
		value := c.objectKeys(label+".answer.value", fields["value"])
		if len(value) != 1 || !slices.Contains([]string{"option_ids", "text", "attachment_ids"}, value[0]) {
			c.t.Errorf("%s holds the answer value keys %v", label, value)
		}
		for _, file := range c.array(label+".files", fields["files"]) {
			c.object(label+".file", file, answerFileKeys)
		}
	}
	if string(decoded["review"]) != "null" {
		review := c.object(label+".review", decoded["review"], []string{"feedback", "items"})
		for _, item := range c.array(label+".review.items", review["items"]) {
			fields := c.object(label+".review.item", item, []string{"question_id", "comment", "corrected_files"})
			for _, file := range c.array(label+".review.files", fields["corrected_files"]) {
				c.object(label+".review.file", file, answerFileKeys)
			}
		}
	}
	if revealed {
		questions := c.object(label+".revealed", decoded["revealed"], []string{"questions"})
		for _, question := range c.array(label+".revealed.questions", questions["questions"]) {
			var identified struct {
				QuestionID string `json:"question_id"`
			}
			if err := json.Unmarshal(question, &identified); err != nil {
				c.t.Fatal(err)
			}
			c.object(label+".revealed."+identified.QuestionID, question, revealedKeysByQuestion[identified.QuestionID])
		}
		c.noSentinelNumber(label+".revealed", decoded["revealed"])
		delete(decoded, "revealed")
	}
	stripped, err := json.Marshal(decoded)
	if err != nil {
		c.t.Fatal(err)
	}
	c.noSentinel(label, stripped)
}

func (c contractCheck) objectKeys(label string, raw json.RawMessage) []string {
	c.t.Helper()
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(raw, &decoded); err != nil {
		c.t.Fatalf("%s is not an object: %s", label, raw)
	}
	keys := make([]string, 0, len(decoded))
	for key := range decoded {
		keys = append(keys, key)
	}
	return keys
}

func (c contractCheck) assignments(label string, raw []byte) {
	c.t.Helper()
	decoded := c.object(label, raw, []string{"items", "next_cursor", "counts"})
	c.object(label+".counts", decoded["counts"], []string{"todo", "closed"})
	for _, item := range c.array(label+".items", decoded["items"]) {
		fields := c.object(label+".item", item, assignmentItemKeys)
		c.object(label+".item.tutor", fields["tutor"], tutorRefKeys)
	}
	c.noSentinel(label, raw)
}

func (c contractCheck) refusal(label string, raw []byte) {
	c.t.Helper()
	decoded := c.object(label, raw, []string{"error"})
	c.object(label+".error", decoded["error"], []string{"code", "message", "request_id"})
	c.noSentinel(label, raw)
}

func (c contractCheck) fieldRefusal(label string, raw []byte) {
	c.t.Helper()
	decoded := c.object(label, raw, []string{"error"})
	body := c.object(label+".error", decoded["error"], []string{"code", "message", "request_id", "fields"})
	for _, field := range c.array(label+".fields", body["fields"]) {
		c.object(label+".field", field, []string{"path", "code"})
	}
	c.noSentinel(label, raw)
}

type contractWork struct {
	full      string
	automatic string
	closing   string
	lesson    string
}

var studentLessonKeys = []string{
	"id", "title", "description", "starts_at", "ends_at", "duration_minutes", "status", "took_place", "timezone", "series_id", "series", "version",
	"cancelled_at", "other_timezone", "audience", "tutor", "channel_id", "room_open", "embed_allowed", "materials", "issues",
}

func (h *authHarness) contractWork(t *testing.T, parties endingParties, policy string, full string, automatic string, start string) contractWork {
	t.Helper()
	card := toCard(parties.card.ID)
	lesson := h.scheduled(t, parties.tutorToken, oneOffFor(parties.card.ID, "2030-03-12", start)).Lessons[0].ID
	fullWork := h.issued(t, parties.tutorToken, issueJSONWith(full, card, &lesson, nil, policy))
	automaticWork := h.issued(t, parties.tutorToken, issueJSONWith(automatic, card, &lesson, nil, policy))
	closingWork := h.issued(t, parties.tutorToken, issueJSONWith(automatic, card, &lesson, nil, policy))
	return contractWork{
		full: h.submissionsOf(t, fullWork.ID)[0].id, automatic: h.submissionsOf(t, automaticWork.ID)[0].id, closing: h.submissionsOf(t, closingWork.ID)[0].id,
		lesson: lesson,
	}
}

func (c contractCheck) lessonWork(label string, raw []byte) []string {
	c.t.Helper()
	decoded := c.object(label, raw, studentLessonKeys)
	items := c.array(label+".issues", decoded["issues"])
	if len(items) != 3 {
		c.t.Errorf("%s lists %d linked submissions, want 3", label, len(items))
	}
	statuses := make([]string, 0, len(items))
	for _, item := range items {
		fields := c.object(label+".issue", item, []string{"submission_id", "title", "deadline_at", "status", "late"})
		var status string
		if err := json.Unmarshal(fields["status"], &status); err != nil {
			c.t.Fatalf("%s carries a status that is not a string: %s", label, item)
		}
		statuses = append(statuses, status)
	}
	c.noSentinel(label, raw)
	return statuses
}

func observeStatuses(seen map[string]bool, statuses []string) {
	for _, status := range statuses {
		seen[status] = true
	}
}

func TestAnswerKeyContract(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	check := contractCheck{t: t}
	parties := h.activeStudentInAGroup(t)
	picture := h.libraryUpload(t, parties.tutorToken, domain.ContextPromptImage, "prompt.png", pngBytes(t, 2, 2))
	h.mustAnswer(t, picture, http.StatusCreated)
	worksheet := h.libraryUpload(t, parties.tutorToken, domain.ContextLibrary, "worksheet.pdf", pdfFile(100))
	h.mustAnswer(t, worksheet, http.StatusCreated)
	material := h.createMaterial(t, parties.tutorToken, uploadedID(t, worksheet), "Worksheet", nil).ID
	policies := []string{string(domain.AnswerPolicyNever), string(domain.AnswerPolicyAfterSubmission), string(domain.AnswerPolicyAfterCheck)}
	full := h.workTemplate(t, parties.tutorToken, "Contract", contractBlocks(uploadedID(t, picture), material)...)
	automatic := h.workTemplate(t, parties.tutorToken, "Automatic", contractAutomaticBlocks()...)
	works := map[string]contractWork{}
	for index, policy := range policies {
		works[policy] = h.contractWork(t, parties, policy, full, automatic, fmt.Sprintf("%02d:00", 10+index))
	}
	frames := h.recordFrames(t)
	seen := map[string]bool{}

	for _, policy := range policies {
		work := works[policy]
		label := policy + " "
		check.submission(label+"not_started", []byte(h.submissionAnswer(t, parties.studentToken, work.full).body), false)
		observeStatuses(seen, check.lessonWork(label+"lesson not_started", []byte(h.lessonPage(t, parties.studentToken, work.lesson).body)))
		upload := h.homeworkUpload(t, parties.studentToken, homeworkRoute(domain.ContextAnswer, work.full, contractQuestion(7)), "answer.pdf", pdfFile(300))
		h.mustAnswer(t, upload, http.StatusCreated)
		check.object(label+"upload", json.RawMessage(upload.body), []string{"id", "filename", "media_type", "size_bytes"})
		options := h.workOptions(t, parties.studentToken, work.full)
		draft := h.saveAnswers(t, parties.studentToken, work.full, answersJSON(t, 1, saveID(1),
			chosen(contractQuestion(2), options[contractQuestion(2)][1]), typedAnswer(contractQuestion(4), "a guess"), typedAnswer(contractQuestion(5), "12"),
			typedAnswer(contractQuestion(6), "An essay"), attached(contractQuestion(7), uploadedID(t, upload))))
		h.mustAnswer(t, draft, http.StatusOK)
		check.submission(label+"draft save", []byte(draft.body), false)
		check.submission(label+"draft", []byte(h.submissionAnswer(t, parties.studentToken, work.full).body), false)
		observeStatuses(seen, check.lessonWork(label+"lesson draft", []byte(h.lessonPage(t, parties.studentToken, work.lesson).body)))
		minted, _ := h.addressOf(t, parties.studentToken, uploadedID(t, upload), "?ref=answer:"+work.full)
		h.mustAnswer(t, minted, http.StatusOK)
		check.object(label+"mint", json.RawMessage(minted.body), []string{"url", "expires_at"})
		stale := h.saveAnswers(t, parties.studentToken, work.full, answersJSON(t, 1, saveID(2), typedAnswer(contractQuestion(6), "Late")))
		check.refusal(label+"conflict", []byte(stale.body))
		frozenLater := h.submitWork(t, parties.studentToken, work.full, 2)
		h.mustAnswer(t, frozenLater, http.StatusOK)
		afterSubmission := policy == string(domain.AnswerPolicyAfterSubmission)
		check.submission(label+"submit", []byte(frozenLater.body), afterSubmission)
		check.submission(label+"submitted", []byte(h.submissionAnswer(t, parties.studentToken, work.full).body), afterSubmission)
		observeStatuses(seen, check.lessonWork(label+"lesson submitted", []byte(h.lessonPage(t, parties.studentToken, work.lesson).body)))
		published := h.contractReview(t, check, parties, work, label, afterSubmission)
		observeStatuses(seen, check.lessonWork(label+"lesson after the review", []byte(h.lessonPage(t, parties.studentToken, work.lesson).body)))
		checked := h.submitWork(t, parties.studentToken, work.automatic, 1)
		h.mustAnswer(t, checked, http.StatusOK)
		check.submission(label+"checked", []byte(checked.body), policy != string(domain.AnswerPolicyNever))
		check.assignments(label+"assignments", []byte(h.assignmentsAnswer(t, parties.studentToken, nil).body))
		home := h.homeAnswer(t, parties.studentToken, "")
		check.studentHome(label+"home", []byte(home.body))
		if listed := decodedAs[struct {
			RecentlyChecked []json.RawMessage `json:"recently_checked"`
		}](t, json.RawMessage(home.body)); len(listed.RecentlyChecked) == 0 {
			t.Errorf("%shome lists no checked work", label)
		}
		check.object(label+"tutor home", json.RawMessage(h.homeAnswer(t, parties.tutorToken, "").body), tutorHomeKeys)
		check.studentHome(label+"filtered home", []byte(h.homeAnswer(t, parties.studentToken, homeQuery(parties.card.ID)).body))
		observeStatuses(seen, check.lessonWork(label+"lesson checked", []byte(h.lessonPage(t, parties.studentToken, work.lesson).body)))
		snapshotMint, _ := h.addressOf(t, parties.studentToken, uploadedID(t, picture), "?ref=issue_snapshot:"+issueOf(t, h, work.full))
		h.mustAnswer(t, snapshotMint, http.StatusOK)
		check.object(label+"snapshot mint", json.RawMessage(snapshotMint.body), []string{"url", "expires_at"})
		notEditable := h.saveAnswers(t, parties.studentToken, work.automatic, answersJSON(t, 2, saveID(3), typedAnswer(contractQuestion(4), "x")))
		assertRefused(t, label+"a save of checked work", notEditable, http.StatusConflict, "not_editable")
		check.refusal(label+"not_editable", []byte(notEditable.body))
		h.contractOverridesOnly(t, check, parties, work, label, policy != string(domain.AnswerPolicyNever))
		if published.returnedVersion != 0 {
			frozen := h.saveAnswers(t, parties.studentToken, work.full, answersJSON(t, published.returnedVersion, saveID(4), typedAnswer(contractQuestion(4), "x")))
			assertRefused(t, label+"a save of a frozen answer", frozen, http.StatusConflict, "answer_frozen")
			check.refusal(label+"answer_frozen", []byte(frozen.body))
			unknown := h.saveAnswers(t, parties.studentToken, work.full, answersJSON(t, published.returnedVersion, saveID(5), typedAnswer(blockID(75, 1), "x")))
			check.fieldRefusal(label+"validation", []byte(unknown.body))
			h.contractRecheck(t, check, parties, work, label, policy != string(domain.AnswerPolicyNever), published)
		}
		observeStatuses(seen, check.lessonWork(label+"lesson reviewed", []byte(h.lessonPage(t, parties.studentToken, work.lesson).body)))
		h.cancelWithRoster(t, work.lesson, parties.student)
		missing := h.submissionAnswer(t, parties.studentToken, randomSubmissionID)
		check.refusal(label+"not_found", []byte(missing.body))
	}
	h.contractLimitRefusals(t, check, parties, works[string(domain.AnswerPolicyNever)].closing, full)
	movedTo := h.scheduled(t, parties.tutorToken, oneOffFor(parties.card.ID, "2030-03-20", "10:00")).Lessons[0].ID
	movedIssue := issueOf(t, h, works[string(domain.AnswerPolicyNever)].closing)
	h.mustAnswer(t, h.patchIssue(t, parties.tutorToken, movedIssue, `{"lesson_id":"`+movedTo+`","version":1}`), http.StatusOK)
	h.mustAnswer(t, h.patchIssue(t, parties.tutorToken, movedIssue, `{"lesson_id":"`+works[string(domain.AnswerPolicyNever)].lesson+`","version":2}`), http.StatusOK)
	h.mustAnswer(t, h.endAsTutor(t, parties.tutorToken, parties.card.ID), http.StatusOK)
	h.tick(t)

	for _, policy := range policies {
		check.submission(policy+" closed", []byte(h.submissionAnswer(t, parties.studentToken, works[policy].closing).body), false)
		check.assignments(policy+" closed list", []byte(h.assignmentsAnswer(t, parties.studentToken, map[string][]string{"status": {"closed"}}).body))
		check.studentHome(policy+" home after the ending", []byte(h.homeAnswer(t, parties.studentToken, "").body))
		observeStatuses(seen, check.lessonWork(policy+" lesson closed", []byte(h.lessonPage(t, parties.studentToken, works[policy].lesson).body)))
	}
	for _, status := range []string{"not_started", "draft", "submitted", "returned", "checked", "closed"} {
		if !seen[status] {
			t.Errorf("the lesson detail's linked work never showed the status %s: %v", status, seen)
		}
	}
	result, page := h.notificationPage(t, parties.studentToken, "?limit=100")
	h.mustAnswer(t, result, http.StatusOK)
	if len(page.Items) == 0 {
		t.Fatal("the student got no notification rows")
	}
	var rows struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal([]byte(result.body), &rows); err != nil {
		t.Fatal(err)
	}
	for _, row := range rows.Items {
		fields := check.object("notification row", row, notificationKeys)
		if string(fields["link"]) != "null" {
			check.object("notification link", fields["link"], []string{"kind", "id"})
		}
	}
	check.noSentinel("notification rows", []byte(result.body))
	scanned := map[string]bool{}
	for _, message := range h.mailTo(t, studentEmail) {
		check.noSentinelInText("email "+message.Subject, message.Subject+"\n"+message.Text+"\n"+message.HTML)
		for _, subject := range []string{"Assignment returned", "Review updated", "Assignment checked"} {
			scanned[subject] = scanned[subject] || strings.HasPrefix(message.Subject, subject)
		}
	}
	if len(scanned) != 3 || !scanned["Assignment returned"] || !scanned["Review updated"] || !scanned["Assignment checked"] {
		t.Fatalf("the sentinel scan missed a results email: %v", scanned)
	}
	settled := frames.settle(t, h)
	visible := []string{movedTo}
	for _, work := range works {
		visible = append(visible, work.lesson)
	}
	checkStudentHomeworkFrames(t, settled, parties.student, visible...)
	checkStudentFrameKeys(t, settled, parties.student)
	namedBoth := false
	for _, frame := range homeworkFramesTo(settled, parties.student) {
		namedBoth = namedBoth || frame.lessons == lessonList(works[string(domain.AnswerPolicyNever)].lesson, movedTo)
	}
	if !namedBoth {
		t.Error("no frame to the student named both the previous and the new lesson")
	}
}

func TestStudentDownloadsFollowReadability(t *testing.T) {
	t.Parallel()
	h := newUploadHarness(t)
	tutor, tutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	picture := h.libraryUpload(t, tutorToken, domain.ContextPromptImage, "prompt.png", pngBytes(t, 2, 2))
	h.mustAnswer(t, picture, http.StatusCreated)
	image := uploadedID(t, picture)
	blocks := []templateBlockJSON{imageQuestion(blockID(74, 1), image), fileBlock(blockID(74, 2), string(domain.AnswerFilePDF))}
	type readabilityParty struct {
		id         byte
		card       string
		issue      string
		submission string
		file       string
	}
	parties := map[string]*readabilityParty{
		"readable": {id: 10}, "reclaim_pending": {id: 11}, "discarded": {id: 12}, "withdrawn": {id: 13}, "ended": {id: 14}, "marked": {id: 15},
	}
	template := h.workTemplate(t, tutorToken, "Pictured", blocks...)
	for _, party := range parties {
		party.card = h.linkedCard(t, tutor, party.id, domain.RelationshipActive)
		issue, submissions := h.issueTemplate(t, tutorToken, template, toCard(party.card), string(domain.AnswerPolicyNever), nil)
		party.issue, party.submission = issue, submissions[0].id
		token := h.studentToken(t, party.id)
		party.file = h.answerFile(t, token, party.submission, blockID(74, 2), "answer.pdf", 64)
		h.saved(t, token, party.submission, answersJSON(t, 1, saveID(1), attached(blockID(74, 2), party.file)))
	}
	pending := parties["reclaim_pending"]
	_, earlyAddress := h.addressOf(t, h.studentToken(t, pending.id), pending.file, "?ref=answer:"+pending.submission)
	readableParty := parties["readable"]
	classmateCard := h.linkedCard(t, tutor, 16, domain.RelationshipActive)
	h.issueTemplate(t, tutorToken, template, toCard(classmateCard), string(domain.AnswerPolicyNever), nil)
	sharedStudent := domain.UserID{readableParty.id}
	secondTutor, secondTutorToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, memberEmail(2))
	h.seedCard(t, secondTutor, 30, domain.RelationshipActive, &sharedStudent)
	crossParty := []crossPartyMint{
		{label: "a classmate's answer mint", token: h.studentToken(t, 16), attachment: readableParty.file, reference: "answer:" + readableParty.submission},
		{label: "a second tutor's answer mint", token: secondTutorToken, attachment: readableParty.file, reference: "answer:" + readableParty.submission},
		{label: "a snapshot mint by a student without a submission", token: h.studentToken(t, 16), attachment: image, reference: "issue_snapshot:" + readableParty.issue},
	}

	h.setRelationshipState(t, pending.card, domain.RelationshipReclaimPending)
	h.setRelationshipState(t, parties["discarded"].card, domain.RelationshipDiscarded)
	h.mustAnswer(t, h.withdrawIssue(t, tutorToken, parties["withdrawn"].issue), http.StatusOK)
	h.setRelationshipState(t, parties["ended"].card, domain.RelationshipEnded)
	h.markIssueForErasure(t, parties["marked"].issue)

	crossAnswers := h.crossPartyMints(t, crossParty)
	for name, party := range parties {
		token := h.studentToken(t, party.id)
		snapshot, _ := h.addressOf(t, token, image, "?ref=issue_snapshot:"+party.issue)
		answer, _ := h.addressOf(t, token, party.file, "?ref=answer:"+party.submission)
		readable := name == "readable" || name == "ended"
		if (snapshot.status == http.StatusOK) != readable || (answer.status == http.StatusOK) != readable {
			t.Errorf("%s: the snapshot mint answered %s and the answer mint %s", name, describe(snapshot), describe(answer))
		}
	}
	tutorAnswer, _ := h.addressOf(t, tutorToken, parties["readable"].file, "?ref=answer:"+parties["readable"].submission)
	h.mustAnswer(t, tutorAnswer, http.StatusOK)
	assertCrossPartyMintsRefused(t, crossAnswers)
	if served := h.fetch(t, earlyAddress.URL, nil); served.status != http.StatusNotFound {
		t.Fatalf("an answer address minted before the reclaim answered %s", describe(served))
	}
}

type crossPartyMint struct {
	label      string
	token      string
	attachment string
	reference  string
}

type crossPartyAnswer struct {
	label  string
	minted response
	random response
}

func (h *authHarness) crossPartyMints(t *testing.T, mints []crossPartyMint) []crossPartyAnswer {
	t.Helper()
	answers := make([]crossPartyAnswer, 0, len(mints))
	for _, mint := range mints {
		minted, _ := h.addressOf(t, mint.token, mint.attachment, "?ref="+mint.reference)
		random, _ := h.addressOf(t, mint.token, fixtureID(79, 1), "?ref="+mint.reference)
		answers = append(answers, crossPartyAnswer{label: mint.label, minted: minted, random: random})
	}
	return answers
}

func assertCrossPartyMintsRefused(t *testing.T, answers []crossPartyAnswer) {
	t.Helper()
	for _, answer := range answers {
		if answer.minted.status != http.StatusNotFound || refusal(t, answer.minted) != refusal(t, answer.random) {
			t.Errorf("%s answered %s, want the refusal of a random identifier %s", answer.label, describe(answer.minted), describe(answer.random))
		}
	}
}

func issueOf(t *testing.T, h *authHarness, submission string) string {
	t.Helper()
	var issue string
	if err := h.database.Pool.QueryRow(t.Context(), "SELECT issue_id::text FROM submissions WHERE id = $1::uuid", submission).Scan(&issue); err != nil {
		t.Fatal(err)
	}
	return issue
}
