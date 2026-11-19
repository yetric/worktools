package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Exercise real Zsh widgets in a pseudo-terminal. Calling a completion function
// outside ZLE misses the words/CURRENT transformations made by _arguments.
func TestZshCompletionContexts(t *testing.T) {
	zsh, e := exec.LookPath("zsh")
	if e != nil {
		t.Skip("Zsh is not installed")
	}
	completionDir, e := filepath.Abs(filepath.Join("..", "..", "completions"))
	if e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct{ name, input, want, discovery string }{
		{"root-agent", "worktools ag", "worktools agent", "complete\ntools"},
		{"agent-action", "worktools agent pr", "worktools agent preview", "commands"},
		{"agent-tool", "worktools agent preview --tools de", "worktools agent preview --tools dev", "tools"},
		{"agent-tool-list", "worktools agent preview --tools dev,po", "worktools agent preview --tools dev,pod", "tools"},
		{"agentsetup-tool", "agentsetup --tools de", "agentsetup --tools dev", "tools"},
		{"agent-file", "worktools agent preview --files CL", "worktools agent preview --files CLAUDE.md", ""},
		{"dev-explain", "dev explain te", "dev explain test", "complete\ndev"},
		{"nested-dev", "worktools dev explain te", "worktools dev explain test", "complete\ndev"},
		{"dev-directory", "dev --directory '/project with spaces' explain te", "dev --directory '/project with spaces' explain test", "--directory\n/project with spaces"},
		{"service-status", "svc status re", "svc status redis.service", "complete\nsvc"},
		{"service-logs", "svc logs --follow re", "svc logs --follow redis.service", "complete\nsvc"},
		{"database", "db --directory /project connect pr", "db --directory /project connect primary", "--directory\n/project"},
		{"dotfiles", "dots show gh", "dots show ghostty", ""},
		{"provider", "pod --provider do", "pod --provider docker", "providers"},
		{"engine-options", "pod containers --a", "pod containers --all", ""},
		{"project-options", "pod up --deta", "pod up --detach", ""},
		{"scope-options", "pod containers --f", "pod containers --f", ""},
		{"container-explain", "pod explain cont", "pod explain containers", "operations"},
		{"explain-only-operations", "pod explain pro", "pod explain pro", "operations"},
		{"project-services", "pod logs --provider docker --file /compose.yaml app", "pod logs --provider docker --file /compose.yaml app", "--provider\ndocker\n--file\n/compose.yaml"},
		{"nested-quoted-file", "worktools pod logs --provider docker --file '/project with spaces/compose.yml' app", "worktools pod logs --provider docker --file '/project with spaces/compose.yml' app", "--file\n/project with spaces/compose.yml"},
		{"assigned-provider", "pod --provider=docker logs app", "pod --provider=docker logs app", "--provider=docker"},
		{"assigned-directory", "dev --directory='/project with spaces' explain te", "dev --directory='/project with spaces' explain test", "--directory\n/project with spaces"},
		{"exec-arguments", "pod exec app a", "pod exec app a", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			bin := filepath.Join(root, "bin")
			write(t, bin, "worktools", `#!/bin/sh
printf '%s\n' "$@" >> "$CALL_CAPTURE"
case "$1 $2" in
  'complete tools') printf 'dev:Development\npod:Containers\nagent:Agent onboarding\n' ;;
  'complete agent')
    case "$4" in
      commands) printf 'init:Choose tools\npreview:Preview changes\nsync:Update instructions\n' ;;
      tools) printf 'dev:Development\npod:Containers\n' ;;
      files) printf 'AGENTS.md\nCLAUDE.md\n' ;;
    esac ;;
  'complete dev') printf 'test:Run tests\nbuild:Build project\n' ;;
  'complete svc') printf 'redis.service\n' ;;
  'complete db') printf 'primary\n' ;;
  'complete pod')
    case "$4" in
      providers) printf 'docker\npodman\n' ;;
      services) printf 'app\napi\ndb\n' ;;
      *) printf 'containers:List running containers\nup:Start project\n' ;;
    esac ;;
  *) exit 99 ;;
esac
`)
			setup := write(t, root, "setup.zsh", `PROMPT='ready> '
RPROMPT=''
stty rows 24 columns 120
fpath=("$COMPLETION_DIR" $fpath)
autoload -Uz compinit
compinit -u -D
compdef _worktools worktools dev pod svc db dots agentsetup
zstyle ':completion:*' menu no
bindkey -e
capture-buffer() { print -r -- "$BUFFER" > "$BUFFER_CAPTURE" }
zle -N capture-buffer
bindkey '^X^V' capture-buffer
print ready > "$READY_FILE"
`)
			harness := write(t, root, "harness.zsh", `zmodload zsh/zpty || exit 90
zpty -b shell "$ZSH_BINARY" -f || exit 91
trap 'zpty -d shell 2>/dev/null' EXIT
zpty -w shell "source ${(q)SETUP_FILE}"
for (( attempt = 0; attempt < 500; attempt++ )); do
  [[ -f "$READY_FILE" ]] && break
  sleep 0.01
done
[[ -f "$READY_FILE" ]] || exit 92
zpty -w -n shell "$COMPLETION_INPUT"$'\t\x18\x16'
for (( attempt = 0; attempt < 500; attempt++ )); do
  [[ -f "$BUFFER_CAPTURE" ]] && exit 0
  sleep 0.01
done
exit 93
`)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, zsh, "-f", harness)
			command.Dir = root
			command.Env = append(os.Environ(), "HOME="+root, "ZDOTDIR="+root, "TERM=xterm-256color", "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "ZSH_BINARY="+zsh, "SETUP_FILE="+setup, "COMPLETION_DIR="+completionDir, "COMPLETION_INPUT="+tc.input, "BUFFER_CAPTURE="+filepath.Join(root, "buffer"), "CALL_CAPTURE="+filepath.Join(root, "calls"), "READY_FILE="+filepath.Join(root, "ready"))
			if output, e := command.CombinedOutput(); e != nil {
				t.Fatalf("Zsh harness: %v\n%s", e, output)
			}
			buffer, e := os.ReadFile(filepath.Join(root, "buffer"))
			if e != nil || strings.TrimSpace(string(buffer)) != tc.want {
				t.Fatalf("completed %q to %q; want %q (%v)", tc.input, buffer, tc.want, e)
			}
			calls, _ := os.ReadFile(filepath.Join(root, "calls"))
			if tc.discovery != "" && !strings.Contains(string(calls), tc.discovery) {
				t.Fatalf("missing discovery arguments %q in %q", tc.discovery, calls)
			}
		})
	}
}
