package api_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"ovc/backend/internal/domain"
)

func otherIdentifierSpellings() map[string]string {
	return map[string]string{
		"braced":  "{" + randomCardID + "}",
		"URN":     "urn:uuid:" + randomCardID,
		"compact": strings.ReplaceAll(randomCardID, "-", ""),
	}
}

func TestLibraryAndLessonIdentifiersInOtherSpellings(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t)
	_, token := h.signedInAs(t, domain.RoleTutor, domain.AccountActive, 1, tutorEmail)
	folder, material, lesson := libraryRoot+"/folders/"+randomCardID, libraryRoot+"/materials/"+randomCardID, lessonsRoute+"/"+randomCardID
	requests := map[string][4]string{
		"a folder parent":           {http.MethodPost, libraryRoot + "/folders", `{"name":"Notes","parent_id":%q}`, "body.parent_id"},
		"a moved folder parent":     {http.MethodPatch, folder, `{"parent_id":%q}`, "body.parent_id"},
		"a material attachment":     {http.MethodPost, libraryRoot + "/materials", `{"attachment_id":%q,"title":"Notes","folder_id":null}`, "body.attachment_id"},
		"a material folder":         {http.MethodPost, libraryRoot + "/materials", `{"attachment_id":"` + randomCardID + `","title":"Notes","folder_id":%q}`, "body.folder_id"},
		"a moved material folder":   {http.MethodPatch, material, `{"folder_id":%q}`, "body.folder_id"},
		"a new lesson relationship": {http.MethodPost, lessonsRoute, `{"relationship_id":%q,"title":"Lesson","date":"2030-03-12","start":"18:00","duration_minutes":60}`, "body.relationship_id"},
		"a new lesson group":        {http.MethodPost, lessonsRoute, `{"group_id":%q,"title":"Lesson","date":"2030-03-12","start":"18:00","duration_minutes":60}`, "body.group_id"},
		"a new lesson material":     {http.MethodPost, lessonsRoute, `{"group_id":"` + randomCardID + `","title":"Lesson","date":"2030-03-12","start":"18:00","duration_minutes":60,"material_ids":[%q]}`, "body.material_ids[0]"},
		"an edited lesson material": {http.MethodPatch, lesson, `{"scope":"this","version":1,"material_ids":[%q]}`, "body.material_ids[0]"},
		"a lesson's material list":  {http.MethodPost, lesson + "/materials", `{"version":1,"material_ids":[%q]}`, "body.material_ids[0]"},
	}
	type refusal struct {
		answer response
		field  string
	}
	answers := map[string]refusal{}

	for name, request := range requests {
		for spelling, identifier := range otherIdentifierSpellings() {
			answer := h.send(t, request[0], request[1], fmt.Sprintf(request[2], identifier), jsonCall(token))
			answers[name+" in the "+spelling+" spelling"] = refusal{answer: answer, field: request[3]}
		}
	}

	for name, refused := range answers {
		if refused.answer.status != http.StatusUnprocessableEntity || code(t, refused.answer) != "validation_failed" ||
			!strings.Contains(refused.answer.body, `"path":"`+refused.field+`"`) {
			t.Errorf("%s answered %s", name, describe(refused.answer))
		}
	}
}
