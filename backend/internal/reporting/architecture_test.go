package reporting

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDomainPackageKeepsDependenciesBehindPortsAndAdapters(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve architecture test location")
	}
	directory := filepath.Dir(currentFile)
	files, err := filepath.Glob(filepath.Join(directory, "*.go"))
	if err != nil {
		t.Fatalf("list reporting package files: %v", err)
	}

	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse imports in %s: %v", filepath.Base(file), err)
		}
		for _, spec := range parsed.Imports {
			importPath := strings.Trim(spec.Path.Value, `"`)
			lowerPath := strings.ToLower(importPath)
			for _, forbidden := range []string{
				"gorm.io/gorm",
				"/internal/analytics",
				"/internal/audit",
				"/internal/recommendation",
			} {
				if strings.Contains(lowerPath, forbidden) {
					t.Errorf("%s imports %q; domain dependencies must stay behind ports and adapters", filepath.Base(file), importPath)
				}
			}
		}
	}
}

func TestSourceModulesDoNotDependOnReporting(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve architecture test location")
	}
	internalDirectory := filepath.Dir(filepath.Dir(currentFile))
	reportingImport := "/internal/reporting"

	for _, module := range []string{"analytics", "audit", "recommendation"} {
		files, err := filepath.Glob(filepath.Join(internalDirectory, module, "*.go"))
		if err != nil {
			t.Fatalf("list %s package files: %v", module, err)
		}
		for _, file := range files {
			if strings.HasSuffix(file, "_test.go") {
				continue
			}
			parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatalf("parse imports in %s: %v", filepath.Base(file), err)
			}
			for _, spec := range parsed.Imports {
				importPath := strings.Trim(spec.Path.Value, `"`)
				if strings.Contains(strings.ToLower(importPath), reportingImport) {
					t.Errorf("%s imports %q; %s must not depend on Reporting", filepath.Base(file), importPath, module)
				}
			}
		}
	}
}
