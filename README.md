# Worktools

Small command families for humans and coding agents: `dev`, `pod`,
`agent` (planned), `svc`, `db`, and `dots`. Fewer aliases, discoverable actions,
Zsh completion, optional pickers, and structured inspection output.

Worktools is written in Go and builds to one binary. Linux is currently supported.
Python is not needed to install or run Worktools. It may be the underlying runner
for a Python project's actions, just as Java or Node may be for other projects.

## Install

```sh
git clone https://github.com/yetric/worktools.git
cd worktools
./install.sh
```

Building requires Go 1.23+. The installer builds `build/worktools`, then symlinks
`worktools`, `dev`, `pod`, `svc`, `db`, and `dots` into `~/.local/bin`. Keep the checkout
in place; run `./install.sh` after updating source to rebuild the binary. Existing
unrelated commands are never overwritten. Pinned Go modules may be downloaded
when building. Native service tools and database clients are not installed.

Options: `--prefix DIRECTORY` chooses another install prefix;
`--binary FILE` installs an already-built executable without requiring Go.

Add `~/.local/bin` to `PATH`. Add the installed completion directory to Zsh's
`fpath` **before** `compinit` or loading Oh My Zsh:

```zsh
path=("$HOME/.local/bin" $path)
fpath=("$HOME/.local/share/zsh/site-functions" $fpath)
# Then initialize your existing completion framework.
```

Open a new shell. If an older shell function named `dev` or `db` exists, remove
it from your configuration so the executable can be found. Worktools also supports
`worktools dev …`, `worktools db …`, etc., without relying on the short names.

## Project development

```sh
dev                           # Select an action interactively in a terminal
dev start                     # Start development directly
dev list                      # Available actions and descriptions
dev list --json               # Actions, candidates, and resolution for agents
dev explain test              # Selected command, directory, and reason
dev test
dev test --dry-run --json
dev test -- --filter "two words"
dev --directory /path/to/project build
```

Bare `dev` opens an action picker with descriptions and resolution status. It uses
fzf if installed, otherwise a numbered menu. Named actions never open a picker.
Noninteractive bare `dev` defaults to `start`; scripts should use explicit actions.
`--pick` explicitly requests selection. `--dry-run` and `--json` never implicitly
open a picker. Inspection and completion do not execute project commands.

Each action is resolved independently. One project can use Composer for tests,
Make for checks, and pnpm for startup. The executable being installed does not
choose a runner. Conflicting candidates are reported as ambiguous.

| Environment | Detection and actions |
| --- | --- |
| Make | Explicit targets and literal includes; `##` comments supply descriptions |
| JavaScript | npm/pnpm/Yarn/Bun package scripts; `packageManager`, then lockfiles, then npm |
| PHP | Composer scripts and their descriptions |
| Go | `test`, `check` (vet), `build`; startup for a root main package or single command under `cmd/` |
| Rust | Cargo `test`, `check`, `build`, `clean`; startup for a binary or `default-run` |
| Maven | `test`, `check` (verify), `build` (package), `clean`; declared Spring Boot startup |
| Gradle | Conventional lifecycle tasks; application/Spring Boot startup when detected |
| Python | Declared pytest, recognized unittest, Ruff/mypy, packaging, and Django startup |

Make and package-script `dev` targets become the canonical `start` action, taking
precedence over the same runner's `start` target. Other actions such as `lint` and
`format` are retained. `list` and `explain` are reserved inspection commands.
Java uses project `mvnw`/`gradlew` wrappers when present. Python supports uv,
Poetry, Pipenv, a local `.venv`, or Python directly. Multiple managers remain
ambiguous. uv uses `--no-sync --no-python-downloads`; other commands retain their
native behavior, including build dependency resolution. See the
[uv reference](https://docs.astral.sh/uv/reference/cli/) for those options.

### Explicit project choices

Use `.worktools.toml` when detection is ambiguous or a project has custom behavior:

```toml
# Optional preference for all detected, unmapped actions:
# [dev]
# runner = "pnpm"

[dev.actions.start]
command = ["pnpm", "run", "dev"]
description = "Start the frontend"

[dev.actions.test]
command = ["composer", "run", "test"]
directory = "backend"
description = "Run backend tests"

[dev.actions.check]
command = ["make", "check"]

[dev.actions.unusual]
command = ["unheard-of-tool", "verify"]
argument_separator = ["--"]
description = "Run our unusual environment's checks"
```

Mappings override discovery per action. An explicit `dev.runner` applies strictly:
if it has no action, that action is unavailable until mapped. Argument arrays
are executed directly, without a shell or interpolation. Use an explicit script
for shell pipelines. Native arguments follow `--`; npm and Composer receive their
required separator automatically. A mapping's `argument_separator` overrides this.

An ancestor `.worktools.toml` takes priority within the repository boundary;
otherwise the nearest supported manifest determines the project root. Actions
run from that root or their explicitly mapped directory. An action is `ready`
when it resolves to one command; `executable_available` separately reports whether
its executable is installed. Missing tools are never installed automatically.

Discovery is intentionally static. Dynamic Make targets, conditional build logic,
Gradle custom tasks, inherited plugins, workspace selection, and unknown test
frameworks can need mappings. Worktools never runs `make -p` or imports application
code to enumerate actions. Python unittest recognition uses static import lines;
it is a heuristic, not a full interpreter. No generic test command is guessed merely
because a `tests/` directory exists.

### New environments

Each built-in provider owns an independent Go package and implements a small interface. A new environment supplies
marker filenames, runner names, and action records. The CLI, picker, JSON output,
execution, and Tab completion use those records without environment-specific changes.

See [Adding a provider](docs/adapters.md) and [CONTRIBUTING.md](CONTRIBUTING.md). Third-party runtime plugins, Python
entry points, and project-local executable plugins are not loaded. A one-off
project needs only an explicit command mapping; a broadly useful environment can
contribute a compiled adapter.

## Project containers

```sh
pod providers --json          # Inspect Docker, Podman, and nerdctl availability
pod --provider podman ps
pod containers                # Across projects: names, status, ports
pod containers --all          # Include stopped containers
pod containers --json         # Consistent summary for agents and scripts
pod up --detach
pod logs app --follow
pod exec app -- sh
pod services --json
pod explain up --json         # Show selected backend and exact argument array
pod down --dry-run
```

Each backend is an independent, compiled provider. Project operations use its Compose
interface: `up`, `down`, `ps`, `logs`, `exec`, `restart`, `build`, and `services`.
`pod` opens an operation picker in a terminal; otherwise it defaults to `ps`.
Native flags follow `--`, for example `pod up -- --build`. Arguments, working
directory, native contexts, and environment are preserved.

`containers` runs the selected engine's native `ps` without requiring Compose or
a project manifest. It lists across projects in the current engine/context/namespace;
`pod ps` lists only the Compose project. Native filters and output overrides work
after `--`: `pod containers -- --filter name=app`, `-- --quiet`, or
`-- --format TEMPLATE`. Configured Compose files do not affect `containers`;
explicit `--file` is rejected to prevent accidental scope confusion.

`containers --json` returns one object with `provider`, `scope`, `context`, and
`containers`. Each summary has `id` (full ID), `name`, `status`, and `ports` strings;
an empty engine returns `containers: []`. Status and ports retain native display
text; they are not normalized state enums or parsed port mappings. No container
command, environment, or labels are included. Native filters work with JSON;
native `--format`/`--quiet` conflict with it and are rejected. Engine errors retain
their native exit code and do not produce a partial JSON result.

Selection order is `--provider`, `WORKTOOLS_POD_PROVIDER`, project configuration,
personal default, then the sole available provider. Multiple available providers require a choice;
the interactive picker can make a temporary choice. An explicit choice never
falls back to another engine. Optional project configuration:

```toml
[pod]
provider = "podman"
# Optional; paths are relative to this configuration:
files = ["compose.yaml", "compose.dev.yaml"]
```

A personal default uses `[pod] provider = "podman"` in
`$XDG_CONFIG_HOME/worktools/config.toml` (normally `~/.config/worktools/config.toml`).
Project preferences override this default. Explicit unavailable choices never fall
back. `pod providers --json` reports CLI presence in `available`, Compose in
`compose_available`, and supported operations in `operations`. A CLI without Compose
can still list containers; project operations are absent from its picker/completion.

Configuration is found in parent directories up to the Git boundary. `--file FILE`
overrides configured files; otherwise Compose performs native file discovery from
the working directory, including `COMPOSE_FILE`. Podman uses `podman compose` and
honors its configured external Compose provider. Detection checks executable presence
and Compose support, not daemon readiness. Worktools never starts VMs/daemons or
switches contexts automatically. Service completion runs read-only
`compose config --services`; inspection does not print endpoint credentials.

Docker-compatible environments use the Docker provider. See the
[container-provider guide](docs/container-providers.md) to contribute a new backend.
Server tools such as nginx and PM2 are being considered separately in
[issue #1](https://github.com/yetric/worktools/issues/1).

## Services and timers

```sh
svc                         # Read-only user-service overview
svc list --json
svc status redis
svc logs redis --follow
svc restart redis
svc timers
svc failed --json
svc --system status ssh.service
```

Uses systemd user units by default. `--system` explicitly selects system units;
any authorization belongs to systemd. Short names get a `.service` suffix.
Friendly names are optional in `$XDG_CONFIG_HOME/worktools/services.json`
(default `~/.config/worktools/services.json`):

```json
{"aliases": {"redis": "project-redis.service"}}
```

`svc list --aliases --json` inspects these aliases offline. Nothing is started
implicitly. `status --json` reports state properties: an inactive unit is valid
data; a missing unit returns nonzero. Logs with `--json` use JSON lines.

## Project databases

Create `.db.json` at the project root. Reference exported environment variables
instead of storing passwords or URLs in the configuration:

```json
{
  "default": "app",
  "connections": {
    "app": {"engine": "postgres", "url_env": "DATABASE_URL"},
    "reporting": {"engine": "mysql", "url_env": "REPORTING_DATABASE_URL"},
    "local": {"engine": "sqlite", "file": "data/local.db"}
  }
}
```

```sh
db                          # List metadata, without connecting
db info app --json
db connect                  # Native client for the selected default
db connect reporting
db connect --pick
db query app --file report.sql
db --directory /path/to/project list --json
```

Searches parent directories up to the repository boundary. SQLite paths are
relative to `.db.json`. A sole connection is the implicit default; otherwise
select one explicitly or configure a default.

Native clients: Postgres `psql`, MySQL `mysql`, SQLite `sqlite3`. Worktools never
sources `.env` files. Inspection output contains no URL/password; clients receive
credentials in their environment, not process arguments. Readiness means settings
or files exist, not that a remote connection was verified. Clients inherit your
environment and their own configuration. Postgres preserves libpq URL options.
MySQL accepts `mysql://user:password@host:port/database`; query options are rejected.
Missing SQLite files are never created implicitly. SQL files run as supplied and
may modify data; Worktools does not infer read-only intent or run migrations.

## Dotfiles

```sh
dots
dots show ghostty --json
dots edit ghostty
dots edit --pick
dots check --json
dots check shell
dots diff
```

Defaults to `~/dotfiles`; `DOTFILES` selects another checkout. Names include
`shell`, `git`, `profile`, `ghostty`, `alacritty`, `zed`, `starship`, `fonts`, `mise`.
Application paths use conventional locations under your home directory;
an alternate `XDG_CONFIG_HOME` for application configs is not supported yet.

Inspection reports actual symlink sources, including links to older checkouts.
Checks cover missing/broken/unmanaged links, TOML, fontconfig XML, and Zsh syntax
when Zsh is installed. Optional configs that are absent count as issues; select
specific names when appropriate. `edit` honors `VISUAL`, then `EDITOR`, then `vi`,
with simple quoted arguments and no shell expansion. It edits the actual source.
`diff` delegates to Git's tracked working-tree diff. No automatic relinking.

## Shell experience and automation

- Tab completion shows actions/options, project actions, configs, units, and connections.
  Type `dev ` followed by Tab (with a space) to see project actions; `dev go-`
  followed by Tab narrows them by prefix. Run `exec zsh` after installing shell
  integration so an existing terminal loads the new completion function.
- Grey typeahead/history suggestions come from your shell, e.g. Zsh autosuggestions.
- Explicit `--pick` uses fzf when available and a numbered menu otherwise.
- `bat`, `eza`/`exa`, and zoxide remain compatible viewers and navigation tools;
  Worktools does not require them or replace standard commands such as `ps`.
- Completion calls `worktools complete TOOL`, using the same read-only discovery
  as inspection. No Python or jq is needed for completion.
- Help is available through `--help`. Inspection commands have `--json`.
  Diagnostics go to stderr; native execution uses process replacement to preserve
  output, exit status, terminal behavior, and signals.

Worktools errors exit 2, `dots check` findings exit 1, unresolved `dev explain`
results exit 1, and successful inspection exits 0. Native commands keep their exit
codes. Native runner flag completion after `dev ACTION --` is not implemented yet.
Linux is the current target; portability beyond Linux needs platform-specific
terminal/process support. Go is a deployment choice, not a guarantee that project
commands are trustworthy: execution runs the explicitly selected project's tools.

## Development

```sh
go test ./...
go test -race ./...
go vet ./...
go build -trimpath -o build/worktools ./cmd/worktools
bash -n install.sh
zsh -n completions/_worktools
```

Or use `dev test` and `dev check` in this repository. Tests use temporary projects,
fake clients, and child processes; they do not connect to databases or mutate
services. Completion regression tests drive real Zsh widgets in isolated
pseudo-terminals when Zsh is installed; CI installs Zsh to run them. MIT licensed.
