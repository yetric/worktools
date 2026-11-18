// Package dev resolves development actions. Adapters inspect files; only the CLI executes commands.
package dev

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/yetric/worktools/internal/dev/provider"
)

type Action = provider.Action
type Adapter = provider.Provider

type Record struct {
	Name                string   `json:"name"`
	Status              string   `json:"status"`
	Description         string   `json:"description"`
	Command             []string `json:"command"`
	Directory           string   `json:"directory"`
	Reason              string   `json:"reason"`
	Separator           []string `json:"argument_separator,omitempty"`
	ExecutableAvailable bool     `json:"executable_available"`
	Candidates          []Action `json:"candidates"`
}

func Root(directory string, adapters []Adapter) (string, error) {
	directory, e := filepath.Abs(directory)
	if e != nil {
		return "", e
	}
	if s, e := os.Stat(directory); e != nil || !s.IsDir() {
		return "", fmt.Errorf("not a directory: %s", directory)
	}
	nearest := ""
	for p := directory; ; p = filepath.Dir(p) {
		if provider.File(p, ".worktools.toml") {
			return p, nil
		}
		if nearest == "" {
			for _, a := range adapters {
				for _, m := range a.Manifests() {
					if provider.File(p, m) {
						nearest = p
						break
					}
				}
			}
		}
		if _, e := os.Stat(filepath.Join(p, ".git")); e == nil {
			break
		}
		if filepath.Dir(p) == p {
			break
		}
	}
	if nearest != "" {
		return nearest, nil
	}
	return "", fmt.Errorf("no supported project manifest or .worktools.toml found")
}

type mapping struct {
	Command     []string `toml:"command"`
	Description string   `toml:"description"`
	Directory   string   `toml:"directory"`
	Separator   []string `toml:"argument_separator"`
}
type projectConfig struct {
	Dev struct {
		Runner  string             `toml:"runner"`
		Actions map[string]mapping `toml:"actions"`
	} `toml:"dev"`
}

func Discover(root string, adapters []Adapter) ([]Record, error) {
	config := projectConfig{}
	if provider.File(root, ".worktools.toml") {
		if _, e := toml.DecodeFile(filepath.Join(root, ".worktools.toml"), &config); e != nil {
			return nil, fmt.Errorf(".worktools.toml: %w", e)
		}
	}
	runners := []string{}
	for _, a := range adapters {
		runners = append(runners, a.Runners()...)
	}
	if config.Dev.Runner != "" && !provider.Contains(runners, config.Dev.Runner) {
		return nil, fmt.Errorf("unknown preferred runner %q", config.Dev.Runner)
	}
	actions := map[string][]Action{}
	for _, a := range adapters {
		present := false
		for _, m := range a.Manifests() {
			present = present || provider.File(root, m)
		}
		if !present {
			continue
		}
		found, e := a.Discover(root)
		if e != nil {
			return nil, fmt.Errorf("adapter %s: %w", a.Name(), e)
		}
		for _, entry := range found {
			if !provider.ValidName(entry.Name) || !provider.Contains(a.Runners(), entry.Runner) || len(entry.Command) == 0 || entry.Command[0] == "" {
				return nil, fmt.Errorf("adapter %s returned an invalid action", a.Name())
			}
			for _, arg := range append(append([]string{}, entry.Command...), entry.Separator...) {
				if strings.ContainsRune(arg, 0) {
					return nil, fmt.Errorf("adapter %s returned a NUL argument", a.Name())
				}
			}
			if entry.Name == "list" || entry.Name == "explain" {
				continue
			}
			entry.Directory = root
			if entry.Name == "start" { // Adapter may emit a dev alias after a native start action.
				entries := actions[entry.Name]
				for i := len(entries) - 1; i >= 0; i-- {
					if entries[i].Runner == entry.Runner {
						entries = append(entries[:i], entries[i+1:]...)
					}
				}
				actions[entry.Name] = entries
			}
			actions[entry.Name] = append(actions[entry.Name], entry)
		}
	}
	for name, m := range config.Dev.Actions {
		if !provider.ValidName(name) || name == "list" || name == "explain" || len(m.Command) == 0 || m.Command[0] == "" {
			return nil, fmt.Errorf("invalid dev.actions.%s mapping", name)
		}
		for _, arg := range append(append([]string{}, m.Command...), m.Separator...) {
			if strings.ContainsRune(arg, 0) {
				return nil, fmt.Errorf("dev.actions.%s contains a NUL argument", name)
			}
		}
		dir := root
		if m.Directory != "" {
			if filepath.IsAbs(m.Directory) {
				dir = m.Directory
			} else {
				dir = filepath.Join(root, m.Directory)
			}
		}
		if s, e := os.Stat(dir); e != nil || !s.IsDir() {
			return nil, fmt.Errorf("dev.actions.%s: directory does not exist", name)
		}
		if m.Description == "" {
			m.Description = "Configured action " + name
		}
		if m.Separator == nil && len(m.Command) > 1 && provider.Contains([]string{"npm", "composer"}, m.Command[0]) && m.Command[1] == "run" {
			m.Separator = []string{"--"}
		}
		actions[name] = []Action{{Name: name, Runner: "configured", Command: m.Command, Description: m.Description, Reason: "Explicit dev.actions." + name + " mapping", Directory: dir, Separator: m.Separator}}
	}
	names := []string{}
	for name := range actions {
		names = append(names, name)
	}
	sort.Strings(names)
	records := []Record{}
	for _, name := range names {
		candidates := actions[name]
		selected := []Action{}
		_, mapped := config.Dev.Actions[name]
		for _, c := range candidates {
			if mapped || config.Dev.Runner == "" || c.Runner == config.Dev.Runner {
				selected = append(selected, c)
			}
		}
		r := Record{Name: name, Status: "ambiguous", Description: "Choose a runner for " + name, Directory: root, Reason: "Set dev.runner or dev.actions." + name + " in .worktools.toml", Candidates: candidates}
		if len(selected) == 0 {
			r.Status = "unavailable"
		}
		if len(selected) == 1 {
			c := selected[0]
			r.Status = "ready"
			r.Description = c.Description
			r.Command = c.Command
			r.Separator = c.Separator
			r.Directory = c.Directory
			r.Reason = c.Reason
			if strings.Contains(c.Command[0], "/") {
				path := c.Command[0]
				if !filepath.IsAbs(path) {
					path = filepath.Join(c.Directory, path)
				}
				s, e := os.Stat(path)
				r.ExecutableAvailable = e == nil && !s.IsDir() && s.Mode()&0111 != 0
			} else {
				_, e := exec.LookPath(c.Command[0])
				r.ExecutableAvailable = e == nil
			}
		}
		records = append(records, r)
	}
	return records, nil
}
