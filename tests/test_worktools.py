import contextlib
import importlib.util
import io
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch


ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location("worktools", ROOT / "lib/worktools.py")
tools = importlib.util.module_from_spec(spec)
spec.loader.exec_module(tools)


class WorktoolsTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.directory = Path(self.temp.name)

    def config(self, value):
        path = self.directory / ".db.json"
        path.write_text(json.dumps(value))
        return path

    def test_project_lookup_stops_at_git_boundary(self):
        self.config({"connections": {"app": {"engine": "sqlite", "file": "app.db"}}})
        project = self.directory / "project"
        (project / ".git").mkdir(parents=True)
        with self.assertRaisesRegex(ValueError, "No .db.json"):
            tools.database_config(project)

    def test_project_lookup_works_from_subdirectory(self):
        path = self.config({"connections": {"app": {"engine": "sqlite", "file": "app.db"}}})
        child = self.directory / "src"
        child.mkdir()
        self.assertEqual(tools.database_config(child)[0], path)

    def test_multiple_connections_require_explicit_selection(self):
        self.config({"connections": {name: {"engine": "sqlite", "file": name + ".db"} for name in ["app", "reporting"]}})
        args = tools.parser_for("db").parse_args(["--directory", str(self.directory), "connect"])
        with self.assertRaisesRegex(ValueError, "Select a connection"):
            tools.db(args)

    def test_info_does_not_expose_connection_url(self):
        path = self.config({"connections": {"app": {"engine": "postgres", "url_env": "TEST_DB_URL"}}})
        entry = {"engine": "postgres", "url_env": "TEST_DB_URL"}
        with patch.dict(os.environ, {"TEST_DB_URL": "postgres://user:example-password@localhost/app"}):
            record = tools.connection_record("app", entry, path, "app")
        self.assertTrue(record["ready"])
        self.assertNotIn("example-password", json.dumps(record))
        self.assertNotIn("postgres://", json.dumps(record))

    def test_postgres_credentials_stay_out_of_argv(self):
        url = "postgresql://user:example-password@localhost/app?sslmode=require"
        with patch.dict(os.environ, {"TEST_DB_URL": url}):
            argv, env, stdin = tools.db_command({"engine": "postgres", "url_env": "TEST_DB_URL"}, self.directory / ".db.json")
        self.assertEqual(env["PGDATABASE"], url)
        self.assertNotIn("example-password", " ".join(argv))
        self.assertIsNone(stdin)

    def test_mysql_url_is_parsed_and_password_is_not_an_argument(self):
        with patch.dict(os.environ, {"TEST_DB_URL": "mysql://user:p%40ss@localhost:3307/app"}):
            argv, env, _ = tools.db_command({"engine": "mysql", "url_env": "TEST_DB_URL"}, self.directory / ".db.json")
        self.assertEqual(env["MYSQL_PWD"], "p@ss")
        self.assertIn("3307", argv)
        self.assertNotIn("p@ss", argv)

    def test_sqlite_never_creates_a_missing_database(self):
        with self.assertRaisesRegex(ValueError, "does not exist"):
            tools.db_command({"engine": "sqlite", "file": "missing.db"}, self.directory / ".db.json")
        self.assertFalse((self.directory / "missing.db").exists())

    def test_sqlite_path_is_relative_to_config(self):
        database = self.directory / "app.db"
        database.touch()
        argv, _, _ = tools.db_command({"engine": "sqlite", "file": "app.db"}, self.directory / ".db.json")
        self.assertEqual(argv[-1], str(database))

    def test_dotfiles_reports_old_checkout_and_broken_links(self):
        old = self.directory / "old.zshrc"
        old.touch()
        config = self.directory / ".zshrc"
        config.symlink_to(old)
        record = tools.dot_record("shell", self.directory)
        self.assertEqual(record["source"], str(old))
        self.assertFalse(record["managed"])
        old.unlink()
        self.assertEqual(tools.dot_record("shell", self.directory)["status"], "broken")

    def test_service_names_reject_flags_and_patterns(self):
        for value in ["--all", "*.service", "foo bar", "foo\nbar", ""]:
            with self.assertRaises(ValueError):
                tools.unit_name(value)
        with patch.object(tools, "service_aliases", return_value={"redis": "project-redis.service"}):
            self.assertEqual(tools.unit_name("redis"), "project-redis.service")
        self.assertEqual(tools.unit_name("backup.timer"), "backup.timer")

    def test_picker_refuses_noninteractive_input(self):
        with patch.object(tools.sys, "stdin", io.StringIO()):
            with self.assertRaisesRegex(ValueError, "requires a terminal"):
                tools.choose(["app"], "connection")

    def test_numbered_picker_without_fzf(self):
        with contextlib.redirect_stdout(io.StringIO()), \
             patch.object(tools.sys.stdin, "isatty", return_value=True), \
             patch.object(tools.sys.stdout, "isatty", return_value=True), \
             patch.object(tools.shutil, "which", return_value=None), \
             patch("builtins.input", side_effect=["no", "2"]):
            self.assertEqual(tools.choose(["app", "reporting"], "connection"), "reporting")

    def test_service_scope_and_status_json(self):
        args = tools.parser_for("svc").parse_args(["--system", "status", "redis", "--json"])
        with patch.object(tools, "capture", return_value="Id=db-redis-start.service\nLoadState=loaded\nActiveState=inactive\n") as capture, \
             contextlib.redirect_stdout(io.StringIO()) as output:
            self.assertEqual(tools.svc(args), 0)
        self.assertIn("--system", capture.call_args.args[0])
        self.assertEqual(json.loads(output.getvalue())["ActiveState"], "inactive")

    def test_service_aliases_are_explicit_local_configuration(self):
        directory = self.directory / "worktools"
        directory.mkdir()
        (directory / "services.json").write_text('{"aliases":{"redis":"project-redis.service"}}')
        with patch.dict(os.environ, {"XDG_CONFIG_HOME": str(self.directory)}):
            self.assertEqual(tools.unit_name("redis"), "project-redis.service")

    def test_missing_service_status_json_returns_nonzero(self):
        args = tools.parser_for("svc").parse_args(["status", "unknown", "--json"])
        with patch.object(tools, "capture", return_value="LoadState=not-found\n"), \
             contextlib.redirect_stdout(io.StringIO()):
            self.assertEqual(tools.svc(args), 1)

    def test_native_client_exit_code_and_stdout_are_preserved(self):
        fake = self.directory / "psql"
        fake.write_text('#!/bin/sh\nprintf "native output\\n"\nexit 7\n')
        fake.chmod(0o755)
        self.config({"connections": {"app": {"engine": "postgres", "url_env": "TEST_DB_URL"}}})
        env = {**os.environ, "TEST_DB_URL": "postgres://user:example-password@localhost/app", "PATH": str(self.directory) + os.pathsep + os.environ["PATH"]}
        result = subprocess.run([str(ROOT / "bin/db"), "--directory", str(self.directory), "connect"], env=env, text=True, capture_output=True)
        self.assertEqual(result.returncode, 7)
        self.assertEqual(result.stdout, "native output\n")
        self.assertNotIn("example-password", result.stderr)

    def test_installer_is_idempotent_and_preserves_conflicts(self):
        prefix = self.directory / "prefix"
        command = ["bash", str(ROOT / "install.sh"), "--prefix", str(prefix)]
        for _ in range(2):
            subprocess.run(command, check=True, capture_output=True)
        installed = prefix / "bin/dots"
        result = subprocess.run([str(installed), "--help"], text=True, capture_output=True)
        self.assertEqual(result.returncode, 0)
        self.assertIn("dotfiles", result.stdout)
        conflict = self.directory / "conflict"
        (conflict / "bin").mkdir(parents=True)
        (conflict / "bin/db").write_text("keep me")
        result = subprocess.run(["bash", str(ROOT / "install.sh"), "--prefix", str(conflict)], capture_output=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual((conflict / "bin/db").read_text(), "keep me")
        self.assertFalse((conflict / "bin/svc").exists())


if __name__ == "__main__":
    unittest.main()
