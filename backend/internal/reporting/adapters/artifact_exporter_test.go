package adapters

import (
	"context"
	"encoding/csv"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/reporting"
	"github.com/google/uuid"
)

func TestObjectStorageExportAdapterUsesInMemoryStorage(t *testing.T) {
	storage := NewInMemoryObjectStorage()
	exporter := NewObjectStorageExportAdapter(storage)
	formatted := formattedReportForTest()

	artifact, err := exporter.Export(context.Background(), formatted)
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	body, err := storage.Get(context.Background(), artifact.StorageURI)
	if err != nil || string(body) != string(formatted.Body) {
		t.Fatalf("stored artifact read error=%v body=%q", err, body)
	}
	if artifact.FileSizeBytes == nil || *artifact.FileSizeBytes != int64(len(formatted.Body)) {
		t.Fatalf("artifact size metadata = %#v", artifact.FileSizeBytes)
	}
	if artifact.ChecksumSHA256 == nil || *artifact.ChecksumSHA256 == "" {
		t.Fatalf("artifact checksum metadata = %#v", artifact.ChecksumSHA256)
	}
}

func TestObjectStorageExportAdapterWritesLocalFile(t *testing.T) {
	storage, err := NewLocalFilesystemObjectStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFilesystemObjectStorage() error = %v", err)
	}
	formatted := formattedReportForTest()
	artifact, err := NewObjectStorageExportAdapter(storage).Export(context.Background(), formatted)
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	uri, err := url.Parse(artifact.StorageURI)
	if err != nil {
		t.Fatalf("parse artifact URI: %v", err)
	}
	if uri.Scheme != "file" {
		t.Fatalf("artifact URI scheme = %q, want file", uri.Scheme)
	}
	body, err := os.ReadFile(filepath.FromSlash(uri.Path))
	if err != nil {
		t.Fatalf("read exported artifact: %v", err)
	}
	if string(body) != string(formatted.Body) {
		t.Fatalf("exported body = %q", body)
	}
}

func TestObjectStorageExportAdapterRejectsInvalidMetadata(t *testing.T) {
	formatted := formattedReportForTest()
	formatted.SizeBytes++
	_, err := NewObjectStorageExportAdapter(NewInMemoryObjectStorage()).Export(context.Background(), formatted)
	if err == nil || !strings.Contains(err.Error(), "size does not match body") {
		t.Fatalf("Export() error = %v, want invalid size error", err)
	}
}

func TestCSVFormatterEscapesSpreadsheetFormulas(t *testing.T) {
	report := reporting.Report{
		Format: reporting.ReportFormatCSV, Title: "Reporte",
		Sections: []reporting.ReportSection{{
			Key: "recommendations", Title: "Recomendaciones",
			Recommendations: []reporting.ReportRecommendation{{
				RecommendationUID: uuid.New(),
				Action:            "=1+1",
				Status:            "pending",
			}},
		}},
	}
	formatted, err := NewCSVFormatter().Format(context.Background(), report)
	if err != nil {
		t.Fatalf("Format() error = %v", err)
	}
	rows, err := csv.NewReader(strings.NewReader(string(formatted.Body))).ReadAll()
	if err != nil {
		t.Fatalf("parse formatted CSV: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected header + 1 row, got %d rows", len(rows))
	}
	// action está en la columna índice 2.
	if rows[1][2] != "'=1+1" {
		t.Fatalf("action cell = %q, want formula-safe %q", rows[1][2], "'=1+1")
	}
}

func formattedReportForTest() reporting.FormattedReport {
	body := []byte(`{"report":"test"}`)
	return reporting.FormattedReport{
		Format: reporting.ReportFormatJSON, ContentType: "application/json",
		Filename: "report.json", Body: body, SizeBytes: int64(len(body)),
	}
}

func TestInMemoryObjectStoragePutGetDelete(t *testing.T) {
	storage := NewInMemoryObjectStorage()
	uri, err := storage.Put(context.Background(), "test.txt", []byte("hello"))
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if uri == "" {
		t.Fatal("Put() returned empty URI")
	}
	body, err := storage.Get(context.Background(), uri)
	if err != nil || string(body) != "hello" {
		t.Fatalf("Get() error=%v body=%q", err, body)
	}
	if err := storage.Delete(context.Background(), uri); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := storage.Get(context.Background(), uri); !errors.Is(err, reporting.ErrReportNotFound) {
		t.Fatalf("Get() after delete error=%v, want ErrReportNotFound", err)
	}
}

func TestLocalFilesystemObjectStoragePutGetDelete(t *testing.T) {
	storage, err := NewLocalFilesystemObjectStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFilesystemObjectStorage() error = %v", err)
	}
	uri, err := storage.Put(context.Background(), "test.txt", []byte("world"))
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if uri == "" {
		t.Fatal("Put() returned empty URI")
	}
	body, err := storage.Get(context.Background(), uri)
	if err != nil || string(body) != "world" {
		t.Fatalf("Get() error=%v body=%q", err, body)
	}
	if err := storage.Delete(context.Background(), uri); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := storage.Get(context.Background(), uri); !errors.Is(err, reporting.ErrReportNotFound) {
		t.Fatalf("Get() after delete error=%v, want ErrReportNotFound", err)
	}
}
