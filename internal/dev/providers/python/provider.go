package python

import (
	"github.com/BurntSushi/toml"
	"github.com/yetric/worktools/internal/dev/provider"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Provider struct{}

var _ provider.Provider = Provider{}

func (Provider) Name() string { return "python" }
func (Provider) Manifests() []string {
	return []string{"pyproject.toml", "Pipfile", "setup.py", "setup.cfg", "requirements.txt", "pytest.ini"}
}
func (Provider) Runners() []string { return []string{"python", "uv", "poetry", "pipenv"} }

func collectStrings(value any) []string {
	result := []string{}
	switch v := value.(type) {
	case string:
		return []string{v}
	case []any:
		for _, child := range v {
			result = append(result, collectStrings(child)...)
		}
	case []map[string]any:
		for _, child := range v {
			result = append(result, collectStrings(child)...)
		}
	case map[string]any:
		for key, child := range v {
			result = append(result, key)
			result = append(result, collectStrings(child)...)
		}
	case []string:
		result = append(result, v...)
	}
	return result
}
func table(value any) map[string]any {
	if v, ok := value.(map[string]any); ok {
		return v
	}
	return map[string]any{}
}
func hasKey(value map[string]any, key string) bool { _, ok := value[key]; return ok }

func usesUnittest(root string) bool {
	files, _ := filepath.Glob(filepath.Join(root, "test*.py"))
	filepath.WalkDir(filepath.Join(root, "tests"), func(path string, d fs.DirEntry, e error) error {
		if e == nil && !d.IsDir() && strings.HasPrefix(d.Name(), "test") && strings.HasSuffix(d.Name(), ".py") {
			files = append(files, path)
		}
		return nil
	})
	for _, path := range files {
		b, e := os.ReadFile(path)
		if e == nil && unittestImport.Match(b) {
			return true
		}
	}
	return false
}

func (Provider) Discover(root string) ([]provider.Action, error) {
	manifest := map[string]any{}
	if provider.File(root, "pyproject.toml") {
		if _, e := toml.DecodeFile(filepath.Join(root, "pyproject.toml"), &manifest); e != nil {
			return nil, e
		}
	}
	tool := table(manifest["tool"])
	project := table(manifest["project"])
	declarations := []string{}
	for _, value := range []any{project["dependencies"], project["optional-dependencies"], manifest["dependency-groups"], tool["poetry"]} {
		declarations = append(declarations, collectStrings(value)...)
	}
	if provider.File(root, "requirements.txt") {
		text, e := provider.Read(root, "requirements.txt")
		if e != nil {
			return nil, e
		}
		declarations = append(declarations, strings.Split(text, "\n")...)
	}
	if provider.File(root, "Pipfile") {
		pip := map[string]any{}
		if _, e := toml.DecodeFile(filepath.Join(root, "Pipfile"), &pip); e != nil {
			return nil, e
		}
		declarations = append(declarations, collectStrings(pip)...)
	}
	declared := func(name string) bool {
		pattern := regexp.MustCompile(`(?i)^` + regexp.QuoteMeta(name) + `(?:\[|[<>=!~; @]|$)`)
		for _, value := range declarations {
			if pattern.MatchString(strings.TrimSpace(value)) {
				return true
			}
		}
		return false
	}
	type environment struct {
		runner string
		prefix []string
	}
	envs := []environment{}
	if provider.File(root, "uv.lock") || hasKey(tool, "uv") {
		envs = append(envs, environment{"uv", []string{"uv", "run", "--no-sync", "--no-python-downloads", "python"}})
	}
	if provider.File(root, "poetry.lock") || hasKey(tool, "poetry") {
		envs = append(envs, environment{"poetry", []string{"poetry", "run", "python"}})
	}
	if provider.File(root, "Pipfile") {
		envs = append(envs, environment{"pipenv", []string{"pipenv", "run", "python"}})
	}
	if len(envs) == 0 {
		interpreter := "python3"
		if provider.File(root, ".venv/bin/python") {
			interpreter = filepath.Join(root, ".venv/bin/python")
		}
		envs = append(envs, environment{"python", []string{interpreter}})
	}
	result := []provider.Action{}
	for _, env := range envs {
		add := func(name string, args []string, reason string) {
			command := append(append([]string{}, env.prefix...), args...)
			result = append(result, provider.NewAction(name, env.runner, command, reason))
		}
		if hasKey(tool, "pytest") || declared("pytest") || provider.File(root, "pytest.ini") || provider.File(root, "conftest.py") {
			add("test", []string{"-m", "pytest"}, "Declared pytest tooling")
		} else if usesUnittest(root) {
			dir := "."
			if s, e := os.Stat(filepath.Join(root, "tests")); e == nil && s.IsDir() {
				dir = "tests"
			}
			add("test", []string{"-m", "unittest", "discover", "-s", dir}, "Static unittest imports in test files")
		}
		if hasKey(tool, "ruff") || declared("ruff") {
			add("check", []string{"-m", "ruff", "check", "."}, "Declared Ruff tooling")
			add("format", []string{"-m", "ruff", "format", "."}, "Declared Ruff tooling")
		}
		if hasKey(tool, "mypy") || declared("mypy") {
			add("typecheck", []string{"-m", "mypy", "."}, "Declared mypy tooling")
		}
		if hasKey(manifest, "build-system") {
			if env.runner == "poetry" {
				result = append(result, provider.NewAction("build", "poetry", []string{"poetry", "build"}, "Python build-system and Poetry"))
			} else {
				add("build", []string{"-m", "build"}, "Python build-system; requires the build module")
			}
		}
		if provider.File(root, "manage.py") {
			add("start", []string{"manage.py", "runserver"}, "Django manage.py convention")
		}
	}
	return result, nil
}

var unittestImport = regexp.MustCompile(`(?m)^\s*(?:import\s+unittest\b|from\s+unittest(?:\.[\w.]+)?\s+import\b)`)
