package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func write(t *testing.T, root, name, text string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if e := os.MkdirAll(filepath.Dir(path), 0755); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(path, []byte(text), 0755); e != nil {
		t.Fatal(e)
	}
	return path
}

// Run the CLI in a subprocess because successful native dispatch replaces the process.
func TestCLIHelper(t *testing.T) {
	if os.Getenv("WORKTOOLS_TEST_HELPER") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Exit(Run(append([]string{"worktools"}, os.Args[i+1:]...)))
		}
	}
	os.Exit(99)
}
func invoke(t *testing.T, directory string, args ...string) (string, int) {
	t.Helper()
	argv := append([]string{"-test.run=^TestCLIHelper$", "--"}, args...)
	cmd := exec.Command(os.Args[0], argv...)
	cmd.Dir = directory
	cmd.Env = append(os.Environ(), "WORKTOOLS_TEST_HELPER=1")
	b, e := cmd.CombinedOutput()
	if e == nil {
		return string(b), 0
	}
	if exit, ok := e.(*exec.ExitError); ok {
		return string(b), exit.ExitCode()
	}
	t.Fatal(e)
	return "", 99
}

func TestParsingPreservesNativeArguments(t *testing.T) {
	o, e := parse([]string{"test", "--dry-run", "--json", "--", "--filter", "two words"}, []string{"--dry-run", "--json"}, nil)
	if e != nil || !reflect.DeepEqual(o.Native, []string{"--filter", "two words"}) || !o.Flags["--json"] {
		t.Fatal(o, e)
	}
	if _, e := parse([]string{"--unknown"}, nil, nil); e == nil {
		t.Fatal("accepted unknown option")
	}
}

func TestDevDryRunArgumentForwarding(t *testing.T) {
	for _, runner := range []string{"npm", "pnpm", "composer"} {
		t.Run(runner, func(t *testing.T) {
			root := t.TempDir()
			if runner == "composer" {
				write(t, root, "composer.json", `{"scripts":{"test":"phpunit"}}`)
			} else {
				write(t, root, "package.json", `{"packageManager":"`+runner+`@1","scripts":{"test":"vitest"}}`)
			}
			out, code := invoke(t, root, "dev", "test", "--dry-run", "--json", "--", "--filter", "two words")
			if code != 0 {
				t.Fatal(out, code)
			}
			var r struct{ Command []string }
			if e := json.Unmarshal([]byte(out), &r); e != nil {
				t.Fatal(out, e)
			}
			want := []string{runner, "run", "test"}
			if runner != "pnpm" {
				want = append(want, "--")
			}
			want = append(want, "--filter", "two words")
			if !reflect.DeepEqual(r.Command, want) {
				t.Fatal(r.Command, want)
			}
		})
	}
}

func TestNativeDevExitStatusAndWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	write(t, root, ".worktools.toml", "[dev.actions.start]\ncommand=['./runner','argument with spaces']\n")
	write(t, root, "runner", "#!/bin/sh\nprintf '%s\\n' \"$PWD\" \"$1\" \"$2\"\nexit 9\n")
	out, code := invoke(t, root, "dev", "--", "forwarded argument")
	if code != 9 || !strings.Contains(out, root+"\nargument with spaces\nforwarded argument\n") {
		t.Fatal(out, code)
	}
}

func TestInspectionDoesNotExecuteProjects(t *testing.T) {
	root := t.TempDir()
	write(t, root, ".worktools.toml", "[dev.actions.test]\ncommand=['touch','DONT_CREATE']\n")
	write(t, root, "worktools_plugin.py", "raise RuntimeError('never import')")
	for _, args := range [][]string{{"dev", "list", "--json"}, {"dev", "explain", "test", "--json"}, {"dev", "test", "--dry-run"}, {"complete", "dev"}} {
		out, code := invoke(t, root, args...)
		if code != 0 {
			t.Fatal(out, code)
		}
	}
	if _, e := os.Stat(filepath.Join(root, "DONT_CREATE")); !os.IsNotExist(e) {
		t.Fatal("inspection executed project code")
	}
}

func TestNoninteractivePickerDoesNotPrompt(t *testing.T) {
	root := t.TempDir()
	write(t, root, ".worktools.toml", "[dev.actions.test]\ncommand=['echo','test']\n")
	out, code := invoke(t, root, "dev", "--pick")
	if code != 2 || !strings.Contains(out, "requires a terminal") {
		t.Fatal(out, code)
	}
}

func TestDatabaseMetadataAndNativeCredentials(t *testing.T) {
	root := t.TempDir()
	write(t, root, ".db.json", `{"connections":{"app":{"engine":"postgres","url_env":"TEST_DB_URL"}}}`)
	t.Setenv("TEST_DB_URL", "postgres://user:example-password@localhost/app")
	out, code := invoke(t, root, "db", "info", "--json")
	if code != 0 || strings.Contains(out, "example-password") || strings.Contains(out, "postgres://") {
		t.Fatal(out, code)
	}
	write(t, root, "psql", "#!/bin/sh\nprintf 'native output\\n'\nexit 7\n")
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, code = invoke(t, root, "db", "connect")
	if code != 7 || out != "native output\n" {
		t.Fatal(out, code)
	}
}

func TestDatabaseCommands(t *testing.T) {
	t.Setenv("TEST_DB_URL", "mysql://user:p%40ss@localhost:3307/app")
	argv, env, _, e := databaseCommand(connection{Engine: "mysql", URLEnv: "TEST_DB_URL"}, "/tmp/.db.json", "")
	if e != nil || strings.Contains(strings.Join(argv, " "), "p@ss") || !strings.Contains(strings.Join(env, "\n"), "MYSQL_PWD=p@ss") {
		t.Fatal(argv, e)
	}
	t.Setenv("TEST_DB_URL", "mysql://user:example-password@localhost:BAD/app")
	_, _, _, e = databaseCommand(connection{Engine: "mysql", URLEnv: "TEST_DB_URL"}, "/tmp/.db.json", "")
	if e == nil || strings.Contains(e.Error(), "example-password") {
		t.Fatal(e)
	}
	root := t.TempDir()
	_, _, _, e = databaseCommand(connection{Engine: "sqlite", File: "missing.db"}, filepath.Join(root, ".db.json"), "")
	if e == nil {
		t.Fatal("created missing sqlite database")
	}
}

func TestDatabaseRootAndAmbiguousConnections(t *testing.T) {
	root := t.TempDir()
	write(t, root, ".db.json", `{"connections":{"a":{"engine":"sqlite","file":"a.db"},"b":{"engine":"sqlite","file":"b.db"}}}`)
	out, code := invoke(t, root, "db", "connect")
	if code != 2 || !strings.Contains(out, "explicitly") {
		t.Fatal(out, code)
	}
	nested := filepath.Join(root, "nested")
	os.MkdirAll(filepath.Join(nested, ".git"), 0755)
	if _, _, e := loadDatabase(nested); e == nil {
		t.Fatal("crossed git boundary")
	}
}

func TestDotsSymlinks(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	source := write(t, root, ".zshrc", "# shell")
	os.Symlink(source, filepath.Join(home, ".zshrc"))
	r, e := dotInfo("shell", home, root)
	if e != nil || !r.Managed || r.Status != "linked" {
		t.Fatal(r, e)
	}
	other := t.TempDir()
	r, e = dotInfo("shell", home, other)
	if e != nil || r.Managed {
		t.Fatal("failed to detect competing checkout", r, e)
	}
	os.Remove(source)
	r, e = dotInfo("shell", home, root)
	if e != nil || r.Status != "broken" {
		t.Fatal(r, e)
	}
}

func TestEditorArguments(t *testing.T) {
	argv, e := splitEditor(`editor --title 'two words'`)
	if e != nil || !reflect.DeepEqual(argv, []string{"editor", "--title", "two words"}) {
		t.Fatal(argv, e)
	}
	if _, e := splitEditor(`editor "unterminated`); e == nil {
		t.Fatal("accepted malformed editor")
	}
}

func TestServiceAliasesAndUnitValidation(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	write(t, root, "worktools/services.json", `{"aliases":{"redis":"project-redis.service"}}`)
	got, e := unitName("redis")
	if e != nil || got != "project-redis.service" {
		t.Fatal(got, e)
	}
	for _, name := range []string{"--all", "*.service", "foo bar", "foo\nbar", ""} {
		if _, e := unitName(name); e == nil {
			t.Fatalf("accepted %q", name)
		}
	}
}

func TestServiceJSONScope(t *testing.T) {
	root := t.TempDir()
	write(t, root, "systemctl", "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$TEST_CAPTURE\"\nprintf 'Id=redis.service\\nLoadState=loaded\\nActiveState=inactive\\n'\n")
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TEST_CAPTURE", filepath.Join(root, "args"))
	out, code := invoke(t, root, "svc", "--system", "status", "redis", "--json")
	if code != 0 || !strings.Contains(out, "inactive") {
		t.Fatal(out, code)
	}
	b, _ := os.ReadFile(filepath.Join(root, "args"))
	if !strings.Contains(string(b), "--system") {
		t.Fatal(string(b))
	}
}

func TestCompletionEscapesScriptColons(t *testing.T) {
	root := t.TempDir()
	write(t, root, "package.json", `{"scripts":{"test:unit":"test"}}`)
	out, code := invoke(t, root, "complete", "dev")
	if code != 0 || !strings.Contains(out, `test\:unit:`) {
		t.Fatal(out, code)
	}
}
