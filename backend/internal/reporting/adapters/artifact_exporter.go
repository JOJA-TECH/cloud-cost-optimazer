package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/reporting"
	"github.com/google/uuid"
)

var errInvalidFormattedReport = errors.New("formatted report is invalid")

type ObjectStorageExportAdapter struct {
	storage reporting.ReportObjectStorage
}

func NewObjectStorageExportAdapter(storage reporting.ReportObjectStorage) *ObjectStorageExportAdapter {
	return &ObjectStorageExportAdapter{storage: storage}
}

var _ reporting.ReportExportAdapter = (*ObjectStorageExportAdapter)(nil)

func (a *ObjectStorageExportAdapter) Export(ctx context.Context, report reporting.FormattedReport) (reporting.ReportArtifact, error) {
	if a == nil || a.storage == nil {
		return reporting.ReportArtifact{}, reporting.ErrReportDependencyUnavailable
	}
	if err := ctx.Err(); err != nil {
		return reporting.ReportArtifact{}, err
	}
	filename := safeFilename(report.Filename)
	if filename == "" || filename == "." || filename == ".." {
		return reporting.ReportArtifact{}, fmt.Errorf("%w: invalid filename", errInvalidFormattedReport)
	}
	expected, err := describeFormattedReport(report)
	if err != nil {
		return reporting.ReportArtifact{}, err
	}
	stored, err := a.storage.Store(ctx, reporting.ReportStorageObject{
		Filename: filename, ContentType: report.ContentType, Body: append([]byte(nil), report.Body...),
	})
	if err != nil {
		return reporting.ReportArtifact{}, fmt.Errorf("store report artifact: %w", err)
	}
	if stored.StorageURI == "" || stored.MIMEType != expected.MIMEType ||
		stored.FileSizeBytes != expected.FileSizeBytes || !strings.EqualFold(stored.ChecksumSHA256, expected.ChecksumSHA256) {
		return reporting.ReportArtifact{}, fmt.Errorf("%w: storage returned inconsistent metadata", errInvalidFormattedReport)
	}
	return artifactFromStored(stored), nil
}

type InMemoryObjectStorage struct {
	mu      sync.RWMutex
	objects map[string][]byte
}

func NewInMemoryObjectStorage() *InMemoryObjectStorage {
	return &InMemoryObjectStorage{objects: make(map[string][]byte)}
}

var _ reporting.ReportObjectStorage = (*InMemoryObjectStorage)(nil)

func (s *InMemoryObjectStorage) Store(ctx context.Context, object reporting.ReportStorageObject) (reporting.StoredReportObject, error) {
	if s == nil {
		return reporting.StoredReportObject{}, reporting.ErrReportDependencyUnavailable
	}
	if err := ctx.Err(); err != nil {
		return reporting.StoredReportObject{}, err
	}
	filename := safeFilename(object.Filename)
	if filename == "" || filename == "." || filename == ".." || object.ContentType == "" || len(object.Body) == 0 {
		return reporting.StoredReportObject{}, errInvalidFormattedReport
	}
	uri := "memory://reports/" + uuid.NewString() + "/" + filename
	stored := describeObject(uri, object.ContentType, object.Body)
	s.mu.Lock()
	if s.objects == nil {
		s.objects = make(map[string][]byte)
	}
	s.objects[uri] = append([]byte(nil), object.Body...)
	s.mu.Unlock()
	return stored, nil
}

func (s *InMemoryObjectStorage) Read(uri string) ([]byte, bool) {
	if s == nil {
		return nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	body, ok := s.objects[uri]
	return append([]byte(nil), body...), ok
}

type InMemoryExportAdapter struct {
	storage  *InMemoryObjectStorage
	exporter *ObjectStorageExportAdapter
}

func NewInMemoryExportAdapter() *InMemoryExportAdapter {
	storage := NewInMemoryObjectStorage()
	return &InMemoryExportAdapter{
		storage: storage, exporter: NewObjectStorageExportAdapter(storage),
	}
}

var _ reporting.ReportExportAdapter = (*InMemoryExportAdapter)(nil)

func (a *InMemoryExportAdapter) Export(ctx context.Context, report reporting.FormattedReport) (reporting.ReportArtifact, error) {
	if a == nil || a.exporter == nil {
		return reporting.ReportArtifact{}, reporting.ErrReportDependencyUnavailable
	}
	return a.exporter.Export(ctx, report)
}

func (a *InMemoryExportAdapter) Read(uri string) ([]byte, bool) {
	if a == nil {
		return nil, false
	}
	return a.storage.Read(uri)
}

type LocalFilesystemObjectStorage struct {
	root string
}

func NewLocalFilesystemObjectStorage(root string) (*LocalFilesystemObjectStorage, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("report storage directory is required")
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve report storage directory: %w", err)
	}
	return &LocalFilesystemObjectStorage{root: absoluteRoot}, nil
}

var _ reporting.ReportObjectStorage = (*LocalFilesystemObjectStorage)(nil)

func (s *LocalFilesystemObjectStorage) Store(ctx context.Context, object reporting.ReportStorageObject) (reporting.StoredReportObject, error) {
	if s == nil || s.root == "" {
		return reporting.StoredReportObject{}, reporting.ErrReportDependencyUnavailable
	}
	filename := safeFilename(object.Filename)
	if filename == "" || filename == "." || filename == ".." || object.ContentType == "" || len(object.Body) == 0 {
		return reporting.StoredReportObject{}, errInvalidFormattedReport
	}
	if err := ctx.Err(); err != nil {
		return reporting.StoredReportObject{}, err
	}
	if err := os.MkdirAll(s.root, 0700); err != nil {
		return reporting.StoredReportObject{}, fmt.Errorf("create report storage directory: %w", err)
	}
	target := filepath.Join(s.root, uuid.NewString()+"-"+filename)
	temp, err := os.CreateTemp(s.root, ".report-*.tmp")
	if err != nil {
		return reporting.StoredReportObject{}, fmt.Errorf("create temporary report artifact: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0600); err != nil {
		_ = temp.Close()
		return reporting.StoredReportObject{}, fmt.Errorf("secure temporary report artifact: %w", err)
	}
	if _, err := temp.Write(object.Body); err != nil {
		_ = temp.Close()
		return reporting.StoredReportObject{}, fmt.Errorf("write report artifact: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return reporting.StoredReportObject{}, fmt.Errorf("sync report artifact: %w", err)
	}
	if err := temp.Close(); err != nil {
		return reporting.StoredReportObject{}, fmt.Errorf("close report artifact: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return reporting.StoredReportObject{}, err
	}
	if err := os.Rename(tempPath, target); err != nil {
		return reporting.StoredReportObject{}, fmt.Errorf("publish report artifact: %w", err)
	}
	absoluteTarget, err := filepath.Abs(target)
	if err != nil {
		return reporting.StoredReportObject{}, fmt.Errorf("resolve report artifact path: %w", err)
	}
	uri := (&url.URL{Scheme: "file", Path: filepath.ToSlash(absoluteTarget)}).String()
	return describeObject(uri, object.ContentType, object.Body), nil
}

type LocalFilesystemExportAdapter struct {
	storage  *LocalFilesystemObjectStorage
	exporter *ObjectStorageExportAdapter
}

func NewLocalFilesystemExportAdapter(root string) (*LocalFilesystemExportAdapter, error) {
	storage, err := NewLocalFilesystemObjectStorage(root)
	if err != nil {
		return nil, err
	}
	return &LocalFilesystemExportAdapter{
		storage: storage, exporter: NewObjectStorageExportAdapter(storage),
	}, nil
}

var _ reporting.ReportExportAdapter = (*LocalFilesystemExportAdapter)(nil)

func (a *LocalFilesystemExportAdapter) Export(ctx context.Context, report reporting.FormattedReport) (reporting.ReportArtifact, error) {
	if a == nil || a.exporter == nil {
		return reporting.ReportArtifact{}, reporting.ErrReportDependencyUnavailable
	}
	return a.exporter.Export(ctx, report)
}

func describeFormattedReport(report reporting.FormattedReport) (reporting.StoredReportObject, error) {
	if report.ContentType == "" || len(report.Body) == 0 {
		return reporting.StoredReportObject{}, errInvalidFormattedReport
	}
	if report.SizeBytes != int64(len(report.Body)) {
		return reporting.StoredReportObject{}, fmt.Errorf("%w: size does not match body", errInvalidFormattedReport)
	}
	checksum := sha256.Sum256(report.Body)
	checksumHex := hex.EncodeToString(checksum[:])
	if report.Checksum != "" && !strings.EqualFold(report.Checksum, checksumHex) {
		return reporting.StoredReportObject{}, fmt.Errorf("%w: checksum does not match body", errInvalidFormattedReport)
	}
	return reporting.StoredReportObject{
		MIMEType: report.ContentType, FileSizeBytes: int64(len(report.Body)), ChecksumSHA256: checksumHex,
	}, nil
}

func describeObject(uri, mimeType string, body []byte) reporting.StoredReportObject {
	checksum := sha256.Sum256(body)
	return reporting.StoredReportObject{
		StorageURI: uri, MIMEType: mimeType, FileSizeBytes: int64(len(body)),
		ChecksumSHA256: hex.EncodeToString(checksum[:]),
	}
}

func artifactFromStored(stored reporting.StoredReportObject) reporting.ReportArtifact {
	size := stored.FileSizeBytes
	checksum := stored.ChecksumSHA256
	return reporting.ReportArtifact{
		UID:            uuid.New(),
		StorageURI:     stored.StorageURI,
		MIMEType:       stored.MIMEType,
		FileSizeBytes:  &size,
		ChecksumSHA256: &checksum,
		CreatedAt:      time.Now().UTC(),
	}
}

func safeFilename(filename string) string {
	filename = filepath.Base(strings.TrimSpace(filename))
	if filename == "." || filename == ".." || strings.ContainsAny(filename, `/\`) {
		return ""
	}
	return filename
}
