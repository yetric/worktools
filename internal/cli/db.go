package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yetric/worktools/internal/catalog"
)

type connection struct {
	Engine string `json:"engine"`
	URLEnv string `json:"url_env,omitempty"`
	File   string `json:"file,omitempty"`
}
type databaseConfig struct {
	Default     string                `json:"default"`
	Connections map[string]connection `json:"connections"`
}

func loadDatabase(directory string) (string, databaseConfig, error) {
	config := databaseConfig{}
	directory, e := filepath.Abs(expandHome(directory))
	if e != nil {
		return "", config, e
	}
	if s, e := os.Stat(directory); e != nil || !s.IsDir() {
		return "", config, errors.New("project directory does not exist")
	}
	for p := directory; ; p = filepath.Dir(p) {
		path := filepath.Join(p, ".db.json")
		b, e := os.ReadFile(path)
		if e == nil {
			if e = json.Unmarshal(b, &config); e != nil {
				return "", config, e
			}
			if len(config.Connections) == 0 {
				return "", config, errors.New(".db.json requires a nonempty connections object")
			}
			for name, c := range config.Connections {
				switch c.Engine {
				case "postgres", "mysql":
					if c.URLEnv == "" {
						return "", config, fmt.Errorf("%s: missing url_env", name)
					}
				case "sqlite":
					if c.File == "" {
						return "", config, fmt.Errorf("%s: missing file", name)
					}
				default:
					return "", config, fmt.Errorf("%s: unsupported engine", name)
				}
			}
			if config.Default != "" {
				if _, ok := config.Connections[config.Default]; !ok {
					return "", config, errors.New("default connection does not exist")
				}
			} else if len(config.Connections) == 1 {
				for name := range config.Connections {
					config.Default = name
				}
			}
			return path, config, nil
		}
		if !os.IsNotExist(e) {
			return "", config, e
		}
		if _, e := os.Stat(filepath.Join(p, ".git")); e == nil {
			break
		}
		if filepath.Dir(p) == p {
			break
		}
	}
	return "", config, errors.New("no .db.json found in this project; see db --help")
}
func databaseFile(c connection, path string) string {
	if filepath.IsAbs(c.File) {
		return filepath.Clean(c.File)
	}
	return filepath.Join(filepath.Dir(path), c.File)
}
func connectionInfo(name string, c connection, path, defaultName string) map[string]any {
	r := map[string]any{"name": name, "engine": c.Engine, "config": path, "default": name == defaultName}
	if c.Engine == "sqlite" {
		file := databaseFile(c, path)
		r["file"] = file
		s, e := os.Stat(file)
		r["ready"] = e == nil && !s.IsDir()
	} else {
		r["url_env"] = c.URLEnv
		r["ready"] = os.Getenv(c.URLEnv) != ""
	}
	return r
}
func setEnv(env []string, key, value string) []string {
	result := []string{}
	for _, entry := range env {
		if !strings.HasPrefix(entry, key+"=") {
			result = append(result, entry)
		}
	}
	return append(result, key+"="+value)
}
func databaseCommand(c connection, path, query string) ([]string, []string, string, error) {
	env := os.Environ()
	stdin := ""
	if c.Engine == "sqlite" {
		file := databaseFile(c, path)
		s, e := os.Stat(file)
		if e != nil || s.IsDir() {
			return nil, nil, "", fmt.Errorf("SQLite file does not exist: %s", file)
		}
		return []string{"sqlite3", "-bail", file}, env, query, nil
	}
	raw := os.Getenv(c.URLEnv)
	if raw == "" {
		return nil, nil, "", fmt.Errorf("set %s before connecting", c.URLEnv)
	}
	u, e := url.Parse(raw)
	if e != nil {
		return nil, nil, "", errors.New("invalid database URL")
	}
	if c.Engine == "postgres" {
		if u.Scheme != "postgres" && u.Scheme != "postgresql" {
			return nil, nil, "", errors.New("expected a postgres:// or postgresql:// URL")
		}
		env = setEnv(env, "PGDATABASE", raw)
		argv := []string{"psql", "-X", "--set=ON_ERROR_STOP=1"}
		if query != "" {
			argv = append(argv, "--file", query)
		}
		return argv, env, stdin, nil
	}
	if u.Scheme != "mysql" || u.RawQuery != "" || u.Fragment != "" || u.Hostname() == "" || strings.Trim(u.Path, "/") == "" {
		return nil, nil, "", errors.New("expected mysql://user:password@host:port/database without URL options")
	}
	port := u.Port()
	if port == "" {
		port = "3306"
	}
	user, password := "", ""
	if u.User != nil {
		user = u.User.Username()
		password, _ = u.User.Password()
	}
	env = setEnv(env, "MYSQL_PWD", password)
	return []string{"mysql", "--host", u.Hostname(), "--port", port, "--user", user, "--database", strings.TrimPrefix(u.Path, "/")}, env, query, nil
}
func runDB(args []string) (int, error) {
	o, e := parse(args, []string{"--json", "--pick"}, []string{"--directory", "--file"})
	if e != nil {
		return 0, e
	}
	if o.Flags["--help"] {
		fmt.Print(catalog.Help("db"))
		return 0, nil
	}
	if len(o.Native) > 0 || len(o.Pos) > 2 {
		return 0, errors.New("unexpected arguments")
	}
	action := "list"
	if len(o.Pos) > 0 {
		action = o.Pos[0]
	}
	if action != "list" && action != "info" && action != "connect" && action != "query" {
		return 0, fmt.Errorf("unknown action %s", action)
	}
	directory := o.Values["--directory"]
	if directory == "" {
		directory = "."
	}
	path, config, e := loadDatabase(directory)
	if e != nil {
		return 0, e
	}
	names := []string{}
	for name := range config.Connections {
		names = append(names, name)
	}
	sort.Strings(names)
	if action == "list" {
		if len(o.Pos) > 1 {
			return 0, errors.New("list does not take a connection")
		}
		rows := []map[string]any{}
		for _, name := range names {
			rows = append(rows, connectionInfo(name, config.Connections[name], path, config.Default))
		}
		if o.Flags["--json"] {
			return 0, emit(rows, true)
		}
		for _, r := range rows {
			fmt.Printf("%s  engine=%s default=%v ready=%v\n", r["name"], r["engine"], r["default"], r["ready"])
		}
		return 0, nil
	}
	name := config.Default
	if len(o.Pos) > 1 {
		name = o.Pos[1]
	}
	if o.Flags["--pick"] {
		if action != "connect" {
			return 0, errors.New("--pick is for db connect")
		}
		name, e = choose(names, "connection")
		if e != nil {
			return 0, e
		}
	}
	c, ok := config.Connections[name]
	if !ok {
		return 0, errors.New("select a connection explicitly; use db list")
	}
	if action == "info" {
		r := connectionInfo(name, c, path, config.Default)
		if o.Flags["--json"] {
			return 0, emit(r, true)
		}
		for _, key := range []string{"name", "engine", "config", "default", "url_env", "file", "ready"} {
			if v, ok := r[key]; ok {
				fmt.Printf("%s: %v\n", key, v)
			}
		}
		return 0, nil
	}
	if o.Flags["--json"] {
		return 0, errors.New("--json is for inspection; native clients control query output")
	}
	query := ""
	if action == "query" {
		if o.Values["--file"] == "" {
			return 0, errors.New("query requires --file")
		}
		query, e = filepath.Abs(expandHome(o.Values["--file"]))
		if e != nil {
			return 0, e
		}
		if s, e := os.Stat(query); e != nil || s.IsDir() {
			return 0, errors.New("SQL file does not exist")
		}
	}
	argv, env, stdin, e := databaseCommand(c, path, query)
	if e != nil {
		return 0, e
	}
	return 0, execute(argv, "", env, stdin)
}
