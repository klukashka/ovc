package api_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"ovc/backend/internal/domain"
)

type lessonMaterialBody struct {
	MaterialID   *string   `json:"material_id"`
	AttachmentID string    `json:"attachment_id"`
	Title        string    `json:"title"`
	Filename     string    `json:"filename"`
	MediaType    string    `json:"media_type"`
	SizeBytes    int64     `json:"size_bytes"`
	Width        *int32    `json:"width"`
	Height       *int32    `json:"height"`
	URL          string    `json:"url"`
	ExpiresAt    time.Time `json:"expires_at"`
}

func materialsPath(lesson string) string {
	return lessonsRoute + "/" + lesson + "/materials"
}

func materialsBody(t *testing.T, version int, ids []string) string {
	t.Helper()
	if ids == nil {
		ids = []string{}
	}
	encoded, err := json.Marshal(ids)
	if err != nil {
		t.Fatal(err)
	}
	return `{"material_ids":` + string(encoded) + `,"version":` + strconv.Itoa(version) + `}`
}

func (h *authHarness) attachMaterials(t *testing.T, token string, lesson string, version int, ids ...string) response {
	t.Helper()
	return h.send(t, http.MethodPost, materialsPath(lesson), materialsBody(t, version, ids), jsonCall(token))
}

func (h *authHarness) attached(t *testing.T, token string, lesson string, version int, ids ...string) lessonView {
	t.Helper()
	answer := h.attachMaterials(t, token, lesson, version, ids...)
	h.mustAnswer(t, answer, http.StatusOK)
	return decodeLesson(t, answer)
}

func (h *authHarness) libraryMaterial(t *testing.T, tutor domain.UserID, token string, n byte, title string) libraryItemBody {
	t.Helper()
	return h.createMaterial(t, token, h.pendingLibraryFile(t, tutor, 70, n, 1000), title, nil)
}

func (h *authHarness) libraryMaterials(t *testing.T, tutor domain.UserID, token string, count int) []libraryItemBody {
	t.Helper()
	materials := make([]libraryItemBody, 0, count)
	for index := range count {
		materials = append(materials, h.libraryMaterial(t, tutor, token, byte(index+1), "Material "+strconv.Itoa(index+1)))
	}
	return materials
}

func materialIDsOfItems(items []libraryItemBody) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

func attachedMaterialIDs(view lessonView) []string {
	ids := make([]string, 0, len(view.Materials))
	for _, material := range view.Materials {
		if material.MaterialID == nil {
			ids = append(ids, "")
			continue
		}
		ids = append(ids, *material.MaterialID)
	}
	return ids
}

func (h *authHarness) referencesOfKind(t *testing.T, attachment string, kind domain.ReferenceKind) int64 {
	t.Helper()
	return h.countRows(t, "SELECT count(ref_id) FROM attachment_references WHERE attachment_id = $1::uuid AND ref_kind = '"+string(kind)+"'", attachment)
}

func (h *authHarness) storageDeletionsOf(t *testing.T, attachment string) int64 {
	t.Helper()
	return h.countRows(t, "SELECT count(id) FROM outbox WHERE kind = 'storage_delete' AND idempotency_key = 'storage_delete:' || $1", attachment)
}

func (h *authHarness) attachmentStateOf(t *testing.T, attachment string) string {
	t.Helper()
	var state string
	if err := h.database.Pool.QueryRow(t.Context(), "SELECT state FROM attachments WHERE id = $1::uuid", attachment).Scan(&state); err != nil {
		t.Fatal(err)
	}
	return state
}

func (h *authHarness) seriesMaterialIDs(t *testing.T, series string) []string {
	t.Helper()
	rows, err := h.database.Pool.Query(t.Context(), "SELECT material_id::text FROM series_materials WHERE series_id = $1::uuid ORDER BY position", series)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return ids
}

func (h *authHarness) lessonMaterialIDs(t *testing.T, lesson string) []string {
	t.Helper()
	rows, err := h.database.Pool.Query(t.Context(), "SELECT material_id::text FROM lesson_materials WHERE lesson_id = $1::uuid ORDER BY position", lesson)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return ids
}
