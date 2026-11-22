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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/reporting"
	"github.com/google/uuid"
)

var errInvalidFormattedReport = errors.New("formatted report is invalid")

type ObjectStorageExportAdapter struct {
	storage reporting.ReportArtifactStorage
}

func NewObjectStorageExportAdapter(storage reporting.ReportArtifactStorage) *ObjectStorageExportAdapter {
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
	uri, err := a.storage.Put(ctx, filename, append([]byte(nil), report.Body...))
	if err != nil {
		return reporting.ReportArtifact{}, fmt.Errorf("store report artifact: %w", err)
	}
	size := expected.FileSizeBytes
	checksum := expected.ChecksumSHA256
	return reporting.ReportArtifact{
		UID:            uuid.New(),
		StorageURI:     uri,
		MIMEType:       report.ContentType,
		FileSizeBytes:  &size,
		ChecksumSHA256: &checksum,
		CreatedAt:      time.Now().UTC(),
	}, nil
}

func (a *ObjectStorageExportAdapter) Download(ctx context.Context, artifact reporting.ReportArtifact) ([]byte, error) {
	if a == nil || a.storage == nil {
		return nil, reporting.ErrReportDependencyUnavailable
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if artifact.StorageURI == "" {
		return nil, reporting.ErrInvalidReportRequest
	}
	body, err := a.storage.Get(ctx, artifact.StorageURI)
	if err != nil {
		return nil, fmt.Errorf("download report artifact: %w", err)
	}
	return body, nil
}

type InMemoryObjectStorage struct {
	mu      sync.RWMutex
	objects map[string][]byte
}

func NewInMemoryObjectStorage() *InMemoryObjectStorage {
	return &InMemoryObjectStorage{objects: make(map[string][]byte)}
}

var _ reporting.ReportArtifactStorage = (*InMemoryObjectStorage)(nil)

func (s *InMemoryObjectStorage) Put(ctx context.Context, name string, content []byte) (string, error) {
	if s == nil {
		return "", reporting.ErrReportDependencyUnavailable
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	filename := safeFilename(name)
	if filename == "" || filename == "." || filename == ".." || len(content) == 0 {
		return "", errInvalidFormattedReport
	}
	uri := "memory://reports/" + uuid.NewString() + "/" + filename
	s.mu.Lock()
	if s.objects == nil {
		s.objects = make(map[string][]byte)
	}
	s.objects[uri] = append([]byte(nil), content...)
	s.mu.Unlock()
	return uri, nil
}

func (s *InMemoryObjectStorage) Get(ctx context.Context, uri string) ([]byte, error) {
	if s == nil {
		return nil, reporting.ErrReportDependencyUnavailable
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	body, ok := s.objects[uri]
	if !ok {
		return nil, reporting.ErrReportNotFound
	}
	return append([]byte(nil), body...), nil
}

func (s *InMemoryObjectStorage) Delete(ctx context.Context, uri string) error {
	if s == nil {
		return reporting.ErrReportDependencyUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.objects[uri]; !ok {
		return reporting.ErrReportNotFound
	}
	delete(s.objects, uri)
	return nil
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

func (a *InMemoryExportAdapter) Download(ctx context.Context, artifact reporting.ReportArtifact) ([]byte, error) {
	if a == nil || a.exporter == nil {
		return nil, reporting.ErrReportDependencyUnavailable
	}
	return a.exporter.Download(ctx, artifact)
}

func (a *InMemoryExportAdapter) Read(ctx context.Context, uri string) ([]byte, error) {
	if a == nil {
		return nil, reporting.ErrReportDependencyUnavailable
	}
	return a.storage.Get(ctx, uri)
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

var _ reporting.ReportArtifactStorage = (*LocalFilesystemObjectStorage)(nil)

func (s *LocalFilesystemObjectStorage) Put(ctx context.Context, name string, content []byte) (string, error) {
	if s == nil || s.root == "" {
		return "", reporting.ErrReportDependencyUnavailable
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	filename := safeFilename(name)
	if filename == "" || filename == "." || filename == ".." || len(content) == 0 {
		return "", errInvalidFormattedReport
	}
	if err := s.enforceLimits(); err != nil {
		return "", err
	}
	if err := os.MkdirAll(s.root, 0700); err != nil {
		return "", fmt.Errorf("create report storage directory: %w", err)
	}
	target := filepath.Join(s.root, uuid.NewString()+"-"+filename)
	if err := os.WriteFile(target, content, 0600); err != nil {
		return "", fmt.Errorf("write report artifact: %w", err)
	}
	absoluteTarget, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("resolve report artifact path: %w", err)
	}
	uri := (&url.URL{Scheme: "file", Path: filepath.ToSlash(absoluteTarget)}).String()
	return uri, nil
}

// enforceLimits applies retention and quota controls to the local report store:
// expired files (older than CCO_REPORTS_TTL_HOURS) are deleted, and the store
// refuses new writes once CCO_REPORTS_MAX_FILES or CCO_REPORTS_MAX_SIZE_MB are
// exceeded. This keeps var/reports from filling disk on a single instance.
func (s *LocalFilesystemObjectStorage) enforceLimits() error {
	maxFiles := envInt("CCO_REPORTS_MAX_FILES", 100)
	maxSizeMB := envInt64("CCO_REPORTS_MAX_SIZE_MB", 500)
	ttlHours := envInt("CCO_REPORTS_TTL_HOURS", 168)

	entries, err := os.ReadDir(s.root)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("list report storage directory: %w", err)
	}

	var (
		totalSize int64
		fileCount int
		now       = time.Now()
		cutoff    = now.Add(-time.Duration(ttlHours) * time.Hour)
	)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(s.root, e.Name()))
			continue
		}
		fileCount++
		totalSize += info.Size()
	}

	if fileCount >= maxFiles {
		return fmt.Errorf("report storage limit reached: %d files (max %d)", fileCount, maxFiles)
	}
	if totalSize >= maxSizeMB*1024*1024 {
		return fmt.Errorf("report storage limit reached: %d MB (max %d MB)", totalSize/(1024*1024), maxSizeMB)
	}
	return nil
}

func envInt(name string, def int) int {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func envInt64(name string, def int64) int64 {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func (s *LocalFilesystemObjectStorage) Get(ctx context.Context, uri string) ([]byte, error) {
	if s == nil || s.root == "" {
		return nil, reporting.ErrReportDependencyUnavailable
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	u, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("parse artifact uri: %w", err)
	}
	path := filepath.FromSlash(u.Path)
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, reporting.ErrReportNotFound
		}
		return nil, fmt.Errorf("read report artifact: %w", err)
	}
	return body, nil
}

func (s *LocalFilesystemObjectStorage) Delete(ctx context.Context, uri string) error {
	if s == nil || s.root == "" {
		return reporting.ErrReportDependencyUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	u, err := url.Parse(uri)
	if err != nil {
		return fmt.Errorf("parse artifact uri: %w", err)
	}
	path := filepath.FromSlash(u.Path)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return reporting.ErrReportNotFound
		}
		return fmt.Errorf("delete report artifact: %w", err)
	}
	return nil
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

func (a *LocalFilesystemExportAdapter) Download(ctx context.Context, artifact reporting.ReportArtifact) ([]byte, error) {
	if a == nil || a.exporter == nil {
		return nil, reporting.ErrReportDependencyUnavailable
	}
	return a.exporter.Download(ctx, artifact)
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
		MIMEType:       report.ContentType,
		FileSizeBytes:  int64(len(report.Body)),
		ChecksumSHA256: checksumHex,
	}, nil
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
