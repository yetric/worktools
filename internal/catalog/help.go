package catalog

import (
	"fmt"
	"strings"
)

func Help(name string) string {
	tool, ok := Get(name)
	if !ok {
		return ""
	}
	var out strings.Builder
	fmt.Fprintf(&out, "Usage: %s\n\n%s\n\nCommands:\n", usage[name], tool.Description)
	for _, command := range tool.Commands {
		fmt.Fprintf(&out, "  %-12s %s\n", command.Name, command.Description)
	}
	out.WriteString("\n" + options[name])
	for _, note := range tool.Notes {
		out.WriteString("\n" + note + "\n")
	}
	return out.String()
}

var usage = map[string]string{
	"dev":   "dev [--directory DIR] [ACTION|list|explain ACTION] [OPTIONS] [-- RUNNER_ARGS...]",
	"pod":   "pod [OPTIONS] ACTION [ARGS...] [-- NATIVE_ARGS...]",
	"svc":   "svc [--system] ACTION [UNIT] [OPTIONS]",
	"db":    "db [--directory DIR] [list|info NAME|connect NAME|query NAME --file SQL] [OPTIONS]",
	"dots":  "dots [list|show NAME|edit NAME|check [NAME]|diff [NAME]] [OPTIONS]",
	"agent": "worktools agent [init|preview|sync] [DIRECTORY] [OPTIONS]",
}

var options = map[string]string{
	"dev": `Options:
  --directory DIR  Project directory
  --json           Structured inspection or dry-run output
  --dry-run        Explain without executing
  --pick           Choose a project action (fzf when available)

Named actions run directly; noninteractive bare dev defaults to start.
Supports Make, JavaScript, Composer, Go, Rust, Java and Python tooling.
Project overrides: .worktools.toml. Discovery never runs project commands.
`,
	"pod": `Options:
  --provider NAME  docker, podman, or nerdctl
  --directory DIR  Working directory
  --file FILE      Override Compose files (project operations only)
  -d, --detach     Run up in the background
  -f, --follow     Follow logs
  -a, --all        Include stopped containers (containers only)
  --dry-run        Explain without running
  --json           Structured inspection, services or container summaries
  --pick           Choose an operation interactively

Bare pod opens a picker in a terminal; otherwise it defaults to project ps.
Native flags follow --, for example: pod containers -- --filter name=app
An explicit unavailable provider fails rather than falling back.
`,
	"svc": `Options:
  --system         Select system units instead of user units
  --json           Structured inspection (logs use JSON lines)
  --aliases        Inspect configured aliases offline (list only)
  -f, --follow     Follow logs
  -n, --lines N    Number of log entries
  --since TIME     Log entries since a time

Aliases: $XDG_CONFIG_HOME/worktools/services.json.
`,
	"db": `Options:
  --directory DIR  Project directory
  --json           Structured list/info metadata
  --pick           Select a connection interactively (connect only)
  --file SQL       SQL file to execute (query only)

Configure .db.json with a default and named connections.
Engines: postgres, mysql, sqlite. SQLite uses file instead of url_env.
connect uses an explicit connection or the configured/sole default.
`,
	"dots": `Options:
  --json           Structured list/show/check output
  --pick           Select a configuration interactively (edit only)

edit uses VISUAL, EDITOR, then vi. check exits 1 on findings.
Checks cover links, TOML, fontconfig XML and Zsh syntax.
`,
	"agent": `Options:
  --directory DIR  Project directory (defaults to the current directory)
  --tools LIST     Comma-separated tools: dev,pod,svc,db,dots
  --files LIST     Comma-separated targets: AGENTS.md,CLAUDE.md
  --json           Structured plan/result (preview and sync)
  --check          Exit 1 when preview finds changes; never writes

Selections are saved in .worktools/agent.toml. Handwritten instructions are preserved.
Shared symlinks inside the project are updated once; external/dangling links and
malformed managed sections are rejected before any writes. Legacy dotfiles-aliases
sections are migrated in the previewed update. Deselected managed sections are removed.
Bare worktools agent opens init in a terminal; otherwise it previews.
Discovery reads files and metadata without running project commands or clients.
`,
}
