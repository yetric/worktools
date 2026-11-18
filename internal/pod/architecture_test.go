package pod

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
	e := filepath.WalkDir("providers", func(path string, entry fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, e := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if e != nil {
			return e
		}
		for _, imp := range file.Imports {
			value, e := strconv.Unquote(imp.Path.Value)
			if e != nil {
				return e
			}
			if strings.HasPrefix(value, module) && value != module+"internal/pod/provider" {
				t.Errorf("%s couples its provider to %s", path, value)
			}
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
}
