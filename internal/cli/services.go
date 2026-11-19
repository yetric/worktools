package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/yetric/worktools/internal/catalog"
)

func configHome() string {
	if value := os.Getenv("XDG_CONFIG_HOME"); value != "" {
		return value
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config")
}
func serviceAliases() (map[string]string, error) {
	path := filepath.Join(configHome(), "worktools/services.json")
	b, e := os.ReadFile(path)
	if os.IsNotExist(e) {
		return map[string]string{}, nil
	}
	if e != nil {
		return nil, e
	}
	var value struct {
		Aliases map[string]string `json:"aliases"`
	}
	if e = json.Unmarshal(b, &value); e != nil {
		return nil, fmt.Errorf("%s: %w", path, e)
	}
	return value.Aliases, nil
}

var unitPattern = regexp.MustCompile(`^[a-zA-Z0-9_@.:\\-]+$`)

func unitName(name string) (string, error) {
	aliases, e := serviceAliases()
	if e != nil {
		return "", e
	}
	if v, ok := aliases[name]; ok {
		name = v
	}
	if !unitPattern.MatchString(name) || strings.HasPrefix(name, "-") {
		return "", errors.New("use a single service or timer name, without options or patterns")
	}
	if !strings.HasSuffix(name, ".service") && !strings.HasSuffix(name, ".timer") {
		name += ".service"
	}
	return name, nil
}
func runSvc(args []string) (int, error) {
	o, e := parse(args, []string{"--json", "--system", "--aliases", "--follow"}, []string{"--lines", "--since"})
	if e != nil {
		return 0, e
	}
	if o.Flags["--help"] {
		fmt.Print(catalog.Help("svc"))
		return 0, nil
	}
	if len(o.Native) > 0 {
		return 0, errors.New("unexpected arguments after --")
	}
	action := "list"
	if len(o.Pos) > 0 {
		action = o.Pos[0]
	}
	scope := "--user"
	if o.Flags["--system"] {
		scope = "--system"
	}
	if action == "list" && o.Flags["--aliases"] {
		aliases, e := serviceAliases()
		if e != nil {
			return 0, e
		}
		rows := []map[string]string{}
		for name, unit := range aliases {
			rows = append(rows, map[string]string{"name": name, "unit": unit})
		}
		if o.Flags["--json"] {
			return 0, emit(rows, true)
		}
		for _, r := range rows {
			fmt.Printf("%s  %s\n", r["name"], r["unit"])
		}
		return 0, nil
	}
	if action == "list" || action == "timers" || action == "failed" {
		if len(o.Pos) > 1 {
			return 0, errors.New("this action does not take a unit")
		}
		commands := map[string][]string{"list": {"list-units", "--all", "--type=service"}, "timers": {"list-timers", "--all"}, "failed": {"list-units", "--failed"}}
		argv := append([]string{"systemctl", scope, "--no-pager"}, commands[action]...)
		if o.Flags["--json"] {
			b, e := capture(append(argv, "--output=json")...)
			if e != nil {
				return 0, e
			}
			var value any
			if e = json.Unmarshal(b, &value); e != nil {
				return 0, e
			}
			return 0, emit(value, true)
		}
		return 0, execute(argv, "", nil, "")
	}
	if action != "status" && action != "logs" && action != "start" && action != "stop" && action != "restart" {
		return 0, fmt.Errorf("unknown action %s", action)
	}
	if len(o.Pos) != 2 {
		return 0, errors.New("this action requires one unit name")
	}
	unit, e := unitName(o.Pos[1])
	if e != nil {
		return 0, e
	}
	if action == "status" && o.Flags["--json"] {
		b, e := capture("systemctl", scope, "show", unit, "--property=Id,LoadState,ActiveState,SubState,Description,Result")
		if e != nil {
			return 0, e
		}
		record := map[string]string{}
		for _, line := range strings.Split(string(b), "\n") {
			k, v, ok := strings.Cut(line, "=")
			if ok {
				record[k] = v
			}
		}
		code := 0
		if record["LoadState"] == "not-found" {
			code = 1
		}
		return code, emit(record, true)
	}
	if action == "logs" {
		lines := "50"
		if v, ok := o.Values["--lines"]; ok {
			if _, e := strconv.Atoi(v); e != nil {
				return 0, errors.New("--lines must be an integer")
			}
			lines = v
		}
		argv := []string{"journalctl", scope, "--no-pager", "--unit", unit, "--lines", lines}
		if o.Flags["--follow"] {
			argv = append(argv, "--follow")
		}
		if v, ok := o.Values["--since"]; ok {
			argv = append(argv, "--since", v)
		}
		if o.Flags["--json"] {
			argv = append(argv, "--output=json")
		}
		return 0, execute(argv, "", nil, "")
	}
	if o.Flags["--json"] && action != "status" {
		return 0, errors.New("--json is only for inspection")
	}
	argv := []string{"systemctl", scope}
	if action == "status" {
		argv = append(argv, "--no-pager")
	}
	return 0, execute(append(argv, action, unit), "", nil, "")
}
