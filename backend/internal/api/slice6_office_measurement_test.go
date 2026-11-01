//go:build slice6_measurement

package api_test

import (
	"encoding/binary"
	"fmt"
	"net/http"
	"ovc/backend/internal/adapters/clock"
	"strings"
	"testing"
	"time"
)

type measurementOfficeFixture struct {
	body              []byte
	relationshipBytes int
	directoryBytes    uint32
}

func measurementOfficeDocument(t *testing.T, relationships int, totalRelationshipBytes int) measurementOfficeFixture {
	t.Helper()
	entries := map[string]string{
		"[Content_Types].xml": `<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="bin" ContentType="application/octet-stream"/><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Override PartName="/word/document.xml" ContentType="` + wordMain + `"/></Types>`,
		"_rels/.rels":         `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`,
		"word/document.xml":   `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body/></w:document>`,
	}
	entries["[Content_Types].xml"] += strings.Repeat(" ", (1<<20)-len(entries["[Content_Types].xml"]))
	order := []string{"[Content_Types].xml", "_rels/.rels", "word/document.xml"}
	remaining := totalRelationshipBytes - len(entries["_rels/.rels"])
	for index := range relationships {
		name := fmt.Sprintf("word/_rels/part%d.xml.rels", index)
		value := `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"></Relationships>`
		size := remaining / (relationships - index)
		entries[name] = value + strings.Repeat(" ", size-len(value))
		remaining -= size
		order = append(order, name)
	}
	for len(order) < 10000 {
		name := fmt.Sprintf("word/media/%s%05d.bin", strings.Repeat("a", 340), len(order))
		entries[name] = "x"
		order = append(order, name)
	}
	body := zipOf(t, entries, order)
	if len(body) < 22 {
		t.Fatal("measurement office directory missing")
	}
	directory := binary.LittleEndian.Uint32(body[len(body)-10 : len(body)-6])
	if directory > 4<<20 {
		t.Fatal("measurement office directory exceeds declared bound")
	}
	return measurementOfficeFixture{body: body, relationshipBytes: totalRelationshipBytes, directoryBytes: directory}
}

func TestSlice6OfficeInspectionMeasurement(t *testing.T) {
	h := newUploadHarness(t)
	parties := h.activeStudentInAGroup(t)
	channel := h.directChannel(t, parties.card.ID).ID
	cases := []struct {
		name                   string
		relationships          int
		status                 int
		totalRelationshipBytes int
	}{{name: "bounded", relationships: 1, totalRelationshipBytes: 8 << 20, status: http.StatusCreated}, {name: "aggregate_relationship_refusal", relationships: 2, totalRelationshipBytes: (8 << 20) + 1, status: http.StatusUnsupportedMediaType}}

	for _, sample := range cases {
		data := measurementOfficeDocument(t, sample.relationships, sample.totalRelationshipBytes)
		measurementResources(t, h, "office_before")
		started := (clock.System{}).Now()
		answer := h.chatUpload(t, parties.studentToken, channel, "measurement.docx", data.body)
		t.Logf("event=measurement.office scenario=%s archive_bytes=%d entries=10000 content_types_bytes=1048576 relationships_files=%d relationship_bytes_total=%d directory_bytes=%d duration_ns=%d status=%d", sample.name, len(data.body), sample.relationships+1, data.relationshipBytes, data.directoryBytes, time.Since(started).Nanoseconds(), answer.status)
		measurementResources(t, h, "office_after")
		h.mustAnswer(t, answer, sample.status)
		if answer.status == http.StatusCreated {
			message := h.sent(t, parties.studentToken, channel, "Synthetic office document", uploadedID(t, answer))
			h.mustAnswer(t, h.deleteMessage(t, parties.studentToken, message.ID), http.StatusNoContent)
		}
		h.tick(t)
	}

	if h.usage(t, parties.tutor) != 0 {
		t.Fatal("office measurement cleanup retained charges")
	}
}
