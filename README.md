# Worktools

Small command families for humans and coding agents. Fewer aliases, useful help,
Zsh completion, explicit operations, and structured inspection output.

This first version provides `svc`, `db`, and `dots` on Linux. It uses Python 3.11+
and the standard library. Native tools do the actual work. It never installs
database clients or changes service state during installation.

## Install

```sh
git clone https://github.com/yetric/worktools.git
cd worktools
./install.sh
```

Installation symlinks the executables into `~/.local/bin`. Keep the checkout in
place; updating it updates the installed commands. Existing unrelated files are
never overwritten. `--prefix /another/prefix` installs elsewhere.

Add the bin directory to `PATH`. For Zsh, add the completion directory to `fpath`
**before** `compinit` or loading Oh My Zsh:

```zsh
path=("$HOME/.local/bin" $path)
fpath=("$HOME/.local/share/zsh/site-functions" $fpath)
# Then run your existing compinit or source Oh My Zsh.
```

Open a new shell. If an existing completion dump is stale, rebuild it with
`autoload -Uz compinit; compinit`.

## Services and timers

```sh
svc                         # Read-only user-service overview
svc list --json
svc status redis
svc logs redis --follow
svc timers
svc failed --json
svc restart redis
svc --system status ssh.service
```

Defaults to systemd **user** units. `--system` must precede the action and explicitly
selects system units; any required authorization belongs to systemd. Short names
default to `.service`; full unit names work too. Optional friendly aliases live in
`~/.config/worktools/services.json` (or `$XDG_CONFIG_HOME/worktools/services.json`):

```json
{"aliases": {"redis": "project-redis.service"}}
```

Use `svc list --aliases --json` to inspect aliases offline. No service
is started implicitly. Ordinary output and statuses come from systemctl/journalctl.
`status --json` returns state properties; an inactive unit is valid data, not a
command failure. Missing units return nonzero. Logs with `--json` use JSON lines.

## Project databases

Create `.db.json` at the project root. Store **environment variable names**, not
passwords or connection URLs, in this file:

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
db                          # List configured connections; no network access
db info app --json          # Metadata and environment-variable readiness
db connect                  # Open the default connection in its native client
db connect reporting
db connect --pick           # Explicit interactive connection selection
db query app --file report.sql
db --directory /path/to/project list --json
```

Searches the current directory and parents, stopping at the repository root.
Relative SQLite paths are relative to `.db.json`. A sole connection is the
implicit default; multiple connections without an explicit default require a name.

Supported engines and clients: Postgres (`psql`), MySQL (`mysql`), SQLite (`sqlite3`).
Export the referenced environment variables yourself or use your existing env
manager. Worktools never sources `.env` files. Inspection output contains no URL
or password, and credentials are passed to native clients through their environment,
not process arguments. Readiness means the setting/file exists, not that a remote
connection was verified. Native clients inherit your environment and client config.

Postgres URLs retain libpq options such as `sslmode`. MySQL supports
`mysql://user:password@host:port/database`; URL query options are rejected rather
than silently discarded. Missing SQLite databases are never created implicitly.
SQL files run as supplied and may change data; Worktools does not infer read-only
intent, migrate schemas, or seed databases.

## Dotfiles

```sh
dots                        # List known config paths and symlink status
dots show ghostty --json     # Show actual source and expected source
dots edit ghostty            # Edit actual source, including links to older checkouts
dots edit --pick
dots check --json            # Diagnose links and supported syntax
dots check shell
dots diff                   # Git diff of tracked working-tree changes
```

Defaults to `~/dotfiles`; set `DOTFILES` to use another checkout. Known names are
`shell`, `git`, `profile`, `ghostty`, `alacritty`, `zed`, `starship`, `fonts`, and
`mise`. Application paths use the conventional locations under your home directory.
An alternate `XDG_CONFIG_HOME` is not currently supported. `dots check` reports
missing configs, broken links, and configs pointing outside the selected checkout.
Optional configs that are absent count as issues; check specific names when needed.
Syntax checks currently cover TOML, fontconfig XML, and Zsh when installed. It does
not claim to validate every application's semantics. `edit` honors `VISUAL`, then
`EDITOR`, then `vi`. It never relinks or overwrites configs automatically.

## Shell experience and progressive enhancement

- **Tab:** described actions/options, config names, database names, and service units.
- **Grey suggestions/typeahead:** provided by your shell, e.g. Zsh autosuggestions
  and history. Worktools works with them without requiring a particular plugin.
- **Optional selection:** `--pick` uses `fzf` if present, otherwise a numbered menu.
  It requires a terminal. Scripts and agents should pass explicit names.
- **No forced dependencies:** `bat`, `eza`/`exa`, and zoxide can remain your preferred
  viewer, listing, and navigation tools. The current commands do not need to wrap
  them merely to claim integration.

Every action has `--help`. Inspection commands offer `--json`; diagnostics use
stderr. Execution delegates with `exec`, preserving native output, exit status,
and signal behavior. Worktools usage/config errors exit 2; `dots check` exits 1
when it finds issues. Global options such as `--system` and `--directory` precede
the action; action options such as `--json` follow it.

Future command families such as `dev`, `pod`, and `agent` are not implemented here
yet. The interface will grow from repeated workflows rather than alias count.

## Development

```sh
python3 -m unittest discover -s tests -v
bash -n install.sh
zsh -n completions/_worktools
```

Tests use temporary directories and fake clients; they do not connect to databases
or change services. MIT licensed.
