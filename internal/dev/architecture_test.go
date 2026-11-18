package dev

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestProviderPackagesAreIndependent(t *testing.T) {
	const module = "github.com/yetric/worktools/"
	err := filepath.WalkDir("providers", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range file.Imports {
			value, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				return err
			}
			if strings.HasPrefix(value, module) && value != module+"internal/dev/provider" {
				t.Errorf("%s couples a provider to %s; depend on the provider contract only", path, value)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
