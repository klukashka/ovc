package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/api"
	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

const (
	groupsRoute     = "/api/v1/groups"
	groupName       = "Evening conversation club"
	groupCreated    = `"msg":"group.created"`
	groupArchived   = `"msg":"group.archived"`
	groupUnarchived = `"msg":"group.unarchived"`
	seededCardName  = "A seeded student"
)

type groupMember struct {
	RelationshipID string    `json:"relationship_id"`
	DisplayName    string    `json:"display_name"`
	Name           string    `json:"name"`
	AvatarID       *string   `json:"avatar_id"`
	AddedAt        time.Time `json:"added_at"`
}

type studentGroup struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Colour      string        `json:"colour"`
	ArchivedAt  *time.Time    `json:"archived_at"`
	MemberCount *int64        `json:"member_count"`
	Members     []groupMember `json:"members"`
}

type cardGroup struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Colour string `json:"colour"`
}

func idList(ids []string) string {
	quoted := make([]string, 0, len(ids))
	for _, id := range ids {
		quoted = append(quoted, jsonText(id))
	}
	return "[" + strings.Join(quoted, ",") + "]"
}

func newGroupBody(name string, members []string) string {
	return fmt.Sprintf(`{"name":%s,"colour":"moss","member_ids":%s}`, jsonText(name), idList(members))
}

func (h *authHarness) createGroup(t *testing.T, token string, body string, key string) response {
	t.Helper()
	headers := jsonCall(token)
	if key != "" {
		headers["Idempotency-Key"] = key
	}
	return h.send(t, http.MethodPost, groupsRoute, body, headers)
}

func (h *authHarness) newGroup(t *testing.T, token string, name string, members []string) studentGroup {
	t.Helper()
	result := h.createGroup(t, token, newGroupBody(name, members), "")
	if result.status != http.StatusCreated {
		t.Fatalf("creating a group answered %d %s", result.status, result.body)
	}
	return decodeGroup(t, result)
}

func (h *authHarness) groupCall(t *testing.T, token string, method string, id string, suffix string, body string) response {
	t.Helper()
	return h.send(t, method, groupsRoute+"/"+id+suffix, body, jsonCall(token))
}

func decodeGroup(t *testing.T, result response) studentGroup {
	t.Helper()
	var group studentGroup
	if err := json.Unmarshal([]byte(result.body), &group); err != nil || group.ID == "" {
		t.Fatalf("not a group: %d %s", result.status, result.body)
	}
	return group
}

func (h *authHarness) listGroups(t *testing.T, token string) []studentGroup {
	t.Helper()
	result := h.send(t, http.MethodGet, groupsRoute, "", jsonCall(token))
	var decoded struct {
		Groups []studentGroup `json:"groups"`
	}
	if err := json.Unmarshal([]byte(result.body), &decoded); err != nil || result.status != http.StatusOK || decoded.Groups == nil {
		t.Fatalf("not a group list: %d %s", result.status, result.body)
	}
	if result.headers.Get("Cache-Control") != "no-store" {
		t.Fatalf("the group list has Cache-Control %q", result.headers.Get("Cache-Control"))
	}
	return decoded.Groups
}

func (h *authHarness) seedStudentAccount(t *testing.T, id byte, state domain.AccountState) domain.UserID {
	t.Helper()
	user := domain.UserID{id}
	encoded, err := h.hasher.Hash(context.Background(), accountPassword)
	if err != nil {
		t.Fatal(err)
	}
	var verifiedAt *time.Time
	if state == domain.AccountActive {
		verifiedAt = &authStart
	}
	err = h.store.Repositories().Users.Create(context.Background(), repositories.NewUser{
		ID: user, Role: domain.RoleStudent, State: state, Email: fmt.Sprintf("member-%d@example.org", id), EmailVerifiedAt: verifiedAt,
		PasswordHash: encoded, Name: fmt.Sprintf("Member account %d", id), Language: domain.LanguageEnglish,
		Timezone: "Europe/Moscow", Theme: domain.ThemeSystem, ConsentVersion: "2026-10", ConsentAt: authStart, CreatedAt: authStart,
	})
	if err != nil {
		t.Fatal(err)
	}
	return user
}

func (h *authHarness) linkedCard(t *testing.T, tutor domain.UserID, id byte, state domain.RelationshipState) string {
	t.Helper()
	accountState := domain.AccountActive
	if state == domain.RelationshipProvisional {
		accountState = domain.AccountUnverified
	}
	student := h.seedStudentAccount(t, id, accountState)
	return h.seedCard(t, tutor, id, state, &student)
}

func (h *authHarness) activeCards(t *testing.T, tutor domain.UserID, first byte, count int) []string {
	t.Helper()
	cards := make([]string, 0, count)
	for index := range count {
		cards = append(cards, h.linkedCard(t, tutor, first+byte(index), domain.RelationshipActive))
	}
	return cards
}

func memberIDsOf(group studentGroup) []string {
	ids := make([]string, 0, len(group.Members))
	for _, member := range group.Members {
		ids = append(ids, member.RelationshipID)
	}
	return ids
}

func cardGroupsOf(t *testing.T, card studentCard) []cardGroup {
	t.Helper()
	groups := make([]cardGroup, 0, len(card.Groups))
	for _, raw := range card.Groups {
		var group cardGroup
		if err := json.Unmarshal(raw, &group); err != nil {
			t.Fatal(err)
		}
		groups = append(groups, group)
	}
	return groups
}

func (h *authHarness) cardOf(t *testing.T, token string, id string) studentCard {
	t.Helper()
	return decodeCard(t, h.cardCall(t, token, http.MethodGet, id, "", ""))
}

func (h *authHarness) studentsInGroup(t *testing.T, token string, group string) response {
	t.Helper()
	return h.send(t, http.MethodGet, "/api/v1/students?group="+group, "", jsonCall(token))
}

func relationshipIDsOf(t *testing.T, result response) []string {
	t.Helper()
	var decoded struct {
		Error struct {
			Details struct {
				RelationshipIDs []string `json:"relationship_ids"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(result.body), &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded.Error.Details.RelationshipIDs
}

func fieldsOf(t *testing.T, result response) []string {
	t.Helper()
	var decoded struct {
		Error struct {
			Fields []struct {
				Path string `json:"path"`
				Code string `json:"code"`
			} `json:"fields"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(result.body), &decoded); err != nil {
		t.Fatal(err)
	}
	fields := make([]string, 0, len(decoded.Error.Fields))
	for _, field := range decoded.Error.Fields {
		fields = append(fields, field.Path+":"+field.Code)
	}
	return fields
}

func TestTutorBuildsAGroupAndItShowsOnCardsListsAndTheStudentFilter(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	members := h.activeCards(t, tutor, 10, 7)
	outsider := h.linkedCard(t, tutor, 20, domain.RelationshipActive)

	created := h.createGroup(t, token, newGroupBody("  "+groupName+"  ", members[:6]), "")
	group := decodeGroup(t, created)
	h.advance(t, time.Minute)
	patched := h.groupCall(t, token, http.MethodPatch, group.ID, "",
		fmt.Sprintf(`{"name":"Morning club","colour":"plum","add":%s,"remove":%s}`, idList([]string{members[6], members[6]}), idList(members[:1])))
	read := decodeGroup(t, h.groupCall(t, token, http.MethodGet, group.ID, "", ""))
	listed := h.listGroups(t, token)
	memberCard := h.cardOf(t, token, members[1])
	removedCard := h.cardOf(t, token, members[0])
	filtered := h.studentsInGroup(t, token, group.ID)

	if created.status != http.StatusCreated || created.headers.Get("Cache-Control") != "no-store" || group.Name != groupName || group.Colour != "moss" || group.ArchivedAt != nil {
		t.Fatalf("creating answered %d %v %s", created.status, created.headers, created.body)
	}
	if !slices.Equal(memberIDsOf(group), members[:6]) || group.Members[0].DisplayName != seededCardName || group.Members[0].Name != "Member account 10" {
		t.Fatalf("the created group holds %+v", group.Members)
	}
	if patched.status != http.StatusOK || decodeGroup(t, patched).Name != "Morning club" || decodeGroup(t, patched).Colour != "plum" {
		t.Fatalf("patching answered %d %s", patched.status, patched.body)
	}
	if !slices.Equal(memberIDsOf(read), members[1:]) || read.Members[5].AddedAt.Before(read.Members[0].AddedAt.Add(time.Minute)) {
		t.Fatalf("after the change the group holds %+v", read.Members)
	}
	if len(listed) != 1 || listed[0].MemberCount == nil || *listed[0].MemberCount != 6 || !slices.Equal(memberIDsOf(listed[0]), members[1:6]) {
		t.Fatalf("the list shows %+v", listed)
	}
	if groups := cardGroupsOf(t, memberCard); len(groups) != 1 || groups[0] != (cardGroup{ID: group.ID, Name: "Morning club", Colour: "plum"}) {
		t.Fatalf("a member's card shows the groups %+v", groups)
	}
	if groups := cardGroupsOf(t, removedCard); len(groups) != 0 {
		t.Fatalf("a removed member's card still shows %+v", groups)
	}
	var page cardPage
	if err := json.Unmarshal([]byte(filtered.body), &page); err != nil || filtered.status != http.StatusOK || len(page.Items) != 6 {
		t.Fatalf("the student filter answered %d %s", filtered.status, filtered.body)
	}
	for _, item := range page.Items {
		if item.ID == outsider || item.ID == members[0] || len(item.Groups) != 1 {
			t.Fatalf("the group filter listed %+v", item)
		}
	}
	logs := h.logs.String()
	for _, private := range []string{groupName, "Morning club", seededCardName, "Member account"} {
		if strings.Contains(logs, private) {
			t.Fatalf("the logs contain %q", private)
		}
	}
	for _, event := range []string{
		groupCreated + `,"tutor_id":"` + tutor.String() + `","group_id":"` + group.ID + `","members":6`,
		`"msg":"group.updated","tutor_id":"` + tutor.String() + `","group_id":"` + group.ID + `","fields":"name,colour"`,
		`"msg":"group.membership_changed","tutor_id":"` + tutor.String() + `","group_id":"` + group.ID + `","added":1,"removed":1`,
	} {
		if !strings.Contains(logs, event) {
			t.Fatalf("no record %s in %s", event, logs)
		}
	}
}

func TestTutorCannotReachAnotherTutorsGroup(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	owner, ownerToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	_, intruder := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
	members := h.activeCards(t, owner, 10, 2)
	group := h.newGroup(t, ownerToken, groupName, members)
	calls := map[string]struct{ method, suffix, body string }{
		"read":      {http.MethodGet, "", ""},
		"rename":    {http.MethodPatch, "", `{"name":"Taken over","remove":` + idList(members) + `}`},
		"archive":   {http.MethodPost, "/archive", ""},
		"unarchive": {http.MethodPost, "/unarchive", ""},
	}

	for name, call := range calls {
		outOfScope := h.groupCall(t, intruder, call.method, group.ID, call.suffix, call.body)
		random := h.groupCall(t, intruder, call.method, randomCardID, call.suffix, call.body)

		if outOfScope.status != http.StatusNotFound || refusal(t, outOfScope) != refusal(t, random) {
			t.Errorf("%s of another tutor's group answered %d %s, a random one %d %s", name, outOfScope.status, outOfScope.body, random.status, random.body)
		}
	}
	filteredOutOfScope := h.studentsInGroup(t, intruder, group.ID)
	filteredRandom := h.studentsInGroup(t, intruder, randomCardID)
	if filteredOutOfScope.status != http.StatusNotFound || refusal(t, filteredOutOfScope) != refusal(t, filteredRandom) {
		t.Fatalf("filtering by another tutor's group answered %d %s", filteredOutOfScope.status, filteredOutOfScope.body)
	}
	if listed := h.listGroups(t, intruder); len(listed) != 0 {
		t.Fatalf("another tutor lists %+v", listed)
	}
	after := decodeGroup(t, h.groupCall(t, ownerToken, http.MethodGet, group.ID, "", ""))
	if after.Name != groupName || after.ArchivedAt != nil || !slices.Equal(memberIDsOf(after), members) {
		t.Fatalf("another tutor changed the group: %+v", after)
	}
}

func TestTutorCannotAddAnotherTutorsStudentToAGroup(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	owner, ownerToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	other, _ := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
	own := h.activeCards(t, owner, 10, 1)
	foreign := h.linkedCard(t, other, 20, domain.RelationshipActive)
	withdrawn := h.seedCard(t, owner, 30, domain.RelationshipDiscarded, nil)
	group := h.newGroup(t, ownerToken, groupName, own)
	for name, ids := range map[string][]string{"another tutor's student": {own[0], foreign}, "a discarded card": {withdrawn}} {
		createdOutOfScope := h.createGroup(t, ownerToken, newGroupBody("Second group", ids), "")
		createdRandom := h.createGroup(t, ownerToken, newGroupBody("Second group", []string{own[0], randomCardID}), "")
		addedOutOfScope := h.groupCall(t, ownerToken, http.MethodPatch, group.ID, "", `{"add":`+idList(ids)+`}`)
		addedRandom := h.groupCall(t, ownerToken, http.MethodPatch, group.ID, "", `{"add":`+idList([]string{randomCardID})+`}`)
		removedOutOfScope := h.groupCall(t, ownerToken, http.MethodPatch, group.ID, "", `{"remove":`+idList(ids)+`}`)

		for call, pair := range map[string][2]response{
			"create": {createdOutOfScope, createdRandom}, "add": {addedOutOfScope, addedRandom}, "remove": {removedOutOfScope, addedRandom},
		} {
			if pair[0].status != http.StatusNotFound || refusal(t, pair[0]) != refusal(t, pair[1]) {
				t.Errorf("%s with %s answered %d %s instead of the random answer %s", call, name, pair[0].status, pair[0].body, pair[1].body)
			}
		}
	}

	if listed := h.listGroups(t, ownerToken); len(listed) != 1 || !slices.Equal(memberIDsOf(listed[0]), own) {
		t.Fatalf("the refused calls changed the groups: %+v", listed)
	}
	if h.countRows(t, "SELECT count(group_id) FROM group_members WHERE relationship_id = $1::uuid", foreign) != 0 {
		t.Fatal("another tutor's student joined a group")
	}
}

func TestUnconfirmedStudentCannotJoinAGroup(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	active := h.activeCards(t, tutor, 10, 1)
	provisional := h.linkedCard(t, tutor, 20, domain.RelationshipProvisional)
	reclaimPending := h.linkedCard(t, tutor, 21, domain.RelationshipReclaimPending)
	ended := h.linkedCard(t, tutor, 22, domain.RelationshipEnded)
	invited := h.seedCard(t, tutor, 23, domain.RelationshipInvited, nil)
	group := h.newGroup(t, token, groupName, active)
	refusedIDs := []string{provisional, reclaimPending, ended, invited}
	slices.Sort(refusedIDs)

	created := h.createGroup(t, token, newGroupBody("Second group", append([]string{active[0]}, refusedIDs...)), "")
	added := h.groupCall(t, token, http.MethodPatch, group.ID, "", `{"add":`+idList([]string{provisional, reclaimPending})+`}`)
	removedNonMembers := h.groupCall(t, token, http.MethodPatch, group.ID, "", `{"remove":`+idList(refusedIDs)+`}`)

	if created.status != http.StatusConflict || code(t, created) != "relationship_not_active" || !slices.Equal(relationshipIDsOf(t, created), refusedIDs) {
		t.Fatalf("creating with unconfirmed and inactive students answered %d %s", created.status, created.body)
	}
	expectedAdded := []string{provisional, reclaimPending}
	slices.Sort(expectedAdded)
	if added.status != http.StatusConflict || code(t, added) != "relationship_not_active" || !slices.Equal(relationshipIDsOf(t, added), expectedAdded) {
		t.Fatalf("adding unconfirmed students answered %d %s", added.status, added.body)
	}
	if removedNonMembers.status != http.StatusOK || !slices.Equal(memberIDsOf(decodeGroup(t, removedNonMembers)), active) {
		t.Fatalf("removing cards that are not members answered %d %s", removedNonMembers.status, removedNonMembers.body)
	}
	if listed := h.listGroups(t, token); len(listed) != 1 {
		t.Fatalf("a refused creation left %d groups", len(listed))
	}
	for _, card := range refusedIDs {
		if h.countRows(t, "SELECT count(group_id) FROM group_members WHERE relationship_id = $1::uuid", card) != 0 {
			t.Fatalf("the card %s joined a group", card)
		}
	}
}

func TestGroupRefusesThe31stMember(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	cards := h.activeCards(t, tutor, 10, domain.GroupMemberLimit+1)

	tooMany := h.createGroup(t, token, newGroupBody(groupName, cards), "")
	full := h.newGroup(t, token, groupName, cards[:domain.GroupMemberLimit])
	overfilled := h.groupCall(t, token, http.MethodPatch, full.ID, "", `{"add":`+idList(cards[domain.GroupMemberLimit:])+`}`)
	swapped := h.groupCall(t, token, http.MethodPatch, full.ID, "", `{"add":`+idList(cards[domain.GroupMemberLimit:])+`,"remove":`+idList(cards[:1])+`}`)

	if tooMany.status != http.StatusUnprocessableEntity || code(t, tooMany) != "validation_failed" || !slices.Equal(fieldsOf(t, tooMany), []string{"body.member_ids:invalid"}) {
		t.Fatalf("creating with 31 members answered %d %s", tooMany.status, tooMany.body)
	}
	if overfilled.status != http.StatusUnprocessableEntity || !slices.Equal(fieldsOf(t, overfilled), []string{"body.add:too_long"}) {
		t.Fatalf("adding a 31st member answered %d %s", overfilled.status, overfilled.body)
	}
	if swapped.status != http.StatusOK || len(decodeGroup(t, swapped).Members) != domain.GroupMemberLimit {
		t.Fatalf("swapping a member of a full group answered %d %s", swapped.status, swapped.body)
	}
	if listed := h.listGroups(t, token); len(listed) != 1 || *listed[0].MemberCount != domain.GroupMemberLimit {
		t.Fatalf("the groups are %+v", listed)
	}
}

const seedGroups = `INSERT INTO groups (id, tutor_id, name, colour, archived_at, created_at, updated_at)
SELECT gen_random_uuid(), $1, 'A seeded group', 'teal', CASE WHEN position % 2 = 0 THEN $2::timestamptz END, $2, $2
FROM generate_series(1, $3::integer) AS position`

func TestGroupCapCountsArchivedGroups(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	if _, err := h.database.Pool.Exec(context.Background(), seedGroups, tutor, authStart, domain.GroupLimitPerTutor-2); err != nil {
		t.Fatal(err)
	}
	beforeTheRace := h.createGroup(t, token, newGroupBody("The 199th group", nil), "")
	release := h.holdTutorProfile(t, tutor)
	first := h.sendInBackground(http.MethodPost, groupsRoute, newGroupBody("First racer", nil), jsonCall(token))
	second := h.sendInBackground(http.MethodPost, groupsRoute, newGroupBody("Second racer", nil), jsonCall(token))
	h.waitForLockWaiters(t, 2)

	release()
	answers := []response{awaitAnswer(t, first), awaitAnswer(t, second)}
	afterTheRace := h.createGroup(t, token, newGroupBody("One too many", nil), "")

	if beforeTheRace.status != http.StatusCreated {
		t.Fatalf("the 199th group answered %d %s", beforeTheRace.status, beforeTheRace.body)
	}
	statuses := map[int]int{}
	for _, answer := range answers {
		statuses[answer.status]++
		if answer.status == http.StatusUnprocessableEntity && !slices.Equal(fieldsOf(t, answer), []string{"tutor.groups:too_long"}) {
			t.Fatalf("the refused racer answered %s", answer.body)
		}
	}
	if statuses[http.StatusCreated] != 1 || statuses[http.StatusUnprocessableEntity] != 1 {
		t.Fatalf("the racing creations answered %v", statuses)
	}
	if afterTheRace.status != http.StatusUnprocessableEntity || !slices.Equal(fieldsOf(t, afterTheRace), []string{"tutor.groups:too_long"}) {
		t.Fatalf("the 201st group answered %d %s", afterTheRace.status, afterTheRace.body)
	}
	listed := h.listGroups(t, token)
	archived := 0
	for _, group := range listed {
		if group.ArchivedAt != nil {
			archived++
		}
	}
	if len(listed) != domain.GroupLimitPerTutor || archived != (domain.GroupLimitPerTutor-2)/2 || listed[0].ArchivedAt != nil || listed[len(listed)-1].ArchivedAt == nil {
		t.Fatalf("the tutor lists %d groups, %d archived, active first: %t", len(listed), archived, listed[0].ArchivedAt == nil)
	}
}

func TestArchivedGroupRefusesNewMembers(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	cards := h.activeCards(t, tutor, 10, 3)
	group := h.newGroup(t, token, groupName, cards[:2])

	archived := h.groupCall(t, token, http.MethodPost, group.ID, "/archive", "")
	added := h.groupCall(t, token, http.MethodPatch, group.ID, "", `{"add":`+idList(cards[2:])+`}`)
	renamedAndTrimmed := h.groupCall(t, token, http.MethodPatch, group.ID, "", `{"name":"Archived club","remove":`+idList(cards[:1])+`}`)
	cardWhileArchived := h.cardOf(t, token, cards[1])
	filteredWhileArchived := h.studentsInGroup(t, token, group.ID)
	h.groupCall(t, token, http.MethodPost, group.ID, "/unarchive", "")
	addedAfterUnarchive := h.groupCall(t, token, http.MethodPatch, group.ID, "", `{"add":`+idList(cards[2:])+`}`)

	if archived.status != http.StatusOK || decodeGroup(t, archived).ArchivedAt == nil {
		t.Fatalf("archiving answered %d %s", archived.status, archived.body)
	}
	if added.status != http.StatusConflict || code(t, added) != "audience_not_active" {
		t.Fatalf("adding to an archived group answered %d %s", added.status, added.body)
	}
	if renamedAndTrimmed.status != http.StatusOK || !slices.Equal(memberIDsOf(decodeGroup(t, renamedAndTrimmed)), cards[1:2]) {
		t.Fatalf("renaming and removing in an archived group answered %d %s", renamedAndTrimmed.status, renamedAndTrimmed.body)
	}
	if groups := cardGroupsOf(t, cardWhileArchived); len(groups) != 0 {
		t.Fatalf("a card shows the archived group %+v", groups)
	}
	if filteredWhileArchived.status != http.StatusOK || strings.Count(filteredWhileArchived.body, `"id":"`+cards[1]+`"`) != 1 {
		t.Fatalf("filtering by an archived group answered %d %s", filteredWhileArchived.status, filteredWhileArchived.body)
	}
	if addedAfterUnarchive.status != http.StatusOK || !slices.Equal(memberIDsOf(decodeGroup(t, addedAfterUnarchive)), cards[1:]) {
		t.Fatalf("adding after unarchiving answered %d %s", addedAfterUnarchive.status, addedAfterUnarchive.body)
	}
}

func TestArchiveAndUnarchiveAreIdempotent(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	group := h.newGroup(t, token, groupName, nil)

	first := h.groupCall(t, token, http.MethodPost, group.ID, "/archive", "")
	h.advance(t, 5*time.Minute)
	repeated := h.groupCall(t, token, http.MethodPost, group.ID, "/archive", "")
	unarchived := h.groupCall(t, token, http.MethodPost, group.ID, "/unarchive", "")
	unarchivedAgain := h.groupCall(t, token, http.MethodPost, group.ID, "/unarchive", "")

	firstArchivedAt, repeatedArchivedAt := decodeGroup(t, first).ArchivedAt, decodeGroup(t, repeated).ArchivedAt
	if first.status != http.StatusOK || repeated.status != http.StatusOK || firstArchivedAt == nil || repeatedArchivedAt == nil || !firstArchivedAt.Equal(*repeatedArchivedAt) {
		t.Fatalf("archiving twice answered %s and %s", first.body, repeated.body)
	}
	if unarchived.status != http.StatusOK || unarchivedAgain.status != http.StatusOK || decodeGroup(t, unarchived).ArchivedAt != nil || decodeGroup(t, unarchivedAgain).ArchivedAt != nil {
		t.Fatalf("unarchiving twice answered %s and %s", unarchived.body, unarchivedAgain.body)
	}
	logs := h.logs.String()
	if strings.Count(logs, groupArchived) != 1 || strings.Count(logs, groupUnarchived) != 1 {
		t.Fatalf("expected one archive and one unarchive record")
	}
}

func TestRepeatedGroupIdempotencyKeyReturnsTheFirstGroup(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	cards := h.activeCards(t, tutor, 10, 2)

	first := h.createGroup(t, token, newGroupBody(groupName, cards[:1]), "group-key-1")
	repeated := h.createGroup(t, token, newGroupBody("Another name", cards), "group-key-1")
	other := h.createGroup(t, token, newGroupBody(groupName, cards[:1]), "group-key-2")

	if first.status != http.StatusCreated || repeated.status != http.StatusCreated || other.status != http.StatusCreated {
		t.Fatalf("the creations answered %d, %d and %d", first.status, repeated.status, other.status)
	}
	original, replayed := decodeGroup(t, first), decodeGroup(t, repeated)
	if replayed.ID != original.ID || replayed.Name != groupName || !slices.Equal(memberIDsOf(replayed), cards[:1]) || decodeGroup(t, other).ID == original.ID {
		t.Fatalf("the repeated key answered %+v instead of %+v", replayed, original)
	}
	if strings.Count(h.logs.String(), groupCreated) != 2 || len(h.listGroups(t, token)) != 2 {
		t.Fatal("a repeated key created another group")
	}
}

func TestGroupBodiesAreRefusedBeforeScope(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	cards := h.activeCards(t, tutor, 10, 1)
	group := h.newGroup(t, token, groupName, cards)
	document := loadPublishedDocument(t)
	tooMany := make([]string, domain.GroupMemberLimit+1)
	for index := range tooMany {
		tooMany[index] = fmt.Sprintf("0b3c7a52-3f7e-4e21-9a3d-5c1f0e2d4b%02d", index)
	}
	routes := map[string]struct {
		operation api.OperationAudience
		path      string
		extra     map[string]bodyProbe
	}{
		"create": {api.OperationAudience{Method: http.MethodPost, Path: groupsRoute}, groupsRoute, map[string]bodyProbe{
			"a 61-character name":      {body: newGroupBody(strings.Repeat("n", 61), nil), contentType: "application/json"},
			"31 member identifiers":    {body: newGroupBody(groupName, tooMany), contentType: "application/json"},
			"a malformed member":       {body: newGroupBody(groupName, []string{"not-an-identifier"}), contentType: "application/json"},
			"a colour outside the set": {body: `{"name":"A group","colour":"pink","member_ids":[]}`, contentType: "application/json"},
			"a null member list":       {body: `{"name":"A group","colour":"moss","member_ids":null}`, contentType: "application/json"},
		}},
		"update": {api.OperationAudience{Method: http.MethodPatch, Path: groupsRoute + "/{id}"}, groupsRoute + "/" + randomCardID, map[string]bodyProbe{
			"an empty name":                {body: `{"name":""}`, contentType: "application/json"},
			"31 added identifiers":         {body: `{"add":` + idList(tooMany) + `}`, contentType: "application/json"},
			"a malformed removal":          {body: `{"remove":["nope"]}`, contentType: "application/json"},
			"a colour outside the palette": {body: `{"colour":"pink"}`, contentType: "application/json"},
		}},
	}

	for name, route := range routes {
		schema, takesBody := document.bodySchema(t, route.operation)
		probes := operationProbes(schema)
		for probe, request := range route.extra {
			probes[probe] = request
		}
		if !takesBody {
			t.Fatalf("%s publishes no request body", name)
		}

		h.assertProbesAreRefusedBeforeScope(t, route.operation, route.path, probes, token)
	}

	listed := h.listGroups(t, token)
	if len(listed) != 1 || listed[0].ID != group.ID || listed[0].Name != groupName || strings.Contains(h.logs.String(), `"msg":"group.updated"`) {
		t.Fatalf("a refused body changed the groups: %+v", listed)
	}
}

func anotherTutorsGroup(method string, suffix string, body string) denialCase {
	return func(t *testing.T, h *authHarness) (response, response) {
		t.Helper()
		owner, ownerToken := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
		intruderID, intruder := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 2, secondTutorEmail)
		group := h.newGroup(t, ownerToken, groupName, h.activeCards(t, owner, 10, 1))
		h.shareStudentWith(t, intruderID, domain.UserID{10})
		outOfScope := h.groupCall(t, intruder, method, group.ID, suffix, body)
		random := h.groupCall(t, intruder, method, randomCardID, suffix, body)
		if after := decodeGroup(t, h.groupCall(t, ownerToken, http.MethodGet, group.ID, "", "")); after.Name != groupName || after.ArchivedAt != nil {
			t.Fatalf("another tutor changed the group: %+v", after)
		}
		return outOfScope, random
	}
}

func (h *authHarness) holdGroup(t *testing.T, group string) func() {
	t.Helper()
	return h.holdRow(t, "SELECT "+lockedRowPosition+" FROM groups WHERE id = $1::uuid FOR UPDATE", group)
}

func TestConcurrentAdditionsStopAtTheMemberLimit(t *testing.T) {
	t.Parallel()
	h := newAuthHarness(t)
	tutor, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	cards := h.activeCards(t, tutor, 10, domain.GroupMemberLimit+1)
	group := h.newGroup(t, token, groupName, cards[:domain.GroupMemberLimit-1])
	release := h.holdGroup(t, group.ID)
	first := h.sendInBackground(http.MethodPatch, groupsRoute+"/"+group.ID, `{"add":`+idList(cards[domain.GroupMemberLimit-1:domain.GroupMemberLimit])+`}`, jsonCall(token))
	second := h.sendInBackground(http.MethodPatch, groupsRoute+"/"+group.ID, `{"add":`+idList(cards[domain.GroupMemberLimit:])+`}`, jsonCall(token))
	h.waitForLockWaiters(t, 2)

	release()
	answers := []response{awaitAnswer(t, first), awaitAnswer(t, second)}

	statuses := map[int]int{}
	for _, answer := range answers {
		statuses[answer.status]++
		if answer.status == http.StatusUnprocessableEntity && !slices.Equal(fieldsOf(t, answer), []string{"body.add:too_long"}) {
			t.Fatalf("the refused addition answered %s", answer.body)
		}
	}
	if statuses[http.StatusOK] != 1 || statuses[http.StatusUnprocessableEntity] != 1 {
		t.Fatalf("the racing additions answered %v", statuses)
	}
	after := decodeGroup(t, h.groupCall(t, token, http.MethodGet, group.ID, "", ""))
	if len(after.Members) != domain.GroupMemberLimit {
		t.Fatalf("the group has %d members after the race", len(after.Members))
	}
}
