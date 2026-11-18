package cli

import (
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

var dotPaths = map[string]string{"shell": ".zshrc", "git": ".gitconfig", "profile": ".profile", "ghostty": ".config/ghostty/config", "alacritty": ".config/alacritty/alacritty.toml", "zed": ".config/zed/settings.json", "starship": ".config/starship.toml", "fonts": ".config/fontconfig/fonts.conf", "mise": ".config/mise/config.toml"}

type dotRecord struct {
	Name           string   `json:"name"`
	Config         string   `json:"config"`
	Source         string   `json:"source"`
	Status         string   `json:"status"`
	ExpectedSource string   `json:"expected_source"`
	Managed        bool     `json:"managed"`
	Issues         []string `json:"issues,omitempty"`
	OK             *bool    `json:"ok,omitempty"`
}

func dotRoot() string {
	if value := os.Getenv("DOTFILES"); value != "" {
		path, _ := filepath.Abs(expandHome(value))
		return path
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "dotfiles")
}
func dotInfo(name, home, root string) (dotRecord, error) {
	rel, ok := dotPaths[name]
	if !ok {
		return dotRecord{}, fmt.Errorf("unknown configuration %s", name)
	}
	path := filepath.Join(home, rel)
	expected := filepath.Join(root, rel)
	r := dotRecord{Name: name, Config: path, Source: path, Status: "missing", ExpectedSource: expected}
	info, e := os.Lstat(path)
	if os.IsNotExist(e) {
		return r, nil
	}
	if e != nil {
		return r, e
	}
	if info.Mode()&os.ModeSymlink != 0 {
		source, e := filepath.EvalSymlinks(path)
		if e != nil {
			r.Status = "broken"
			target, _ := os.Readlink(path)
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(path), target)
			}
			r.Source = filepath.Clean(target)
			return r, nil
		}
		r.Status = "linked"
		r.Source = source
		expectedSource, e := filepath.EvalSymlinks(expected)
		if e != nil {
			expectedSource = expected
		}
		r.Managed = source == expectedSource
	} else {
		r.Status = "local"
	}
	return r, nil
}
func checkDot(r dotRecord) dotRecord {
	issues := []string{}
	if r.Status == "missing" || r.Status == "broken" {
		issues = append(issues, r.Status)
	} else {
		if !r.Managed {
			issues = append(issues, "configuration is not linked to the selected dotfiles checkout")
		}
		if strings.HasSuffix(r.Source, ".toml") {
			var value map[string]any
			if _, e := toml.DecodeFile(r.Source, &value); e != nil {
				issues = append(issues, e.Error())
			}
		}
		if r.Name == "fonts" {
			b, e := os.ReadFile(r.Source)
			if e != nil {
				issues = append(issues, e.Error())
			} else {
				var value any
				if e = xml.Unmarshal(b, &value); e != nil {
					issues = append(issues, e.Error())
				}
			}
		}
		if r.Name == "shell" {
			if _, e := exec.LookPath("zsh"); e != nil {
				issues = append(issues, "zsh unavailable; syntax not checked")
			} else {
				b, e := exec.Command("zsh", "-n", r.Source).CombinedOutput()
				if e != nil {
					issues = append(issues, strings.TrimSpace(string(b)))
				}
			}
		}
	}
	r.Issues = issues
	ok := len(issues) == 0
	r.OK = &ok
	return r
}

// SplitEditor supports the common VISUAL='editor --flag' form without invoking a shell.
func splitEditor(value string) ([]string, error) {
	result := []string{}
	var current strings.Builder
	var quote rune
	escaped := false
	started := false
	for _, c := range value {
		if escaped {
			current.WriteRune(c)
			escaped = false
			started = true
			continue
		}
		if c == '\\' && quote != '\'' {
			escaped = true
			started = true
			continue
		}
		if quote != 0 {
			if c == quote {
				quote = 0
			} else {
				current.WriteRune(c)
			}
			started = true
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			started = true
			continue
		}
		if c == ' ' || c == '\t' {
			if started {
				result = append(result, current.String())
				current.Reset()
				started = false
			}
			continue
		}
		current.WriteRune(c)
		started = true
	}
	if quote != 0 || escaped {
		return nil, errors.New("unterminated quote or escape in editor setting")
	}
	if started {
		result = append(result, current.String())
	}
	if len(result) == 0 || result[0] == "" {
		return nil, errors.New("empty editor command")
	}
	return result, nil
}
func runDots(args []string) (int, error) {
	o, e := parse(args, []string{"--json", "--pick"}, nil)
	if e != nil {
		return 0, e
	}
	if o.Flags["--help"] {
		fmt.Println("Usage: dots [list|show NAME|edit NAME|check [NAME]|diff [NAME]] [--json]\n\nlist/show: inspect configuration paths and symlink sources.\nedit: open the actual source using VISUAL, EDITOR, or vi; --pick selects a name.\ncheck: check links, TOML, fontconfig XML, and Zsh syntax; exits 1 on issues.\ndiff: show tracked Git working-tree changes.\nDOTFILES selects the checkout (default ~/dotfiles). No automatic relinking.")
		return 0, nil
	}
	if len(o.Native) > 0 || len(o.Pos) > 2 {
		return 0, errors.New("unexpected arguments")
	}
	action := "list"
	if len(o.Pos) > 0 {
		action = o.Pos[0]
	}
	name := ""
	if len(o.Pos) > 1 {
		name = o.Pos[1]
	}
	names := []string{}
	for n := range dotPaths {
		names = append(names, n)
	}
	sort.Strings(names)
	if o.Flags["--pick"] {
		if action != "edit" {
			return 0, errors.New("--pick is for dots edit")
		}
		name, e = choose(names, "configuration")
		if e != nil {
			return 0, e
		}
	}
	if action == "edit" || action == "show" {
		if name == "" {
			return 0, errors.New("this action requires a name; use dots edit --pick to select")
		}
	}
	if name != "" {
		if _, ok := dotPaths[name]; !ok {
			return 0, fmt.Errorf("unknown configuration %s", name)
		}
		names = []string{name}
	}
	home, e := os.UserHomeDir()
	if e != nil {
		return 0, e
	}
	root := dotRoot()
	if action == "diff" {
		argv := []string{"git", "-C", root, "diff"}
		if name != "" {
			argv = append(argv, "--", dotPaths[name])
		}
		return 0, execute(argv, "", nil, "")
	}
	if action != "list" && action != "show" && action != "check" && action != "edit" {
		return 0, fmt.Errorf("unknown action %s", action)
	}
	rows := []dotRecord{}
	code := 0
	for _, name := range names {
		r, e := dotInfo(name, home, root)
		if e != nil {
			return 0, e
		}
		if action == "check" {
			r = checkDot(r)
			if !*r.OK {
				code = 1
			}
		}
		rows = append(rows, r)
	}
	if action == "edit" {
		r := rows[0]
		if r.Status == "missing" || r.Status == "broken" {
			return 0, fmt.Errorf("%s: %s", r.Name, r.Status)
		}
		editor := os.Getenv("VISUAL")
		if editor == "" {
			editor = os.Getenv("EDITOR")
		}
		if editor == "" {
			editor = "vi"
		}
		argv, e := splitEditor(editor)
		if e != nil {
			return 0, e
		}
		return 0, execute(append(argv, r.Source), "", nil, "")
	}
	if o.Flags["--json"] {
		if action == "show" {
			return code, emit(rows[0], true)
		}
		return code, emit(rows, true)
	}
	for _, r := range rows {
		fmt.Printf("%s  %s\n  config: %s\n  source: %s\n  expected: %s\n  managed: %v\n", r.Name, r.Status, r.Config, r.Source, r.ExpectedSource, r.Managed)
		for _, issue := range r.Issues {
			fmt.Printf("  issue: %s\n", issue)
		}
	}
	return code, nil
}
