package dev

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

func discoverMake(root string) ([]Action, error) {
	targets := map[string]string{}
	seen := map[string]bool{}
	var scan func(string) error
	scan = func(name string) error {
		path := filepath.Join(root, name)
		if filepath.IsAbs(name) {
			path = name
		}
		if seen[path] {
			return nil
		}
		seen[path] = true
		text, e := read(filepath.Dir(path), filepath.Base(path))
		if e != nil {
			return nil
		}
		for _, line := range strings.Split(text, "\n") {
			if line == "" || strings.ContainsRune(" \t#", rune(line[0])) {
				continue
			}
			trim := strings.TrimPrefix(line, "-")
			if strings.HasPrefix(trim, "include ") || strings.HasPrefix(trim, "include\t") {
				for _, name := range strings.Fields(strings.SplitN(trim, "#", 2)[0])[1:] {
					if !strings.ContainsAny(name, "$*?%") {
						if e := scan(name); e != nil {
							return e
						}
					}
				}
				continue
			}
			i := strings.Index(line, ":")
			if i < 0 || strings.ContainsAny(line[:i], "=$") || strings.HasPrefix(line[i+1:], "=") {
				continue
			}
			description := ""
			if d := strings.Index(line, "##"); d >= 0 {
				description = strings.TrimSpace(line[d+2:])
			}
			for _, name := range strings.Fields(line[:i]) {
				if actionName.MatchString(name) && !strings.HasPrefix(name, ".") {
					d := description
					if d == "" {
						d = "Make target " + name
					}
					targets[name] = d
				}
			}
		}
		return nil
	}
	for _, name := range []string{"GNUmakefile", "makefile", "Makefile"} {
		if File(root, name) {
			if e := scan(name); e != nil {
				return nil, e
			}
			break
		}
	}
	names := []string{}
	for name := range targets {
		names = append(names, name)
	}
	sort.Strings(names)
	result := []Action{}
	for _, name := range names {
		a := action(name, "make", []string{"make", name}, "Makefile target")
		a.Description = targets[name]
		result = append(result, a)
	}
	if d, ok := targets["dev"]; ok {
		a := action("start", "make", []string{"make", "dev"}, "Makefile dev target used for startup")
		a.Description = d
		result = append(result, a)
	}
	return result, nil
}

func discoverJS(root string) ([]Action, error) {
	text, e := read(root, "package.json")
	if e != nil {
		return nil, e
	}
	var manifest struct {
		Scripts        map[string]string `json:"scripts"`
		PackageManager string            `json:"packageManager"`
	}
	if e = json.Unmarshal([]byte(text), &manifest); e != nil {
		return nil, e
	}
	runners := []string{}
	if manifest.PackageManager != "" {
		name := strings.SplitN(manifest.PackageManager, "@", 2)[0]
		if !contains([]string{"npm", "pnpm", "yarn", "bun"}, name) {
			return nil, fmt.Errorf("unsupported packageManager %q", name)
		}
		runners = append(runners, name)
	} else {
		for _, entry := range []struct {
			runner string
			files  []string
		}{{"npm", []string{"package-lock.json", "npm-shrinkwrap.json"}}, {"pnpm", []string{"pnpm-lock.yaml"}}, {"yarn", []string{"yarn.lock"}}, {"bun", []string{"bun.lock", "bun.lockb"}}} {
			for _, name := range entry.files {
				if File(root, name) {
					runners = append(runners, entry.runner)
					break
				}
			}
		}
		if len(runners) == 0 {
			runners = []string{"npm"}
		}
	}
	result := []Action{}
	names := []string{}
	for n := range manifest.Scripts {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, runner := range runners {
		for _, name := range names {
			if actionName.MatchString(name) {
				a := action(name, runner, []string{runner, "run", name}, "package.json script")
				a.Description = "Package script " + name
				if runner == "npm" {
					a.Separator = []string{"--"}
				}
				result = append(result, a)
			}
		}
		if _, ok := manifest.Scripts["dev"]; ok {
			a := action("start", runner, []string{runner, "run", "dev"}, "package.json dev script used for startup")
			if runner == "npm" {
				a.Separator = []string{"--"}
			}
			result = append(result, a)
		}
	}
	return result, nil
}

func discoverComposer(root string) ([]Action, error) {
	text, e := read(root, "composer.json")
	if e != nil {
		return nil, e
	}
	var manifest struct {
		Scripts      map[string]json.RawMessage `json:"scripts"`
		Descriptions map[string]string          `json:"scripts-descriptions"`
	}
	if e = json.Unmarshal([]byte(text), &manifest); e != nil {
		return nil, e
	}
	result := []Action{}
	names := []string{}
	for n := range manifest.Scripts {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		if actionName.MatchString(name) {
			a := action(name, "composer", []string{"composer", "run", name}, "composer.json script")
			a.Separator = []string{"--"}
			if d := manifest.Descriptions[name]; d != "" {
				a.Description = d
			}
			result = append(result, a)
		}
	}
	if _, ok := manifest.Scripts["dev"]; ok {
		a := action("start", "composer", []string{"composer", "run", "dev"}, "composer.json dev script used for startup")
		a.Separator = []string{"--"}
		result = append(result, a)
	}
	return result, nil
}

var unittestImport = regexp.MustCompile(`(?m)^\s*(?:import\s+unittest\b|from\s+unittest(?:\.[\w.]+)?\s+import\b)`)
