package cli

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/yetric/worktools/internal/dev"
)

// Completion is read-only and uses the same discovery routines as execution.
func runComplete(args []string) (int, error) {
	o, e := parse(args, []string{"--system"}, []string{"--directory"})
	if e != nil {
		return 0, e
	}
	if len(o.Pos) != 1 {
		return 0, errors.New("usage: worktools complete <dev|db|dots|svc> [--directory DIR] [--system]")
	}
	directory := o.Values["--directory"]
	if directory == "" {
		directory = "."
	}
	switch o.Pos[0] {
	case "dev":
		root, e := dev.Root(expandHome(directory), dev.Builtins())
		if e != nil {
			return 0, e
		}
		records, e := dev.Discover(root, dev.Builtins())
		if e != nil {
			return 0, e
		}
		for _, r := range records {
			name := strings.ReplaceAll(r.Name, ":", `\:`)
			description := strings.NewReplacer(":", " ", "\n", " ", "\t", " ").Replace(r.Description)
			fmt.Printf("%s:%s [%s]\n", name, description, r.Status)
		}
	case "db":
		_, config, e := loadDatabase(directory)
		if e != nil {
			return 0, e
		}
		names := []string{}
		for name := range config.Connections {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Println(strings.ReplaceAll(name, ":", `\:`))
		}
	case "dots":
		names := []string{}
		for name := range dotPaths {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Println(name)
		}
	case "svc":
		scope := "--user"
		if o.Flags["--system"] {
			scope = "--system"
		}
		output, e := capture("systemctl", scope, "list-unit-files", "--type=service,timer", "--no-legend", "--no-pager")
		if e == nil {
			for _, line := range strings.Split(string(output), "\n") {
				fields := strings.Fields(line)
				if len(fields) > 0 {
					fmt.Println(fields[0])
				}
			}
		}
		aliases, e := serviceAliases()
		if e != nil {
			return 0, e
		}
		for name := range aliases {
			fmt.Println(name)
		}
	default:
		return 0, fmt.Errorf("unknown tool %s", o.Pos[0])
	}
	return 0, nil
}
