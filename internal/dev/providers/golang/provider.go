package golang

import (
	"github.com/yetric/worktools/internal/dev/provider"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

type Provider struct{}

var _ provider.Provider = Provider{}

func (Provider) Name() string        { return "golang" }
func (Provider) Manifests() []string { return []string{"go.mod"} }
func (Provider) Runners() []string   { return []string{"go"} }

func (Provider) Discover(root string) ([]provider.Action, error) {
	result := []provider.Action{provider.NewAction("test", "go", []string{"go", "test", "./..."}, "go.mod default"), provider.NewAction("check", "go", []string{"go", "vet", "./..."}, "go.mod default"), provider.NewAction("build", "go", []string{"go", "build", "./..."}, "go.mod default")}
	hasMain := func(directory string) bool {
		files, _ := filepath.Glob(filepath.Join(root, directory, "*.go"))
		for _, file := range files {
			if strings.HasSuffix(file, "_test.go") {
				continue
			}
			parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.PackageClauseOnly)
			if err == nil && parsed.Name.Name == "main" {
				return true
			}
		}
		return false
	}
	if hasMain(".") {
		result = append(result, provider.NewAction("start", "go", []string{"go", "run", "."}, "Root Go main package"))
	} else {
		entries, _ := os.ReadDir(filepath.Join(root, "cmd"))
		commands := []string{}
		for _, entry := range entries {
			if entry.IsDir() && hasMain(filepath.Join("cmd", entry.Name())) {
				commands = append(commands, "./cmd/"+entry.Name())
			}
		}
		if len(commands) == 1 {
			result = append(result, provider.NewAction("start", "go", []string{"go", "run", commands[0]}, "Single Go command under cmd/"))
		}
	}
	return result, nil
}
