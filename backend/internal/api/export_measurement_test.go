//go:build export_measurement

package api_test

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	commandline "flag"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"runtime/metrics"
	"slices"
	"strings"
	"testing"
	"time"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

var exportMeasurementSmoke = commandline.Bool("export-measurement-smoke", false, "Exercise the export measurements with small files; never report performance acceptance")

const (
	exportMemorySample    = 10 * time.Millisecond
	exportLargeFileBytes  = domain.LibraryMaxBytes
	exportSmallFileBytes  = 25_000_000
	exportSmokeLargeBytes = 10_000_000
	exportSmokeSmallBytes = 2_500_000
	exportHeapMetric      = "/memory/classes/heap/objects:bytes"
	exportPeakTolerance   = 1.1
)

type exportMemory struct {
	fileBytes  int
	peak       uint64
	totalAlloc uint64
	size       int64
	parts      int64
}

func heapObjectBytes() uint64 {
	samples := []metrics.Sample{{Name: exportHeapMetric}}
	metrics.Read(samples)
	return samples[0].Value.Uint64()
}

func sampleHeapPeak(stop <-chan struct{}, peaks chan<- uint64) {
	ticker := time.NewTicker(exportMemorySample)
	defer ticker.Stop()
	var peak uint64
	for {
		select {
		case <-stop:
			peaks <- peak
			return
		case <-ticker.C:
			peak = max(peak, heapObjectBytes())
		}
	}
}

func measureExportMemory(t *testing.T, fileBytes int) exportMemory {
	t.Helper()
	h := newUploadHarness(t)
	world := h.exportWorld(t)
	upload := h.libraryUpload(t, world.tutor.token, domain.ContextLibrary, "large.pdf", incompressiblePDF(fileBytes, 3))
	h.mustAnswer(t, upload, http.StatusCreated)
	material := h.createMaterial(t, world.tutor.token, uploadedID(t, upload), "Large", nil).ID
	template := h.workTemplate(t, world.tutor.token, "Large work", materialBlock(blockID(85, 1), material))
	h.issueTemplate(t, world.tutor.token, template, toCard(world.firstCard.ID), studentExportPolicy, nil)
	student := h.signedInAgain(t, world.first)
	runtime.GC()
	baseline := heapObjectBytes()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	stop, peaks := make(chan struct{}), make(chan uint64, 1)
	go sampleHeapPeak(stop, peaks)

	row := h.buildDirectly(t, student)

	close(stop)
	peak := <-peaks
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	if row.State != domain.ExportReady || row.SizeBytes == nil {
		t.Fatalf("the measured export is %s", row.State)
	}
	measured := exportMemory{fileBytes: fileBytes, totalAlloc: after.TotalAlloc - before.TotalAlloc, size: *row.SizeBytes}
	if peak > baseline {
		measured.peak = peak - baseline
	}
	measured.parts = (measured.size + domain.ExportPartBytes - 1) / domain.ExportPartBytes
	return measured
}

func TestExportMemoryMeasurement(t *testing.T) {
	large, small := exportLargeFileBytes, exportSmallFileBytes
	if *exportMeasurementSmoke {
		large, small = exportSmokeLargeBytes, exportSmokeSmallBytes
	}

	smaller := measureExportMemory(t, small)
	larger := measureExportMemory(t, large)

	for _, measured := range []exportMemory{smaller, larger} {
		t.Logf("TestExportMemoryMeasurement file_bytes=%d peak_heap_over_baseline=%d total_alloc=%d archive_bytes=%d parts=%d smoke=%t",
			measured.fileBytes, measured.peak, measured.totalAlloc, measured.size, measured.parts, *exportMeasurementSmoke)
	}
	if !*exportMeasurementSmoke && float64(larger.peak) > exportPeakTolerance*float64(smaller.peak) {
		t.Fatalf("the peak heap grew with the file: %d bytes for %d and %d bytes for %d", larger.peak, larger.fileBytes, smaller.peak, smaller.fileBytes)
	}
}

const (
	exportBuildQuotaBytes        = 5_000_000_000
	exportBuildSmallBytes        = domain.ChatMaxBytes
	exportSmokeBuildBytes        = 50_000_000
	exportSmokeMaterialBytes     = 10_000_000
	exportSmokeSmallBuildBytes   = 2_500_000
	exportMeasuredCards          = 300
	exportMeasuredGroupSize      = 10
	exportMeasuredLessons        = 5_000
	exportMeasuredIssues         = 500
	exportMeasuredMessages       = 100_000
	exportSmokeRowShare          = 10
	exportMaterialsPerTemplate   = 10
	exportMaterialsPerHour       = 30
	exportProgressSample         = 500 * time.Millisecond
	exportBuildDecisionLimit     = time.Hour
	exportMeasuredMaterialGroup  = 92
	exportMeasuredFileGroup      = 93
	exportMeasuredStudentPrefix  = 0xe0
	exportMeasuredCardPrefix     = 0xe1
	exportMeasuredItemsAttribute = "items_"
)

const insertMeasuredLessons = `INSERT INTO lessons (id, tutor_id, relationship_id, title, starts_at, duration_minutes, status, timezone, created_at, updated_at)
SELECT gen_random_uuid(), $1::uuid, (($2::text[])[1 + item % cardinality($2::text[])])::uuid, 'Measured lesson',
    timestamptz '2031-01-01T00:00:00Z' + make_interval(hours => item), 45, 'scheduled', 'Europe/Moscow', $3::timestamptz, $3::timestamptz
FROM generate_series(1, $4::int) AS item`

const insertMeasuredMessages = `WITH advanced AS (
    UPDATE channels SET last_seq = last_seq + $3::int, last_activity_at = $4::timestamptz WHERE id = $1::uuid RETURNING id, last_seq
)
INSERT INTO messages (id, channel_id, seq, sender_id, body, version, created_at, updated_at)
SELECT gen_random_uuid(), advanced.id, advanced.last_seq - $3::int + item, $2::uuid, 'Measured message ' || item, 1, $4::timestamptz, $4::timestamptz
FROM advanced CROSS JOIN generate_series(1, $3::int) AS item`

const submitMeasuredWork = `UPDATE submissions SET status = 'submitted', first_submitted_at = $2::timestamptz, submitted_at = $2::timestamptz, version = version + 1
WHERE tutor_id = $1::uuid AND status = 'not_started'`

const readExportProgress = "SELECT coalesce(progress_done, 0) FROM exports WHERE user_id = $1::uuid"

type buildTimeWorkload struct {
	quota    int
	material int
	small    int
	cards    int
	lessons  int
	issues   int
	messages int
}

func buildTimeWorkloadFor(smoke bool) buildTimeWorkload {
	if smoke {
		return buildTimeWorkload{
			quota: exportSmokeBuildBytes, material: exportSmokeMaterialBytes, small: exportSmokeSmallBuildBytes,
			cards: exportMeasuredCards / exportSmokeRowShare, lessons: exportMeasuredLessons / exportSmokeRowShare,
			issues: exportMeasuredIssues / exportSmokeRowShare, messages: exportMeasuredMessages / exportSmokeRowShare,
		}
	}
	return buildTimeWorkload{
		quota: exportBuildQuotaBytes, material: exportLargeFileBytes, small: exportBuildSmallBytes, cards: exportMeasuredCards,
		lessons: exportMeasuredLessons, issues: exportMeasuredIssues, messages: exportMeasuredMessages,
	}
}

func (w buildTimeWorkload) materials() int {
	return (w.quota - 3*w.small) / w.material
}

func (h *authHarness) pastHourlyLimit(t *testing.T, account exportAccount, done int, perHour int) exportAccount {
	t.Helper()
	if done == 0 || done%perHour != 0 {
		return account
	}
	h.advance(t, time.Hour)
	return h.signedInAgain(t, account)
}

func measuredDate(index int) string {
	return authStart.AddDate(0, 0, index+2).Format(time.DateOnly)
}

func (h *authHarness) seedMeasuredMaterials(t *testing.T, tutor exportAccount, card string, workload buildTimeWorkload) (exportAccount, []string) {
	t.Helper()
	materials := make([]string, 0, workload.materials())
	for index := range workload.materials() {
		tutor = h.pastHourlyLimit(t, tutor, index, exportMaterialsPerHour)
		upload := h.libraryUpload(t, tutor.token, domain.ContextLibrary, fmt.Sprintf("material-%d.pdf", index), incompressiblePDF(workload.material, byte(index)))
		h.mustAnswer(t, upload, http.StatusCreated)
		material := h.createMaterial(t, tutor.token, uploadedID(t, upload), fmt.Sprintf("Material %d", index), nil).ID
		lesson := h.scheduled(t, tutor.token, oneOffFor(card, measuredDate(index), "10:00")).Lessons[0].ID
		h.attachToLesson(t, tutor.token, lesson, material)
		materials = append(materials, material)
	}
	blocks := make([]templateBlockJSON, 0, exportMaterialsPerTemplate)
	for index, material := range materials[:min(len(materials), exportMaterialsPerTemplate)] {
		blocks = append(blocks, materialBlock(blockID(exportMeasuredMaterialGroup, byte(index+1)), material))
	}
	template := h.workTemplate(t, tutor.token, "Measured materials", blocks...)
	h.issueTemplate(t, tutor.token, template, toCard(card), studentExportPolicy, nil)
	return tutor, materials
}

func (h *authHarness) seedMeasuredLargeFiles(t *testing.T, world exportWorld, tutor exportAccount, size int) {
	t.Helper()
	group := h.groupChannel(t, world.group.ID).ID
	chat := h.chatUpload(t, tutor.token, group, "large-chat.pdf", incompressiblePDF(size, 201))
	h.mustAnswer(t, chat, http.StatusCreated)
	h.sent(t, tutor.token, group, "A large file", uploadedID(t, chat))
	question := blockID(exportMeasuredFileGroup, 1)
	template := h.workTemplate(t, tutor.token, "Measured files", fileBlock(question, string(domain.AnswerFilePDF)))
	_, submissions := h.issueTemplate(t, tutor.token, template, toCard(world.firstCard.ID), studentExportPolicy, nil)
	student := h.signedInAgain(t, world.first)
	answer := h.homeworkUpload(t, student.token, homeworkRoute(domain.ContextAnswer, submissions[0].id, question), "large-answer.pdf",
		incompressiblePDF(size, 202))
	h.mustAnswer(t, answer, http.StatusCreated)
	draft := h.saved(t, student.token, submissions[0].id, answersJSON(t, 1, saveID(1), attached(question, uploadedID(t, answer))))
	h.mustAnswer(t, h.submitWork(t, student.token, submissions[0].id, draft.Version), http.StatusOK)
	corrected := h.homeworkUpload(t, tutor.token, homeworkRoute(domain.ContextCorrected, submissions[0].id, question), "large-corrected.pdf",
		incompressiblePDF(size, 203))
	h.mustAnswer(t, corrected, http.StatusCreated)
	h.reviewSaved(t, tutor.token, submissions[0].id, reviewJSON(t, 1, 0, saveID(2), "Measured feedback",
		commented(scored(question, 4), "Measured comment", uploadedID(t, corrected))))
	h.published(t, tutor.token, submissions[0].id, domain.ReviewCheck, 1)
}

func (h *authHarness) seedMeasuredCards(t *testing.T, tutor domain.UserID, count int) []string {
	t.Helper()
	encoded, err := h.hasher.Hash(context.Background(), accountPassword)
	if err != nil {
		t.Fatal(err)
	}
	colour := domain.ColourClay
	cards := make([]string, 0, count)
	for index := range count {
		student := domain.UserID{exportMeasuredStudentPrefix, byte(index >> 8), byte(index)}
		err := h.store.Repositories().Users.Create(context.Background(), repositories.NewUser{
			ID: student, Role: domain.RoleStudent, State: domain.AccountActive, Email: fmt.Sprintf("measured-%d@example.org", index),
			EmailVerifiedAt: &authStart, PasswordHash: encoded, Name: fmt.Sprintf("Measured student %d", index), Language: domain.LanguageEnglish,
			Timezone: "Europe/Moscow", Theme: domain.ThemeSystem, ConsentVersion: "2026-10", ConsentAt: authStart, CreatedAt: authStart,
		})
		if err != nil {
			t.Fatal(err)
		}
		card := repositories.NewRelationship{
			ID: domain.RelationshipID{exportMeasuredCardPrefix, byte(index >> 8), byte(index)}, TutorID: tutor, State: domain.RelationshipActive,
			DisplayName: fmt.Sprintf("Measured card %d", index), CreatedAt: authStart, StudentID: &student, Colour: &colour, LinkedAt: &authStart,
		}
		if err := h.store.Repositories().Relationships.Create(context.Background(), card); err != nil {
			t.Fatal(err)
		}
		cards = append(cards, card.ID.String())
	}
	return cards
}

func (h *authHarness) seedMeasuredRows(t *testing.T, tutor exportAccount, workload buildTimeWorkload) exportAccount {
	t.Helper()
	cards := h.seedMeasuredCards(t, tutor.id, workload.cards)
	var groups, channels []string
	for start := 0; start+exportMeasuredGroupSize <= len(cards); start += exportMeasuredGroupSize {
		group := h.newGroup(t, tutor.token, fmt.Sprintf("Measured group %d", start/exportMeasuredGroupSize), cards[start:start+exportMeasuredGroupSize])
		groups = append(groups, group.ID)
		channels = append(channels, h.groupChannel(t, group.ID).ID)
	}
	if _, err := h.database.Pool.Exec(context.Background(), insertMeasuredLessons, tutor.id.String(), cards, authStart, workload.lessons); err != nil {
		t.Fatal(err)
	}
	for _, channel := range channels {
		if _, err := h.database.Pool.Exec(context.Background(), insertMeasuredMessages, channel, tutor.id.String(), workload.messages/len(channels),
			authStart); err != nil {
			t.Fatal(err)
		}
	}
	template := h.workTemplate(t, tutor.token, "Measured work", automaticBlocks()...)
	for index := range workload.issues {
		tutor = h.pastHourlyLimit(t, tutor, index, domain.IssuesPerHour/2)
		h.issued(t, tutor.token, issueJSON(template, toGroup(groups[index%len(groups)])))
	}
	if _, err := h.database.Pool.Exec(context.Background(), submitMeasuredWork, tutor.id.String(), h.now(t)); err != nil {
		t.Fatal(err)
	}
	return tutor
}

type progressSample struct {
	elapsed time.Duration
	done    int32
}

func sampleExportProgress(read func() (int32, error), started time.Time, stop <-chan struct{}, samples chan<- []progressSample, failures chan<- error) {
	ticker := time.NewTicker(exportProgressSample)
	defer ticker.Stop()
	var taken []progressSample
	for {
		select {
		case <-stop:
			samples <- taken
			failures <- nil
			return
		case <-ticker.C:
			done, err := read()
			if err != nil {
				samples <- taken
				failures <- err
				return
			}
			taken = append(taken, progressSample{elapsed: time.Since(started), done: done})
		}
	}
}

func filesStreamedWithin(samples []progressSample, files int32) (time.Duration, bool) {
	index := slices.IndexFunc(samples, func(sample progressSample) bool { return sample.done > files })
	if index < 0 {
		return 0, false
	}
	return samples[index].elapsed, true
}

type rangedArchive struct {
	address string
}

func (r rangedArchive) ReadAt(buffer []byte, offset int64) (int, error) {
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, r.address, nil)
	if err != nil {
		return 0, err
	}
	request.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, offset+int64(len(buffer))-1))
	answer, err := http.DefaultClient.Do(request)
	if err != nil {
		return 0, err
	}
	if answer.StatusCode != http.StatusPartialContent {
		return 0, errors.Join(fmt.Errorf("a ranged archive read answered %d", answer.StatusCode), answer.Body.Close())
	}
	read, readErr := io.ReadFull(answer.Body, buffer)
	return read, errors.Join(readErr, answer.Body.Close())
}

type archivedEntrySize struct {
	uncompressed uint64
	compressed   uint64
}

func archivedEntrySizes(t *testing.T, address string, size int64) map[string]archivedEntrySize {
	t.Helper()
	reader, err := zip.NewReader(rangedArchive{address: address}, size)
	if err != nil {
		t.Fatal(err)
	}
	sizes := map[string]archivedEntrySize{}
	for _, file := range reader.File {
		sizes[file.Name] = archivedEntrySize{uncompressed: file.UncompressedSize64, compressed: file.CompressedSize64}
	}
	return sizes
}

func sheetRows(t *testing.T, record map[string]json.RawMessage) string {
	t.Helper()
	var counts []string
	for key, value := range record {
		if strings.HasPrefix(key, exportMeasuredItemsAttribute) {
			counts = append(counts, key+"="+string(value))
		}
	}
	slices.Sort(counts)
	return strings.Join(counts, " ")
}

func TestExportBuildTimeMeasurement(t *testing.T) {
	workload := buildTimeWorkloadFor(*exportMeasurementSmoke)
	h := newUploadHarness(t)
	world := h.exportWorld(t)
	tutor, materials := h.seedMeasuredMaterials(t, world.tutor, world.firstCard.ID, workload)
	h.seedMeasuredLargeFiles(t, world, tutor, workload.small)
	tutor = h.signedInAgain(t, h.seedMeasuredRows(t, tutor, workload))
	read := func() (int32, error) {
		var done int32
		err := h.database.Pool.QueryRow(context.Background(), readExportProgress, tutor.id.String()).Scan(&done)
		return done, err
	}
	runtime.GC()
	baseline := heapObjectBytes()
	stop, peaks := make(chan struct{}), make(chan uint64, 1)
	stopProgress, samples, failures := make(chan struct{}), make(chan []progressSample, 1), make(chan error, 1)
	started := time.Now()
	go sampleHeapPeak(stop, peaks)
	go sampleExportProgress(read, started, stopProgress, samples, failures)

	row := h.buildDirectly(t, tutor)

	wall := time.Since(started)
	close(stop)
	close(stopProgress)
	peak, taken, sampled := <-peaks, <-samples, <-failures
	if sampled != nil {
		t.Fatalf("the progress sampler failed: %v", sampled)
	}
	if row.State != domain.ExportReady || row.SizeBytes == nil {
		t.Fatalf("the measured tutor export is %s", row.State)
	}
	built := h.exportRecords(t, "export.built")
	if len(built) != 1 {
		t.Fatalf("the measured build logged %d export.built records", len(built))
	}
	minted, address := h.mintExport(t, tutor.token)
	h.mustAnswer(t, minted, http.StatusOK)
	sizes := archivedEntrySizes(t, address.URL, *row.SizeBytes)
	files := decodedAs[int32](t, built[0]["files"]) + decodedAs[int32](t, built[0]["files_unreadable"])
	streamed, measured := filesStreamedWithin(taken, files)
	share := -1.0
	if measured {
		share = streamed.Seconds() / wall.Seconds()
	}
	var heapPeak uint64
	if peak > baseline {
		heapPeak = peak - baseline
	}
	parts := (*row.SizeBytes + domain.ExportPartBytes - 1) / domain.ExportPartBytes
	t.Logf("TestExportBuildTimeMeasurement wall_ms=%d built_duration_ms=%s peak_heap_over_baseline=%d archive_bytes=%d parts=%d materials=%d files=%d "+
		"tables_bytes=%d tables_compressed_bytes=%d data_bytes=%d data_compressed_bytes=%d files_streaming_share=%.3f progress_samples=%d "+
		"authorization_queries=not_measured smoke=%t",
		wall.Milliseconds(), built[0]["duration_ms"], heapPeak, *row.SizeBytes, parts, len(materials), files,
		sizes["tables.xlsx"].uncompressed, sizes["tables.xlsx"].compressed, sizes["data.json"].uncompressed, sizes["data.json"].compressed,
		share, len(taken), *exportMeasurementSmoke)
	t.Logf("TestExportBuildTimeMeasurement rows_per_sheet %s", sheetRows(t, built[0]))
	if !*exportMeasurementSmoke && wall > exportBuildDecisionLimit {
		t.Logf("TestExportBuildTimeMeasurement the build took %s, more than half of the two-hour budget; the files must move to a second archive", wall)
	}
}
