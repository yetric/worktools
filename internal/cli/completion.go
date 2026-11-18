package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/yetric/worktools/internal/dev"
	"github.com/yetric/worktools/internal/pod"
	"github.com/yetric/worktools/internal/pod/provider"
)

// Completion is read-only and uses the same discovery routines as execution.
func runComplete(args []string) (int, error) {
	o, e := parse(args, []string{"--system"}, []string{"--directory", "--provider", "--file", "--kind"})
	if e != nil {
		return 0, e
	}
	if len(o.Pos) != 1 {
		return 0, errors.New("usage: worktools complete <dev|pod|db|dots|svc> [--directory DIR] [--system]")
	}
	directory := o.Values["--directory"]
	if directory == "" {
		directory = "."
	}
	switch o.Pos[0] {
	case "pod":
		directory, config, e := podSettings(o)
		if e != nil {
			return 0, e
		}
		registry := pod.Builtins()
		if o.Values["--kind"] == "providers" {
			for _, p := range registry {
				fmt.Println(p.Name())
			}
			return 0, nil
		}
		preferred, _ := pod.Preference(o.Values["--provider"], config, os.Getenv("WORKTOOLS_POD_PROVIDER"))
		probe := pod.NativeProbe{Directory: directory}
		ctx := context.Background()
		if o.Values["--kind"] == "services" {
			p, info, e := pod.Select(ctx, registry, probe, preferred)
			if e != nil {
				return 0, e
			}
			names, e := pod.Services(ctx, p, info, probe, provider.Request{Directory: directory, Files: config.Files})
			if e != nil {
				return 0, e
			}
			for _, name := range names {
				fmt.Println(name)
			}
			return 0, nil
		}
		seen := map[string]bool{}
		for _, info := range pod.Inspect(ctx, registry, probe) {
			if preferred != "" && preferred != info.Name {
				continue
			}
			for _, op := range info.Operations {
				if !seen[op.Name] {
					fmt.Printf("%s:%s\n", op.Name, op.Description)
					seen[op.Name] = true
				}
			}
		}
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
