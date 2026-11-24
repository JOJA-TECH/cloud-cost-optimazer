package recommendation

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDomainPackageKeepsInfrastructureDependenciesBehindAdapters(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve architecture test location")
	}
	directory := filepath.Dir(currentFile)
	files, err := filepath.Glob(filepath.Join(directory, "*.go"))
	if err != nil {
		t.Fatalf("list recommendation package files: %v", err)
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
				"/internal/data/repositories",
				"/internal/analytics",
				"terraform",
				"azure",
				"frontend",
			} {
				if strings.Contains(lowerPath, forbidden) {
					t.Errorf("%s imports %q; domain code must use narrow ports and adapters", filepath.Base(file), importPath)
				}
			}
		}
	}
}
