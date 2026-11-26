//go:build export_measurement

package api_test

import (
	commandline "flag"
	"net/http"
	"runtime"
	"runtime/metrics"
	"testing"
	"time"

	"ovc/backend/internal/domain"
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
