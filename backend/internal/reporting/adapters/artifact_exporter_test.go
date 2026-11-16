package adapters

import (
	"context"
	"encoding/csv"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/reporting"
)

func TestObjectStorageExportAdapterUsesInMemoryStorage(t *testing.T) {
	storage := NewInMemoryObjectStorage()
	exporter := NewObjectStorageExportAdapter(storage)
	formatted := formattedReportForTest()

	artifact, err := exporter.Export(context.Background(), formatted)
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	body, ok := storage.Read(artifact.StorageURI)
	if !ok || string(body) != string(formatted.Body) {
		t.Fatalf("stored artifact = %q, found=%v", body, ok)
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
		Format: reporting.ReportFormatCSV,
		Sections: []reporting.ReportSection{{
			Key: "metrics", Title: "Metrics",
			Metrics: []reporting.ReportMetric{{Code: "external", Label: "External", Value: "=1+1"}},
		}},
	}
	formatted, err := NewCSVFormatter().Format(context.Background(), report)
	if err != nil {
		t.Fatalf("Format() error = %v", err)
	}
	rows, err := csv.NewReader(strings.NewReader(string(formatted.Body))).ReadAll()
	if err != nil && err != io.EOF {
		t.Fatalf("parse formatted CSV: %v", err)
	}
	if len(rows) != 2 || rows[1][11] != "'=1+1" {
		t.Fatalf("CSV metric value = %v, want formula-safe value", rows)
	}
}

func formattedReportForTest() reporting.FormattedReport {
	body := []byte(`{"report":"test"}`)
	return reporting.FormattedReport{
		Format: reporting.ReportFormatJSON, ContentType: "application/json",
		Filename: "report.json", Body: body, SizeBytes: int64(len(body)),
	}
}
