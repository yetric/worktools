package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/yetric/worktools/internal/agent"
)

func TestAgentPreviewAndSyncJSON(t *testing.T) {
	root := t.TempDir()
	original := "# Handwritten\nKeep project-specific rules.\n"
	write(t, root, "AGENTS.md", original)
	out, code := invoke(t, root, "agent", "preview", "--tools", "dev,pod", "--files", "AGENTS.md", "--json", "--check")
	var report struct {
		Applied, Changed bool
		Selection        agent.Selection
		Files            []struct{ Path, Diff string }
	}
	if e := json.Unmarshal([]byte(out), &report); e != nil || code != 1 || report.Applied || !report.Changed || len(report.Files) != 2 {
		t.Fatal(out, code, e)
	}
	if _, e := os.Stat(filepath.Join(root, ".worktools")); !os.IsNotExist(e) {
		t.Fatal("preview wrote state")
	}
	data, _ := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if string(data) != original {
		t.Fatal("preview changed instructions")
	}
	out, code = invoke(t, root, "agent", "sync", "--tools", "dev,pod", "--files", "AGENTS.md", "--json")
	if e := json.Unmarshal([]byte(out), &report); e != nil || code != 0 || !report.Applied {
		t.Fatal(out, code, e)
	}
	out, code = invoke(t, root, "agent", "preview", "--check", "--json")
	if e := json.Unmarshal([]byte(out), &report); e != nil || code != 0 || report.Changed || !reflect.DeepEqual(report.Selection.Tools, []string{"dev", "pod"}) {
		t.Fatal(out, code, e)
	}
}

func TestAgentCommandsRejectInvalidArgumentsWithoutWriting(t *testing.T) {
	for _, args := range [][]string{{"init"}, {"sync", "--check"}, {"preview", "--tools="}, {"sync", "--directory="}, {"sync", "--files", "../AGENTS.md"}, {"unknown"}, {"preview", "--", "touch", "oops"}} {
		root := t.TempDir()
		out, code := invoke(t, root, append([]string{"agent"}, args...)...)
		if code != 2 {
			t.Fatal(args, out, code)
		}
		entries, _ := os.ReadDir(root)
		if len(entries) != 0 {
			t.Fatal("invalid invocation wrote files", args)
		}
	}
}

func TestAgentDiscoveryDoesNotRunToolsOrReadCredentials(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	for _, name := range []string{"make", "docker", "podman", "nerdctl", "systemctl", "psql", "mysql", "sqlite3"} {
		write(t, bin, name, "#!/bin/sh\nprintf 'EXECUTED' > \"$FORBIDDEN_MARKER\"\nexit 1\n")
	}
	t.Setenv("PATH", bin)
	t.Setenv("FORBIDDEN_MARKER", filepath.Join(root, "executed"))
	write(t, root, "Makefile", "test: ## Run project tests\n\ttouch EXECUTED\n")
	write(t, root, ".db.json", `{"connections":{"app":{"engine":"postgres","url_env":"SECRET_DATABASE_URL"}}}`)
	t.Setenv("SECRET_DATABASE_URL", "postgres://user:password@secret-host/project")
	out, code := invoke(t, root, "agent", "preview", "--json")
	if code != 0 || strings.Contains(out, "password") || strings.Contains(out, "secret-host") {
		t.Fatal(out, code)
	}
	if _, e := os.Stat(filepath.Join(root, "executed")); !os.IsNotExist(e) {
		t.Fatal("executed native tool during discovery")
	}
}

func TestAgentMetadataIsAvailableWithoutAProject(t *testing.T) {
	root := t.TempDir()
	for _, args := range [][]string{{"agent", "--help"}, {"complete", "tools"}, {"complete", "agent", "--kind", "commands"}, {"complete", "agent", "--kind", "tools"}, {"complete", "agent", "--kind", "files"}} {
		out, code := invoke(t, root, args...)
		if code != 0 || strings.TrimSpace(out) == "" {
			t.Fatal(args, out, code)
		}
	}
}

func TestChecklistPreservesDefaultsAndHandlesInvalidSelections(t *testing.T) {
	choices := []pickerChoice{{Value: "dev", Label: "Development"}, {Value: "pod", Label: "Containers"}}
	for _, tc := range []struct {
		input          string
		defaults, want []string
	}{
		{"\n", []string{"pod"}, []string{"pod"}},
		{"9\n1\n\n", []string{"pod"}, []string{"dev", "pod"}},
		{"none\n\nall\n\n", nil, []string{"dev", "pod"}},
	} {
		var out bytes.Buffer
		got, e := checklist(bufio.NewReader(strings.NewReader(tc.input)), &out, choices, tc.defaults, "tools")
		if e != nil || !reflect.DeepEqual(got, tc.want) {
			t.Fatal(got, e, out.String())
		}
	}
	if _, e := checklist(bufio.NewReader(strings.NewReader("")), &bytes.Buffer{}, choices, nil, "tools"); e == nil {
		t.Fatal("ignored end of input")
	}
}

func TestMultiPickerPreservesSelectionIdentifiers(t *testing.T) {
	root := t.TempDir()
	capture := filepath.Join(root, "arguments")
	write(t, root, "fzf", "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$FZF_CAPTURE\"\nprintf '2\\tThird label\\n0\\tFirst label\\n'\n")
	t.Setenv("PATH", root)
	t.Setenv("FZF_CAPTURE", capture)
	choices := []pickerChoice{{Value: "dev", Label: "Development"}, {Value: "pod", Label: "Containers"}, {Value: "db", Label: "Databases"}}
	got, e := chooseManyFZF(choices, []string{"dev", "db"}, "tools")
	if e != nil || !reflect.DeepEqual(got, []string{"dev", "db"}) {
		t.Fatal(got, e)
	}
	args, _ := os.ReadFile(capture)
	if !strings.Contains(string(args), "load:pos(1)+select+pos(3)+select+first") {
		t.Fatal(string(args))
	}
}

func TestAgentGuideConfirmationInATerminal(t *testing.T) {
	zsh, e := exec.LookPath("zsh")
	if e != nil {
		t.Skip("Zsh is not installed")
	}
	for _, answer := range []string{"n", "y"} {
		t.Run(answer, func(t *testing.T) {
			root := t.TempDir()
			project := filepath.Join(root, "project")
			bin := filepath.Join(root, "bin")
			original := "# Handwritten\nKeep this.\n"
			write(t, project, "AGENTS.md", original)
			// No fzf on this PATH: exercise the numbered checklist fallback.
			os.MkdirAll(bin, 0755)
			sleep, e := exec.LookPath("sleep")
			if e != nil {
				t.Fatal(e)
			}
			os.Symlink(sleep, filepath.Join(bin, "sleep"))
			harness := write(t, root, "guide.zsh", `zmodload zsh/zpty || exit 90
zpty -b guide "$CLI_BINARY" -test.run=^TestCLIHelper\$ -- agent init --directory "$PROJECT" --tools dev --files AGENTS.md || exit 91
trap 'zpty -d guide 2>/dev/null' EXIT
zpty -w -n guide $'\n\n\n'"$ANSWER"$'\n'
for (( attempt=0; attempt<1000; attempt++ )); do
  while zpty -r guide piece; do print -rn -- "$piece" >> "$OUTPUT"; done
  if ! zpty -t guide; then
    while zpty -r guide piece; do print -rn -- "$piece" >> "$OUTPUT"; done
    exit 0
  fi
  sleep 0.01
done
exit 92
`)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, zsh, "-f", harness)
			cmd.Env = append(os.Environ(), "WORKTOOLS_TEST_HELPER=1", "CLI_BINARY="+os.Args[0], "PROJECT="+project, "ANSWER="+answer, "PATH="+bin, "OUTPUT="+filepath.Join(root, "output"), "TERM=xterm-256color")
			if out, e := cmd.CombinedOutput(); e != nil {
				t.Fatal(string(out), e)
			}
			out, _ := os.ReadFile(filepath.Join(root, "output"))
			data, _ := os.ReadFile(filepath.Join(project, "AGENTS.md"))
			if !strings.Contains(string(out), "Write these changes?") {
				t.Fatal("guide did not reach confirmation", string(out))
			}
			if answer == "n" {
				if string(data) != original {
					t.Fatal("cancelled guide wrote instructions")
				}
				if _, e := os.Stat(filepath.Join(project, agent.SelectionFile)); !os.IsNotExist(e) {
					t.Fatal("cancelled guide saved selections")
				}
			} else if !strings.HasPrefix(string(data), original) || !strings.Contains(string(data), agent.Start) {
				t.Fatal(string(data), string(out))
			}
		})
	}
}
