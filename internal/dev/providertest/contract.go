// Package providertest supplies reusable, runtime-free tests for provider PRs.
package providertest

import (
	"crypto/sha256"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/yetric/worktools/internal/dev/provider"
)

type Case struct {
	Files   map[string]string
	Action  string
	Command []string
}

// Check verifies metadata, repeatable discovery, valid actions, an expected
// command, and that discovery leaves the fixture tree unchanged.
func Check(t *testing.T, p provider.Provider, fixture Case) {
	t.Helper()
	root := t.TempDir()
	for name, content := range fixture.Files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if p.Name() == "" || len(p.Manifests()) == 0 || len(p.Runners()) == 0 {
		t.Fatal("provider must declare its name, manifests, and runners")
	}
	snapshot := func() map[string][32]byte {
		files := map[string][32]byte{}
		if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !entry.IsDir() {
				content, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				files[path] = sha256.Sum256(content)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		return files
	}
	before := snapshot()
	first, err := p.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := p.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("discovery must be repeatable")
	}
	if !reflect.DeepEqual(before, snapshot()) {
		t.Fatal("discovery changed project files")
	}
	found := false
	for _, a := range first {
		if !provider.ValidName(a.Name) || !provider.Contains(p.Runners(), a.Runner) || len(a.Command) == 0 || a.Command[0] == "" || a.Description == "" || a.Reason == "" {
			t.Fatalf("invalid action: %+v", a)
		}
		if a.Name == fixture.Action {
			if !reflect.DeepEqual(a.Command, fixture.Command) {
				t.Fatalf("%s: got %v, want %v", a.Name, a.Command, fixture.Command)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("missing action %q", fixture.Action)
	}
}
