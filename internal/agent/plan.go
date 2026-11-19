package agent

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/BurntSushi/toml"
)

type File struct {
	Path              string   `json:"path"`
	Target            string   `json:"target"`
	Aliases           []string `json:"aliases"`
	Changed           bool     `json:"changed"`
	Created           bool     `json:"created"`
	Migrated          bool     `json:"migrated"`
	Removal           bool     `json:"remove_reference"`
	Diff              string   `json:"diff"`
	original, updated []byte
	mode              os.FileMode
}
type Plan struct {
	Directory string    `json:"directory"`
	Selection Selection `json:"selection"`
	Files     []*File   `json:"files"`
	Changed   bool      `json:"changed"`
}

func projectDirectory(directory string) (string, error) {
	path, e := filepath.Abs(directory)
	if e != nil {
		return "", e
	}
	path, e = filepath.EvalSymlinks(path)
	if e != nil {
		return "", e
	}
	s, e := os.Stat(path)
	if e != nil || !s.IsDir() {
		return "", fmt.Errorf("not a project directory: %s", directory)
	}
	return path, nil
}

func inside(root, path string) bool {
	rel, e := filepath.Rel(root, path)
	return e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// Resolve missing paths through their nearest existing parent as well, so a
// symlinked .worktools directory cannot redirect writes outside the project.
func resolve(path string) (string, error) {
	if _, e := os.Lstat(path); e == nil {
		return filepath.EvalSymlinks(path)
	} else if !os.IsNotExist(e) {
		return "", e
	}
	parent, e := filepath.EvalSymlinks(filepath.Dir(path))
	if os.IsNotExist(e) {
		parent, e = resolve(filepath.Dir(path))
	}
	if e != nil {
		return "", e
	}
	return filepath.Join(parent, filepath.Base(path)), nil
}

func readFile(root, name string) (*File, error) {
	path := filepath.Join(root, name)
	target, e := resolve(path)
	if e != nil {
		return nil, fmt.Errorf("%s: cannot resolve file (including dangling symlinks): %w", name, e)
	}
	if !inside(root, target) {
		return nil, fmt.Errorf("%s points outside the project; update its source explicitly", name)
	}
	file := &File{Path: name, Target: target, Aliases: []string{name}, Created: true, mode: 0644}
	info, e := os.Stat(target)
	if os.IsNotExist(e) {
		return file, nil
	}
	if e != nil {
		return nil, e
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular instruction/configuration file", name)
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok && stat.Nlink > 1 {
		return nil, fmt.Errorf("%s has hard links; use a shared symlink target to preserve link relationships", name)
	}
	file.original, e = os.ReadFile(target)
	if e != nil {
		return nil, e
	}
	file.Created = false
	file.mode = info.Mode().Perm()
	return file, nil
}

func LoadSelection(directory string) (Selection, bool, error) {
	root, e := projectDirectory(directory)
	if e != nil {
		return Selection{}, false, e
	}
	file, e := readFile(root, SelectionFile)
	if e != nil {
		return Selection{}, false, e
	}
	if file.Created {
		return Defaults(), false, nil
	}
	var selection Selection
	metadata, e := toml.Decode(string(file.original), &selection)
	if e != nil {
		return selection, true, fmt.Errorf("%s: %w", SelectionFile, e)
	}
	if len(metadata.Undecoded()) > 0 {
		return selection, true, fmt.Errorf("%s: unknown selection settings", SelectionFile)
	}
	selection, e = Normalize(selection)
	return selection, true, e
}

func selectionText(selection Selection) string {
	array := func(values []string) string {
		quoted := []string{}
		for _, value := range values {
			quoted = append(quoted, strconv.Quote(value))
		}
		return "[" + strings.Join(quoted, ", ") + "]"
	}
	return "# Managed by worktools agent; init changes these selections.\nversion = 1\ntools = " + array(selection.Tools) + "\nfiles = " + array(selection.Files) + "\n"
}

func Prepare(directory string, tools, files []string) (*Plan, error) {
	root, e := projectDirectory(directory)
	if e != nil {
		return nil, e
	}
	previous, saved, e := LoadSelection(root)
	if e != nil {
		return nil, e
	}
	selection := previous
	if tools != nil {
		selection.Tools = tools
	}
	if files != nil {
		selection.Files = files
	}
	selection, e = Normalize(selection)
	if e != nil {
		return nil, e
	}
	block, e := Render(root, selection)
	if e != nil {
		return nil, e
	}
	plan := &Plan{Directory: root, Selection: selection, Files: []*File{}}
	seen := map[string]*File{}
	for _, name := range selection.Files {
		file, e := readFile(root, name)
		if e != nil {
			return nil, e
		}
		if shared := seen[file.Target]; shared != nil {
			shared.Aliases = append(shared.Aliases, name)
			continue
		}
		updated, legacy, e := Replace(string(file.original), block, false)
		if e != nil {
			return nil, fmt.Errorf("%s: %w", name, e)
		}
		file.updated = []byte(updated)
		file.Migrated = legacy
		plan.Files = append(plan.Files, file)
		seen[file.Target] = file
	}
	if saved {
		for _, name := range previous.Files {
			retained := false
			for _, chosen := range selection.Files {
				retained = retained || chosen == name
			}
			if retained {
				continue
			}
			file, e := readFile(root, name)
			if e != nil {
				return nil, e
			}
			if seen[file.Target] != nil || file.Created {
				continue
			}
			updated, _, e := Replace(string(file.original), "", true)
			if e != nil {
				return nil, fmt.Errorf("%s: %w", name, e)
			}
			file.updated = []byte(updated)
			file.Removal = true
			plan.Files = append(plan.Files, file)
			seen[file.Target] = file
		}
	}
	config, e := readFile(root, SelectionFile)
	if e != nil {
		return nil, e
	}
	if seen[config.Target] != nil {
		return nil, errors.New("selection configuration and instruction files point to the same file")
	}
	config.updated = []byte(selectionText(selection))
	plan.Files = append(plan.Files, config)
	for _, file := range plan.Files {
		file.Changed = !bytes.Equal(file.original, file.updated)
		file.Diff = unifiedDiff(file.Path, string(file.original), string(file.updated), file.Created)
		plan.Changed = plan.Changed || file.Changed
	}
	return plan, nil
}

func (plan *Plan) unchanged() error {
	for _, file := range plan.Files {
		for _, alias := range file.Aliases {
			now, e := readFile(plan.Directory, alias)
			if e != nil {
				return e
			}
			if now.Target != file.Target || now.Created != file.Created || !bytes.Equal(now.original, file.original) || now.mode != file.mode {
				return fmt.Errorf("%s changed since preview; preview again before syncing", alias)
			}
		}
	}
	return nil
}

// Apply preflights all files, stages replacements beside their targets and rolls
// back ordinary commit failures. Symlinks retain their existing relationships.
func (plan *Plan) Apply() error {
	if e := plan.unchanged(); e != nil {
		return e
	}
	type staged struct {
		file            *File
		updated, backup string
	}
	stages := []staged{}
	createdDir := ""
	defer func() {
		for _, s := range stages {
			if s.updated != "" {
				os.Remove(s.updated)
			}
			if s.backup != "" {
				os.Remove(s.backup)
			}
		}
		if createdDir != "" {
			os.Remove(createdDir)
		}
	}()
	stage := func(target string, data []byte, mode os.FileMode) (string, error) {
		file, e := os.CreateTemp(filepath.Dir(target), ".worktools-agent-*")
		if e != nil {
			return "", e
		}
		name := file.Name()
		if _, e = file.Write(data); e == nil {
			e = file.Chmod(mode)
		}
		if e == nil {
			e = file.Sync()
		}
		closeError := file.Close()
		if e == nil {
			e = closeError
		}
		if e != nil {
			os.Remove(name)
			return "", e
		}
		return name, nil
	}
	for _, file := range plan.Files {
		if !file.Changed {
			continue
		}
		parent := filepath.Dir(file.Target)
		if _, e := os.Stat(parent); os.IsNotExist(e) {
			if e = os.Mkdir(parent, 0755); e != nil {
				return e
			}
			createdDir = parent
		}
		updated, e := stage(file.Target, file.updated, file.mode)
		if e != nil {
			return e
		}
		stages = append(stages, staged{file: file, updated: updated})
		if !file.Created {
			backup, e := stage(file.Target, file.original, file.mode)
			if e != nil {
				return e
			}
			stages[len(stages)-1].backup = backup
		}
	}
	if e := plan.unchanged(); e != nil {
		return e
	}
	for i, s := range stages {
		if e := os.Rename(s.updated, s.file.Target); e != nil {
			failures := []string{}
			for j := i - 1; j >= 0; j-- {
				previous := stages[j]
				var rollback error
				if previous.file.Created {
					rollback = os.Remove(previous.file.Target)
				} else {
					rollback = os.Rename(previous.backup, previous.file.Target)
				}
				if rollback != nil {
					failures = append(failures, fmt.Sprintf("%s: %v (original backup: %s)", previous.file.Path, rollback, previous.backup))
					stages[j].backup = "" // Preserve recovery data if rollback itself failed.
				}
			}
			if len(failures) > 0 {
				return fmt.Errorf("write failed: %v; rollback errors: %s", e, strings.Join(failures, "; "))
			}
			return e
		}
	}
	return nil
}
