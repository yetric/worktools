"""Small CLI families for personal services, databases, and dotfiles."""

import argparse
import json
import os
from pathlib import Path
import shlex
import shutil
import subprocess
import sys
import tomllib
from urllib.parse import unquote, urlsplit
import xml.etree.ElementTree as ET


ROOT = Path(os.environ.get("DOTFILES", "~/dotfiles")).expanduser().resolve()
DOTS = {
    "shell": ".zshrc", "git": ".gitconfig", "profile": ".profile",
    "ghostty": ".config/ghostty/config",
    "alacritty": ".config/alacritty/alacritty.toml",
    "zed": ".config/zed/settings.json", "starship": ".config/starship.toml",
    "fonts": ".config/fontconfig/fonts.conf", "mise": ".config/mise/config.toml",
}


def emit(value, as_json=False):
    if as_json:
        print(json.dumps(value, indent=2))
    else:
        rows = value if isinstance(value, list) else [value]
        for row in rows:
            if isinstance(row, dict):
                print("  ".join(f"{key}={value}" for key, value in row.items()))
            else:
                print(row)


def execute(argv, env=None, stdin=None):
    """Replace this process so the native command owns streams, signals, and status."""
    if stdin is not None:
        descriptor = os.open(stdin, os.O_RDONLY)
        os.dup2(descriptor, 0)
        os.close(descriptor)
    os.execvpe(argv[0], argv, os.environ.copy() if env is None else env)


def capture(argv):
    result = subprocess.run(argv, text=True, stdout=subprocess.PIPE)
    if result.returncode:
        raise SystemExit(result.returncode if result.returncode > 0 else 128 - result.returncode)
    return result.stdout


def choose(names, title):
    """An explicitly requested picker; never read stdin in automation."""
    if not sys.stdin.isatty() or not sys.stdout.isatty():
        raise ValueError("--pick requires a terminal; pass a name explicitly in scripts")
    if shutil.which("fzf"):
        result = subprocess.run(["fzf", "--prompt", title + "> "], input="\n".join(names) + "\n",
                                text=True, stdout=subprocess.PIPE)
        if result.returncode:
            raise SystemExit(130 if result.returncode in {1, 130} else result.returncode)
        selected = result.stdout.strip()
        if selected not in names:
            raise ValueError("Invalid selection")
        return selected
    for index, name in enumerate(names, 1):
        print(f"{index}. {name}")
    while True:
        try:
            index = int(input(f"{title} (number, Ctrl+C to cancel): "))
            if 1 <= index <= len(names):
                return names[index - 1]
        except ValueError:
            pass
        print("Choose a number from the list.")


def dot_record(name, home=None):
    path = (home or Path.home()) / DOTS[name]
    linked = path.is_symlink()
    source = path.resolve()
    expected = ROOT / DOTS[name]
    if linked:
        status = "linked" if path.exists() else "broken"
    else:
        status = "local" if path.exists() else "missing"
    return {"name": name, "config": str(path), "source": str(source),
            "status": status, "expected_source": str(expected),
            "managed": linked and source == expected.resolve()}


def dot_check(name):
    record = dot_record(name)
    issues = []
    if record["status"] in {"missing", "broken"}:
        issues.append(record["status"])
    elif not record["managed"]:
        issues.append("configuration is not linked to the active dotfiles checkout")
    if record["status"] not in {"missing", "broken"}:
        path = Path(record["source"])
        try:
            if path.suffix == ".toml":
                tomllib.loads(path.read_text())
            elif name == "fonts":
                ET.parse(path)
            elif name == "shell":
                if not shutil.which("zsh"):
                    issues.append("zsh unavailable; syntax not checked")
                else:
                    result = subprocess.run(["zsh", "-n", str(path)], capture_output=True, text=True)
                    if result.returncode:
                        issues.append(result.stderr.strip())
        except (ValueError, ET.ParseError, OSError) as error:
            issues.append(str(error))
    record["issues"] = issues
    record["ok"] = not issues
    return record


def dots(args):
    action = args.action or "list"
    names = [args.name] if getattr(args, "name", None) else list(DOTS)
    if action in {"list", "show"}:
        rows = [dot_record(name) for name in names]
        emit(rows[0] if action == "show" else rows, args.json)
    elif action == "edit":
        if args.pick:
            args.name = choose(list(DOTS), "configuration")
        if not args.name:
            raise ValueError("Pass a configuration name or use dots edit --pick")
        record = dot_record(args.name)
        if record["status"] in {"missing", "broken"}:
            raise ValueError(f"{args.name}: {record['status']}; inspect with dots show {args.name}")
        editor = shlex.split(os.environ.get("VISUAL") or os.environ.get("EDITOR") or "vi")
        execute([*editor, record["source"]])
    elif action == "check":
        rows = [dot_check(name) for name in names]
        emit(rows, args.json)
        return int(any(not row["ok"] for row in rows))
    elif action == "diff":
        execute(["git", "-C", str(ROOT), "diff", *( ["--", DOTS[args.name]] if args.name else [])])
    return 0


def service_aliases():
    directory = Path(os.environ.get("XDG_CONFIG_HOME", str(Path.home() / ".config")))
    path = directory / "worktools/services.json"
    if not path.exists():
        return {}
    aliases = json.loads(path.read_text()).get("aliases", {})
    if not isinstance(aliases, dict) or any(not isinstance(value, str) for value in aliases.values()):
        raise ValueError(f"{path}: aliases must map names to unit names")
    return aliases


def unit_name(name):
    name = service_aliases().get(name, name)
    if not name or name.startswith("-") or any(c not in "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_@.:\\-" for c in name):
        raise ValueError("Use a single service or timer name (no options or patterns)")
    return name if name.endswith((".service", ".timer")) else name + ".service"


def svc(args):
    scope = "--system" if args.system else "--user"
    action = args.action or "list"
    if action == "list" and getattr(args, "aliases", False):
        emit([{"name": name, "unit": unit} for name, unit in service_aliases().items()], args.json)
        return 0
    if action in {"list", "timers", "failed"}:
        commands = {"list": ["list-units", "--all", "--type=service"],
                    "timers": ["list-timers", "--all"],
                    "failed": ["list-units", "--failed"]}
        argv = ["systemctl", scope, "--no-pager", *commands[action]]
        if args.json:
            emit(json.loads(capture([*argv, "--output=json"])), True)
        else:
            execute(argv)
    elif action == "status":
        unit = unit_name(args.unit)
        if args.json:
            output = capture(["systemctl", scope, "show", unit,
                              "--property=Id,LoadState,ActiveState,SubState,Description,Result"])
            record = dict(line.split("=", 1) for line in output.splitlines() if "=" in line)
            emit(record, True)
            return int(record.get("LoadState") == "not-found")
        execute(["systemctl", scope, "--no-pager", "status", unit])
    elif action == "logs":
        argv = ["journalctl", scope, "--no-pager", "--unit", unit_name(args.unit), "--lines", str(args.lines)]
        if args.follow:
            argv.append("--follow")
        if args.since:
            argv += ["--since", args.since]
        if args.json:
            argv += ["--output=json"]
        execute(argv)
    else:
        execute(["systemctl", scope, action, unit_name(args.unit)])
    return 0


def database_config(directory):
    directory = directory.resolve()
    for parent in [directory, *directory.parents]:
        path = parent / ".db.json"
        if path.exists():
            config = json.loads(path.read_text())
            if not isinstance(config, dict) or not isinstance(config.get("connections"), dict) or not config["connections"]:
                raise ValueError(f"{path}: expected a nonempty connections object")
            for name, entry in config["connections"].items():
                if not isinstance(entry, dict) or entry.get("engine") not in {"postgres", "mysql", "sqlite"}:
                    raise ValueError(f"Connection {name}: engine must be postgres, mysql, or sqlite")
                key = "file" if entry["engine"] == "sqlite" else "url_env"
                if not isinstance(entry.get(key), str) or not entry[key]:
                    raise ValueError(f"Connection {name}: missing {key}")
            if "default" in config and config["default"] not in config["connections"]:
                raise ValueError("Default connection does not exist")
            return path, config
        if (parent / ".git").exists():
            break
    raise ValueError("No .db.json found in this project; see db --help")


def connection_record(name, entry, config_path, default):
    record = {"name": name, "engine": entry["engine"], "default": name == default,
              "config": str(config_path)}
    if entry["engine"] == "sqlite":
        record["file"] = str((config_path.parent / entry["file"]).resolve())
        record["ready"] = Path(record["file"]).is_file()
    else:
        record["url_env"] = entry["url_env"]
        record["ready"] = bool(os.environ.get(entry["url_env"]))
    return record


def db_command(entry, config_path, query_file=None):
    env = os.environ.copy()
    engine = entry["engine"]
    stdin = None
    if engine == "sqlite":
        path = (config_path.parent / entry["file"]).resolve()
        if not path.is_file():
            raise ValueError(f"SQLite file does not exist: {path}")
        argv = ["sqlite3", "-bail", str(path)]
        stdin = query_file
    else:
        raw = env.get(entry["url_env"])
        if not raw:
            raise ValueError(f"Set {entry['url_env']} before connecting")
        # Do not place connection URLs or passwords in process arguments or output.
        try:
            url = urlsplit(raw)
            port = url.port
        except ValueError:
            raise ValueError("Invalid database URL") from None
        if engine == "postgres":
            if url.scheme not in {"postgres", "postgresql"}:
                raise ValueError("Expected a postgres:// or postgresql:// URL")
            env["PGDATABASE"] = raw
            argv = ["psql", "-X", "--set=ON_ERROR_STOP=1"]
            if query_file:
                argv += ["--file", str(query_file)]
        else:
            if url.scheme != "mysql" or url.query or url.fragment:
                raise ValueError("Expected mysql://user:password@host:port/database (URL options are not supported)")
            if not url.hostname or not url.path.strip("/"):
                raise ValueError("MySQL URL requires a host and database name")
            env["MYSQL_PWD"] = unquote(url.password or "")
            argv = ["mysql", "--host", url.hostname, "--port", str(port or 3306),
                    "--user", unquote(url.username or ""), "--database", unquote(url.path.lstrip("/"))]
            stdin = query_file
    return argv, env, stdin


def db(args):
    path, config = database_config(args.directory)
    connections = config["connections"]
    default = config.get("default") or (next(iter(connections)) if len(connections) == 1 else None)
    action = args.action or "list"
    if action == "list":
        emit([connection_record(name, entry, path, default) for name, entry in connections.items()], args.json)
        return 0
    name = args.connection or default
    if getattr(args, "pick", False):
        name = choose(list(connections), "connection")
    if name not in connections:
        raise ValueError("Select a connection explicitly; use db list")
    entry = connections[name]
    if action == "info":
        emit(connection_record(name, entry, path, default), args.json)
        return 0
    query_file = args.file.resolve() if action == "query" else None
    if query_file and not query_file.is_file():
        raise ValueError(f"SQL file does not exist: {query_file}")
    argv, env, stdin = db_command(entry, path, query_file)
    execute(argv, env, stdin)
    return 0


def json_option(parser):
    parser.add_argument("--json", action="store_true", help="structured output (logs use JSON lines)")


def parser_for(tool):
    descriptions = {"dots": "Inspect and edit your dotfiles; never relinks files automatically.",
                    "svc": "Manage user services and timers; use --system explicitly for system units.",
                    "db": "Use project database connections without printing credentials."}
    epilog = None
    if tool == "db":
        epilog = ('Configure .db.json: {"default":"app","connections":{"app":'
                  '{"engine":"postgres","url_env":"DATABASE_URL"}}}. '
                  'Engines: postgres, mysql, sqlite. SQLite uses "file":"relative/path.db" instead of url_env. '
                  'Environment variables must already be exported; .env files are never executed.')
    parser = argparse.ArgumentParser(prog=tool, description=descriptions[tool], epilog=epilog)
    if tool == "svc":
        parser.add_argument("--system", action="store_true", help="use system units instead of user units (before subcommand)")
    if tool == "db":
        parser.add_argument("--directory", type=Path, default=Path.cwd(), help="project directory (before subcommand)")
    sub = parser.add_subparsers(dest="action")
    if tool == "dots":
        for name in ["list", "show", "edit", "check", "diff"]:
            p = sub.add_parser(name, help={"list": "list config paths and symlink status", "show": "inspect one config and its source", "edit": "open the actual source in your editor", "check": "check links and supported syntax (nonzero on issues)", "diff": "show tracked changes in the dotfiles repository"}[name])
            if name in {"show", "edit", "check", "diff"}:
                p.add_argument("name", choices=DOTS, **({"nargs": "?"} if name in {"check", "diff", "edit"} else {}))
            if name == "edit":
                p.add_argument("--pick", action="store_true", help="choose interactively; uses fzf if installed")
            if name in {"list", "show", "check"}:
                json_option(p)
    elif tool == "svc":
        for name in ["list", "status", "start", "stop", "restart", "logs", "timers", "failed"]:
            p = sub.add_parser(name, help=f"{name} {'units' if name in {'list', 'timers', 'failed'} else 'a service or timer'}")
            if name not in {"list", "timers", "failed"}:
                p.add_argument("unit", help="unit name or a configured service alias")
            if name == "list":
                p.add_argument("--aliases", action="store_true", help="list configured aliases without contacting systemd")
            if name in {"list", "status", "logs", "timers", "failed"}:
                json_option(p)
            if name == "logs":
                p.add_argument("--follow", "-f", action="store_true")
                p.add_argument("--lines", "-n", type=int, default=50)
                p.add_argument("--since", help="journal time expression")
    else:
        for name in ["list", "info", "connect", "query"]:
            p = sub.add_parser(name, help={"list": "list configured connections without credentials", "info": "inspect connection readiness without connecting", "connect": "open the native database client", "query": "execute a SQL file with the native client"}[name])
            if name != "list":
                p.add_argument("connection", nargs="?", help="configured name; uses explicit or sole default")
            if name in {"list", "info"}:
                json_option(p)
            if name == "query":
                p.add_argument("--file", type=Path, required=True)
            if name == "connect":
                p.add_argument("--pick", action="store_true", help="choose interactively; uses fzf if installed")
    return parser


def main(tool):
    parser = parser_for(tool)
    args = parser.parse_args()
    # Bare commands provide a read-only overview.
    if not args.action:
        args.json = False
    try:
        return {"dots": dots, "svc": svc, "db": db}[tool](args)
    except (OSError, ValueError) as error:
        print(f"{tool}: {error}", file=sys.stderr)
        return 2
    except KeyboardInterrupt:
        return 130
    except EOFError:
        return 130
