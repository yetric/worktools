package dev

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type adapter struct {
	name             string
	markers, runners []string
	discover         func(string) ([]Action, error)
}

func (a adapter) Name() string                           { return a.name }
func (a adapter) Manifests() []string                    { return a.markers }
func (a adapter) Runners() []string                      { return a.runners }
func (a adapter) Discover(root string) ([]Action, error) { return a.discover(root) }

func fixture(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, text := range files {
		path := filepath.Join(root, name)
		if e := os.MkdirAll(filepath.Dir(path), 0755); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(path, []byte(text), 0755); e != nil {
			t.Fatal(e)
		}
	}
	return root
}
func records(t *testing.T, root string) map[string]Record {
	t.Helper()
	rows, e := Discover(root, Builtins())
	if e != nil {
		t.Fatal(e)
	}
	result := map[string]Record{}
	for _, r := range rows {
		result[r.Name] = r
	}
	return result
}

func TestEnvironmentDetection(t *testing.T) {
	cases := []struct {
		name   string
		files  map[string]string
		action string
		want   []string
	}{
		{"make", map[string]string{"Makefile": "include local.mk\nX := $(shell touch DONT_CREATE)\nCONFIG := value\n.PHONY: test\ntest: ## Run tests\n\ttouch DONT_CREATE\n", "local.mk": "dev:\n"}, "start", []string{"make", "dev"}},
		{"make precedence", map[string]string{"GNUmakefile": "test:\n", "Makefile": "build:\n"}, "test", []string{"make", "test"}},
		{"npm", map[string]string{"package.json": `{"scripts":{"test":"vitest"}}`}, "test", []string{"npm", "run", "test"}},
		{"pnpm", map[string]string{"package.json": `{"scripts":{"dev":"vite"}}`, "pnpm-lock.yaml": ""}, "start", []string{"pnpm", "run", "dev"}},
		{"declared bun", map[string]string{"package.json": `{"packageManager":"bun@1.0.0","scripts":{"test":"vitest"}}`, "package-lock.json": "{}"}, "test", []string{"bun", "run", "test"}},
		{"composer", map[string]string{"composer.json": `{"scripts":{"dev":"php -S localhost:8000","test":["phpunit"]}}`}, "start", []string{"composer", "run", "dev"}},
		{"go", map[string]string{"go.mod": "module example.test/app\n"}, "check", []string{"go", "vet", "./..."}},
		{"rust binary", map[string]string{"Cargo.toml": "[package]\nname='app'\nversion='0.1.0'\n", "src/main.rs": "fn main(){}"}, "start", []string{"cargo", "run"}},
		{"maven wrapper", map[string]string{"pom.xml": "<project><build><plugins><plugin><artifactId>spring-boot-maven-plugin</artifactId></plugin></plugins></build></project>", "mvnw": "#!/bin/sh\n"}, "start", []string{"./mvnw", "spring-boot:run"}},
		{"gradle wrapper", map[string]string{"build.gradle.kts": "plugins { application }", "gradlew": "#!/bin/sh\n"}, "start", []string{"./gradlew", "run"}},
		{"python pytest", map[string]string{"pyproject.toml": "[project]\nname='example'\n[dependency-groups]\ndev=['pytest>=8','ruff']\n"}, "test", []string{"python3", "-m", "pytest"}},
		{"python ruff", map[string]string{"pyproject.toml": "[tool.ruff]\n"}, "check", []string{"python3", "-m", "ruff", "check", "."}},
		{"python uv", map[string]string{"pyproject.toml": "[tool.pytest.ini_options]\n", "uv.lock": ""}, "test", []string{"uv", "run", "--no-sync", "--no-python-downloads", "python", "-m", "pytest"}},
		{"python poetry", map[string]string{"pyproject.toml": "[tool.poetry]\nname='example'\n[tool.pytest.ini_options]\n"}, "test", []string{"poetry", "run", "python", "-m", "pytest"}},
		{"python unittest", map[string]string{"setup.cfg": "[metadata]\nname=example\n", "tests/test_app.py": "import unittest\nraise RuntimeError('never import')\n"}, "test", []string{"python3", "-m", "unittest", "discover", "-s", "tests"}},
		{"python django", map[string]string{"requirements.txt": "django\n", "manage.py": "raise RuntimeError('never import')"}, "start", []string{"python3", "manage.py", "runserver"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := fixture(t, tc.files)
			r := records(t, root)[tc.action]
			if r.Status != "ready" || !reflect.DeepEqual(r.Command, tc.want) {
				t.Fatalf("got %+v; want %v", r, tc.want)
			}
			if _, e := os.Stat(filepath.Join(root, "DONT_CREATE")); !os.IsNotExist(e) {
				t.Fatal("discovery executed project code")
			}
		})
	}
}

func TestAmbiguity(t *testing.T) {
	for name, files := range map[string]map[string]string{
		"mixed runners":     {"Makefile": "test:\n", "package.json": `{"scripts":{"test":"vitest"}}`},
		"conflicting locks": {"package.json": `{"scripts":{"test":"vitest"}}`, "yarn.lock": "", "pnpm-lock.yaml": ""},
		"java":              {"pom.xml": "<project/>", "build.gradle": "plugins { id 'java' }"},
		"python":            {"pyproject.toml": "[tool.pytest.ini_options]\n", "uv.lock": "", "poetry.lock": ""},
	} {
		t.Run(name, func(t *testing.T) {
			r := records(t, fixture(t, files))["test"]
			if r.Status != "ambiguous" || r.Command != nil {
				t.Fatalf("expected unresolved ambiguity: %+v", r)
			}
		})
	}
}

func TestMissingCapabilities(t *testing.T) {
	for name, files := range map[string]map[string]string{"rust library": {"Cargo.toml": "[package]\nname='library'\nversion='0.1.0'"}, "python unknown tests": {"pyproject.toml": "[project]\nname='app'", "tests/test_app.py": "def test_app(): pass"}} {
		t.Run(name, func(t *testing.T) {
			r := records(t, fixture(t, files))
			if _, ok := r["start"]; ok {
				t.Fatal("invented a startup action")
			}
			if name == "python unknown tests" {
				if _, ok := r["test"]; ok {
					t.Fatal("invented a passing unittest command")
				}
			}
		})
	}
}

func TestProjectOverrides(t *testing.T) {
	root := fixture(t, map[string]string{"Makefile": "test:\n", "package.json": `{"scripts":{"dev":"vite","test":"vitest"}}`, "backend/keep": "", ".worktools.toml": "[dev]\nrunner='make'\n[dev.actions.check]\ncommand=['custom','check']\ndirectory='backend'\nargument_separator=['--']\ndescription='Backend checks'\n"})
	r := records(t, root)
	if !reflect.DeepEqual(r["test"].Command, []string{"make", "test"}) {
		t.Fatal(r["test"])
	}
	if r["start"].Status != "unavailable" {
		t.Fatal("preference silently fell back")
	}
	if r["check"].Directory != filepath.Join(root, "backend") || r["check"].Description != "Backend checks" {
		t.Fatal(r["check"])
	}
}

func TestRootDiscovery(t *testing.T) {
	root := fixture(t, map[string]string{".worktools.toml": "[dev]\n", "backend/composer.json": "{}", "backend/src/keep": ""})
	got, e := Root(filepath.Join(root, "backend/src"), Builtins())
	if e != nil || got != root {
		t.Fatalf("%s %v", got, e)
	}
	os.Mkdir(filepath.Join(root, "backend/.git"), 0755)
	got, e = Root(filepath.Join(root, "backend/src"), Builtins())
	if e != nil || got != filepath.Join(root, "backend") {
		t.Fatalf("%s %v", got, e)
	}
}

func TestAdapterExtension(t *testing.T) {
	root := fixture(t, map[string]string{"Moonfile": "test: moon"})
	moon := adapter{"moon", []string{"Moonfile"}, []string{"moon"}, func(root string) ([]Action, error) {
		return []Action{{Name: "test", Runner: "moon", Command: []string{"moon", "test"}, Description: "Run Moon tests", Reason: "Moonfile", Separator: []string{"--"}}}, nil
	}}
	registry := append(Builtins(), moon)
	got, e := Root(root, registry)
	if e != nil || got != root {
		t.Fatal(got, e)
	}
	r, e := Discover(root, registry)
	if e != nil || len(r) != 1 || !reflect.DeepEqual(r[0].Command, []string{"moon", "test"}) {
		t.Fatal(r, e)
	}
	if r[0].Description != "Run Moon tests" || !reflect.DeepEqual(r[0].Separator, []string{"--"}) {
		t.Fatal(r)
	}
}

func TestInvalidAdapterAndConfig(t *testing.T) {
	root := fixture(t, map[string]string{"Moonfile": ""})
	bad := adapter{"bad", []string{"Moonfile"}, []string{"moon"}, func(string) ([]Action, error) {
		return []Action{{Name: "test", Runner: "bad", Command: []string{"moon"}}}, nil
	}}
	if _, e := Discover(root, []Adapter{bad}); e == nil || !strings.Contains(e.Error(), "bad") {
		t.Fatal("invalid adapter accepted")
	}
	for _, config := range []string{"[dev]\nrunner=['make']", "[dev.actions.test]\ncommand='shell text'", "[dev.actions.test]\ncommand=[]", "[dev.actions.list]\ncommand=['true']"} {
		os.WriteFile(filepath.Join(root, ".worktools.toml"), []byte(config), 0644)
		if _, e := Discover(root, Builtins()); e == nil {
			t.Fatalf("invalid config accepted: %s", config)
		}
	}
}

func TestUnrelatedAdapterNotCalled(t *testing.T) {
	root := t.TempDir()
	a := adapter{"moon", []string{"Moonfile"}, []string{"moon"}, func(string) ([]Action, error) { t.Fatal("called unrelated adapter"); return nil, nil }}
	r, e := Discover(root, []Adapter{a})
	if e != nil || len(r) != 0 {
		t.Fatal(r, e)
	}
}
