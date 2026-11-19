package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/yetric/worktools/internal/catalog"
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
		return 0, errors.New("usage: worktools complete <tools|dev|pod|db|dots|svc|agent> [--kind KIND] [--directory DIR]")
	}
	if o.Pos[0] == "tools" {
		for _, tool := range catalog.Tools() {
			fmt.Printf("%s:%s\n", tool.Name, tool.Description)
		}
		return 0, nil
	}
	if o.Values["--kind"] == "commands" {
		tool, ok := catalog.Get(o.Pos[0])
		if !ok {
			return 0, errors.New("unknown completion tool")
		}
		for _, command := range tool.Commands {
			fmt.Printf("%s:%s\n", command.Name, command.Description)
		}
		return 0, nil
	}
	if o.Pos[0] == "pod" && o.Values["--kind"] == "inspection" {
		tool, _ := catalog.Get("pod")
		for _, command := range tool.Commands {
			if command.Name == "providers" || command.Name == "actions" || command.Name == "explain" {
				fmt.Printf("%s:%s\n", command.Name, command.Description)
			}
		}
		return 0, nil
	}
	if o.Pos[0] == "agent" {
		switch o.Values["--kind"] {
		case "tools":
			for _, tool := range catalog.Selectable() {
				fmt.Printf("%s:%s\n", tool.Name, tool.Description)
			}
		case "files":
			fmt.Println("AGENTS.md\nCLAUDE.md")
		default:
			return 0, errors.New("agent completion requires --kind commands, tools or files")
		}
		return 0, nil
	}
	directory := o.Values["--directory"]
	if directory == "" {
		directory = "."
	}
	switch o.Pos[0] {
	case "pod":
		switch o.Values["--kind"] {
		case "", "actions", "operations", "providers", "services":
		default:
			return 0, errors.New("unknown pod completion kind")
		}
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
