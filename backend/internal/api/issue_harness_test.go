package api_test

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/adapters/clock"
	"ovc/backend/internal/adapters/storage"
	"ovc/backend/internal/api"
	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
	"ovc/backend/internal/services/homework"
	"ovc/backend/internal/testdb"
	"ovc/backend/internal/teststorage"
)

const issuesRoute = "/api/v1/issues"

type issueAudienceBody struct {
	RelationshipID *string `json:"relationship_id"`
	DisplayName    *string `json:"display_name"`
	GroupID        *string `json:"group_id"`
	Name           *string `json:"name"`
}

type issueProgressBody struct {
	Students      int64 `json:"students"`
	Submitted     int64 `json:"submitted"`
	Checked       int64 `json:"checked"`
	Returned      int64 `json:"returned"`
	Late          int64 `json:"late"`
	NeedsChecking int64 `json:"needs_checking"`
}

type issueStudentSubmissionBody struct {
	ID                  string `json:"id"`
	Status              string `json:"status"`
	Late                bool   `json:"late"`
	FinalPoints         *int32 `json:"final_points"`
	IssuedBeforeReclaim bool   `json:"issued_before_reclaim"`
}

type issueItemBody struct {
	ID                 string                      `json:"id"`
	Title              string                      `json:"title"`
	Audience           issueAudienceBody           `json:"audience"`
	LessonID           *string                     `json:"lesson_id"`
	DeadlineAt         *time.Time                  `json:"deadline_at"`
	AnswerPolicy       string                      `json:"answer_policy"`
	Status             string                      `json:"status"`
	HasManualQuestions bool                        `json:"has_manual_questions"`
	MaxPoints          int32                       `json:"max_points"`
	Version            int64                       `json:"version"`
	Progress           issueProgressBody           `json:"progress"`
	Submission         *issueStudentSubmissionBody `json:"submission"`
}

type issueSubmissionStudentBody struct {
	RelationshipID string `json:"relationship_id"`
	DisplayName    string `json:"display_name"`
}

type issueSubmissionBody struct {
	ID                  string                     `json:"id"`
	Student             issueSubmissionStudentBody `json:"student"`
	Status              string                     `json:"status"`
	Late                bool                       `json:"late"`
	IssuedBeforeReclaim bool                       `json:"issued_before_reclaim"`
}

type issueDetailBody struct {
	issueItemBody
	Blocks      []templateBlockBody   `json:"blocks"`
	Submissions []issueSubmissionBody `json:"submissions"`
}

type issuePageBody struct {
	Items      []issueItemBody `json:"items"`
	NextCursor *string         `json:"next_cursor"`
}

type issueParties struct {
	tutor      domain.UserID
	tutorToken string
	template   templateBody
}

func toCard(card string) string {
	return `{"relationship_id":"` + card + `"}`
}

func toGroup(group string) string {
	return `{"group_id":"` + group + `"}`
}

func issueJSON(template string, audience string) string {
	return `{"template_id":"` + template + `","audience":` + audience + `,"lesson_id":null,"deadline_at":null,"answer_policy":"after_check"}`
}

func issueJSONWith(template string, audience string, lesson *string, deadline *time.Time, policy string) string {
	deadlineJSON := "null"
	if deadline != nil {
		deadlineJSON = `"` + deadline.UTC().Format(time.RFC3339Nano) + `"`
	}
	return `{"template_id":"` + template + `","audience":` + audience + `,"lesson_id":` + optionalJSON(lesson) + `,"deadline_at":` + deadlineJSON +
		`,"answer_policy":"` + policy + `"}`
}

func readyBlocks(group byte) []templateBlockJSON {
	manual := templateBlockJSON{ID: blockID(group, 3), Type: string(domain.BlockLongText), Prompt: text("Describe the trip"), MaxLength: points(2000), Points: points(5)}
	return []templateBlockJSON{
		instructionBlock(blockID(group, 1), "Read the text first"),
		singleChoiceBlock(blockID(group, 2), "The capital of France", optionID(group, 1), optionID(group, 2)),
		manual,
	}
}

func (h *authHarness) readyTemplate(t *testing.T, token string, title string, group byte) templateBody {
	t.Helper()
	template := h.createTemplate(t, token, title, nil, readyBlocks(group)...)
	if !template.Ready {
		t.Fatalf("the seeded template is not ready: %+v", template.Problems)
	}
	return template
}

func (h *authHarness) issueParties(t *testing.T) issueParties {
	t.Helper()
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	return issueParties{tutor: tutor, tutorToken: token, template: h.readyTemplate(t, token, "Trip to Paris", 40)}
}

func (h *authHarness) issueAnswer(t *testing.T, token string, key string, body string) response {
	t.Helper()
	headers := jsonCall(token)
	if key != "" {
		headers["Idempotency-Key"] = key
	}
	return h.send(t, http.MethodPost, issuesRoute, body, headers)
}

func (h *authHarness) issued(t *testing.T, token string, body string) issueItemBody {
	t.Helper()
	answer := h.issueAnswer(t, token, "", body)
	h.mustAnswer(t, answer, http.StatusCreated)
	return decodeLibrary[issueItemBody](t, answer)
}

func (h *authHarness) issueDetailAnswer(t *testing.T, token string, id string) response {
	t.Helper()
	return h.send(t, http.MethodGet, issuesRoute+"/"+id, "", jsonCall(token))
}

func (h *authHarness) issueDetail(t *testing.T, token string, id string) issueDetailBody {
	t.Helper()
	answer := h.issueDetailAnswer(t, token, id)
	h.mustAnswer(t, answer, http.StatusOK)
	return decodeLibrary[issueDetailBody](t, answer)
}

func (h *authHarness) issuesAnswer(t *testing.T, token string, query url.Values) response {
	t.Helper()
	path := issuesRoute
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}
	return h.send(t, http.MethodGet, path, "", jsonCall(token))
}

func (h *authHarness) issuesPage(t *testing.T, token string, query url.Values) issuePageBody {
	t.Helper()
	answer := h.issuesAnswer(t, token, query)
	h.mustAnswer(t, answer, http.StatusOK)
	return decodeLibrary[issuePageBody](t, answer)
}

func (h *authHarness) patchIssue(t *testing.T, token string, id string, body string) response {
	t.Helper()
	return h.send(t, http.MethodPatch, issuesRoute+"/"+id, body, jsonCall(token))
}

func (h *authHarness) withdrawIssue(t *testing.T, token string, id string) response {
	t.Helper()
	return h.send(t, http.MethodPost, issuesRoute+"/"+id+"/withdraw", "", jsonCall(token))
}

func (h *authHarness) submissionsOf(t *testing.T, issue string) []issueSubmissionRow {
	t.Helper()
	rows, err := h.database.Pool.Query(t.Context(),
		"SELECT id::text, student_id::text, status, late FROM submissions WHERE issue_id = $1::uuid ORDER BY student_id", issue)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var found []issueSubmissionRow
	for rows.Next() {
		var row issueSubmissionRow
		if err := rows.Scan(&row.id, &row.student, &row.status, &row.late); err != nil {
			t.Fatal(err)
		}
		found = append(found, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return found
}

type issueSubmissionRow struct {
	id      string
	student string
	status  string
	late    bool
}

func (h *authHarness) submitStored(t *testing.T, submission string, at time.Time) {
	t.Helper()
	if _, err := h.database.Pool.Exec(t.Context(),
		"UPDATE submissions SET status = 'submitted', first_submitted_at = $2, submitted_at = $2, late = (deadline_at IS NOT NULL AND $2 > deadline_at) WHERE id = $1::uuid",
		submission, at); err != nil {
		t.Fatal(err)
	}
}

func (h *authHarness) markIssueForErasure(t *testing.T, issue string) {
	t.Helper()
	if _, err := h.database.Pool.Exec(t.Context(), "UPDATE issues SET erase_pending_since = $2 WHERE id = $1::uuid", issue, h.now(t)); err != nil {
		t.Fatal(err)
	}
}

func (h *authHarness) setRelationshipState(t *testing.T, card string, state domain.RelationshipState) {
	t.Helper()
	statement := "UPDATE relationships SET state = $2 WHERE id = $1::uuid"
	switch state {
	case domain.RelationshipEnded:
		statement = "UPDATE relationships SET state = $2, ended_at = now(), ended_by = 'tutor' WHERE id = $1::uuid"
	case domain.RelationshipDiscarded:
		statement = "UPDATE relationships SET state = $2, discard_reason = 'removed', discarded_at = now() WHERE id = $1::uuid"
	case domain.RelationshipInvited, domain.RelationshipProvisional, domain.RelationshipReclaimPending, domain.RelationshipActive:
	}
	if _, err := h.database.Pool.Exec(t.Context(), statement, card, string(state)); err != nil {
		t.Fatal(err)
	}
}

func eventsNamed(items []notificationItem, event string) []notificationItem {
	var named []notificationItem
	for _, item := range items {
		if item.Event == event {
			named = append(named, item)
		}
	}
	return named
}

func issueKey(n int) string {
	return fmt.Sprintf("issue-key-%d", n)
}

func containsAny(haystack string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(haystack, needle) {
			return true
		}
	}
	return false
}

func (h *authHarness) lessonWith(t *testing.T, token string, card string) string {
	t.Helper()
	return h.scheduled(t, token, oneOffFor(card, "2030-03-12", "10:00")).Lessons[0].ID
}

func (h *authHarness) scheduledLessonOf(t *testing.T, token string) string {
	t.Helper()
	tutor := h.userID(t, "another-tutor@example.org")
	card := h.linkedCard(t, tutor, 90, domain.RelationshipActive)
	return h.lessonWith(t, token, card)
}

func (h *authHarness) setSnapshotTotal(t *testing.T, tutor domain.UserID, total int64) {
	t.Helper()
	if _, err := h.database.Pool.Exec(t.Context(), "UPDATE tutor_profiles SET snapshot_bytes_total = $2 WHERE user_id = $1::uuid", tutor.String(), total); err != nil {
		t.Fatal(err)
	}
}

func (h *authHarness) fillHomeworkSlots(t *testing.T, tutor domain.UserID, purpose domain.RateLimitPurpose, count int) {
	t.Helper()
	window, _ := domain.FixedWindow(h.now(t), domain.IssueWindow)
	subject := h.keys.ThrottleSubject(string(purpose), tutor.String())
	if _, err := h.database.Pool.Exec(t.Context(), `INSERT INTO rate_limit_counters (purpose, subject, window_start, count) VALUES ($1, $2, $3, $4)
ON CONFLICT (purpose, subject, window_start) DO UPDATE SET count = excluded.count`, string(purpose), subject, window, count); err != nil {
		t.Fatal(err)
	}
}

func homeworkCapsHook(t *testing.T, caps domain.HomeworkCaps) optionsHook {
	t.Helper()
	return func(options *api.Options, shared *clock.Shared, logger *slog.Logger, database *testdb.Database) {
		keys, err := core.DeriveKeys(core.NewSecret("test-signing-material-0123456789abcdef"))
		if err != nil {
			t.Fatal(err)
		}
		store := repositories.NewStore(database.Pool)
		options.Homework = homework.New(homework.Dependencies{
			Store: store, Clock: shared, Random: randomPerTest(t), Keys: keys, Addresses: options.Files, Templates: options.Library,
			Logger: logger, Caps: caps,
		})
	}
}

func newHomeworkHarnessWithCaps(t *testing.T, caps domain.HomeworkCaps) *authHarness {
	t.Helper()
	return newAuthHarnessStoring(t, flowProxies(), api.NewHandler, func(local *teststorage.Storage) storage.Store { return local.Store },
		homeworkCapsHook(t, caps))
}

const randomIssueID = "0b3c7a52-3f7e-4e21-9a3d-5c1f0e2d4b6a"

type issueOwners struct {
	owner         domain.UserID
	ownerToken    string
	intruder      domain.UserID
	intruderToken string
	template      templateBody
	card          string
}

func (h *authHarness) issueOwners(t *testing.T) issueOwners {
	t.Helper()
	owner, ownerToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	intruder, intruderToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, "another-tutor@example.org")
	return issueOwners{
		owner: owner, ownerToken: ownerToken, intruder: intruder, intruderToken: intruderToken,
		template: h.readyTemplate(t, ownerToken, "Trip to Paris", 45), card: h.linkedCard(t, owner, 10, domain.RelationshipActive),
	}
}

func anotherTutorsIssue(method string, suffix string, body string) denialCase {
	return func(t *testing.T, h *authHarness) (response, response) {
		t.Helper()
		owners := h.issueOwners(t)
		h.shareStudentWith(t, owners.intruder, domain.UserID{10})
		issue := h.issued(t, owners.ownerToken, issueJSON(owners.template.ID, toCard(owners.card)))
		outOfScope := h.send(t, method, issuesRoute+"/"+issue.ID+suffix, body, jsonCall(owners.intruderToken))
		random := h.send(t, method, issuesRoute+"/"+randomIssueID+suffix, body, jsonCall(owners.intruderToken))
		if after := h.issueDetail(t, owners.ownerToken, issue.ID); after.Status != "active" || after.Version != 1 {
			t.Fatalf("another tutor changed the issue: %+v", after.issueItemBody)
		}
		return outOfScope, random
	}
}

func anotherTutorsIssueTemplate(t *testing.T, h *authHarness) (response, response) {
	t.Helper()
	owners := h.issueOwners(t)
	own := h.linkedCard(t, owners.intruder, 11, domain.RelationshipActive)
	ownTemplate := h.readyTemplate(t, owners.intruderToken, "Own", 46)
	outOfScope := h.issueAnswer(t, owners.intruderToken, "", issueJSON(owners.template.ID, toCard(own)))
	foreignCard := h.issueAnswer(t, owners.intruderToken, "", issueJSON(ownTemplate.ID, toCard(owners.card)))
	random := h.issueAnswer(t, owners.intruderToken, "", issueJSON(randomIssueID, toCard(own)))
	if foreignCard.status != http.StatusNotFound || refusal(t, foreignCard) != refusal(t, random) {
		t.Fatalf("issuing to another tutor's card answered %s", describe(foreignCard))
	}
	return outOfScope, random
}

func anotherTutorsIssueFilter(t *testing.T, h *authHarness) (response, response) {
	t.Helper()
	owners := h.issueOwners(t)
	group := h.newGroup(t, owners.ownerToken, "Evening club", []string{owners.card})
	outOfScope := h.issuesAnswer(t, owners.intruderToken, url.Values{"group": {group.ID}})
	foreignStudent := h.issuesAnswer(t, owners.intruderToken, url.Values{"student": {owners.card}})
	random := h.issuesAnswer(t, owners.intruderToken, url.Values{"group": {randomIssueID}})
	if foreignStudent.status != http.StatusNotFound || refusal(t, foreignStudent) != refusal(t, random) {
		t.Fatalf("listing another tutor's student answered %s", describe(foreignStudent))
	}
	return outOfScope, random
}
