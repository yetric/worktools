package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/yetric/worktools/internal/agent"
	"github.com/yetric/worktools/internal/catalog"
)

func commaList(value string) []string {
	if value == "" {
		return nil
	}
	values := strings.Split(value, ",")
	for i := range values {
		values[i] = strings.TrimSpace(values[i])
	}
	return values
}

func runAgent(args []string) (int, error) {
	o, e := parse(args, []string{"--json", "--check"}, []string{"--directory", "--tools", "--files"})
	if e != nil {
		return 0, fmt.Errorf("%s; use worktools agent --help", strings.TrimSuffix(e.Error(), "; pass runner arguments after --"))
	}
	if o.Flags["--help"] {
		fmt.Print(catalog.Help("agent"))
		return 0, nil
	}
	if len(o.Native) > 0 {
		return 0, errors.New("agent does not execute native commands or accept arguments after --")
	}
	action := "preview"
	if terminal(os.Stdin) && terminal(os.Stdout) {
		action = "init"
	}
	if len(o.Pos) > 0 {
		action = o.Pos[0]
	}
	if action != "init" && action != "preview" && action != "sync" {
		return 0, fmt.Errorf("unknown agent action %s; use init, preview, or sync", action)
	}
	if len(o.Pos) > 2 {
		return 0, errors.New("usage: worktools agent ACTION [DIRECTORY] [OPTIONS]")
	}
	directory := o.Values["--directory"]
	if len(o.Pos) == 2 {
		if strings.TrimSpace(o.Pos[1]) == "" {
			return 0, errors.New("project directory cannot be empty")
		}
		if directory != "" {
			return 0, errors.New("choose either DIRECTORY or --directory")
		}
		directory = o.Pos[1]
	}
	if directory == "" {
		directory = "."
	}
	directory = expandHome(directory)
	tools, files := commaList(o.Values["--tools"]), commaList(o.Values["--files"])
	for _, key := range []string{"--directory", "--tools", "--files"} {
		if value, exists := o.Values[key]; exists && strings.TrimSpace(value) == "" {
			return 0, fmt.Errorf("%s cannot be empty", key)
		}
	}
	if o.Flags["--check"] && action != "preview" {
		return 0, errors.New("--check applies only to preview")
	}
	if action == "init" {
		if o.Flags["--json"] {
			return 0, errors.New("init is interactive; use preview --json or sync --json for automation")
		}
		if !terminal(os.Stdin) || !terminal(os.Stdout) {
			return 0, errors.New("init requires a terminal; use preview or sync with explicit --tools and --files")
		}
		return 0, agentInit(directory, tools, files)
	}
	plan, e := agent.Prepare(directory, tools, files)
	if e != nil {
		return 0, e
	}
	applied := false
	if action == "sync" {
		if e = plan.Apply(); e != nil {
			return 0, e
		}
		applied = true
	}
	if o.Flags["--json"] {
		e = emit(struct {
			Action  string `json:"action"`
			Applied bool   `json:"applied"`
			*agent.Plan
		}{action, applied, plan}, true)
	} else if action == "preview" {
		showAgentPlan(plan)
	} else {
		for _, file := range plan.Files {
			status := "Already current"
			if file.Changed {
				status = "Updated"
			}
			fmt.Printf("%s: %s\n", status, strings.Join(file.Aliases, ", "))
		}
	}
	if e != nil {
		return 0, e
	}
	if o.Flags["--check"] && plan.Changed {
		return 1, nil
	}
	return 0, nil
}

func showAgentPlan(plan *agent.Plan) {
	fmt.Printf("Project: %s\nTools: %s\nFiles: %s\n\n", plan.Directory, strings.Join(plan.Selection.Tools, ", "), strings.Join(plan.Selection.Files, ", "))
	for _, file := range plan.Files {
		if !file.Changed {
			continue
		}
		if len(file.Aliases) > 1 || file.Target != filepath.Join(plan.Directory, file.Path) {
			fmt.Printf("Shared target: %s → %s\n", strings.Join(file.Aliases, ", "), file.Target)
		}
		if file.Migrated {
			fmt.Printf("Migrate legacy reference: %s\n", file.Path)
		}
		if file.Removal {
			fmt.Printf("Remove deselected managed reference: %s\n", file.Path)
		}
		fmt.Print(file.Diff)
	}
	if !plan.Changed {
		fmt.Println("Already current.")
	}
}

func agentInit(directory string, tools, files []string) error {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("Worktools agent onboarding\n[ ] Choose a project\n[ ] Select tools and instruction files\n[ ] Review the diff\n[ ] Write changes")
	fmt.Printf("\nProject directory [%s]: ", singleLine(directory))
	line, e := reader.ReadString('\n')
	if e != nil {
		return e
	}
	if answer := strings.TrimSpace(line); answer != "" {
		directory = expandHome(answer)
	}
	selection, _, e := agent.LoadSelection(directory)
	if e != nil {
		return e
	}
	if tools != nil {
		selection.Tools = tools
	}
	if files != nil {
		selection.Files = files
	}
	selection, e = agent.Normalize(selection)
	if e != nil {
		return e
	}
	fmt.Println("[x] Project chosen")
	choices := []pickerChoice{}
	for _, tool := range catalog.Selectable() {
		choices = append(choices, pickerChoice{Value: tool.Name, Label: fmt.Sprintf("%-5s  %s", tool.Name, tool.Description), Colored: "\x1b[1;36m" + fmt.Sprintf("%-5s", tool.Name) + "\x1b[0m  " + tool.Description})
	}
	selection.Tools, e = chooseMany(reader, os.Stdout, choices, selection.Tools, "tools")
	if e != nil {
		return e
	}
	fileChoices := []pickerChoice{{Value: "AGENTS.md", Label: "AGENTS.md  Shared agent instructions"}, {Value: "CLAUDE.md", Label: "CLAUDE.md  Claude project instructions"}}
	selection.Files, e = chooseMany(reader, os.Stdout, fileChoices, selection.Files, "files")
	if e != nil {
		return e
	}
	fmt.Println("[x] Tools and instruction files selected")
	plan, e := agent.Prepare(directory, selection.Tools, selection.Files)
	if e != nil {
		return e
	}
	showAgentPlan(plan)
	if !plan.Changed {
		return nil
	}
	fmt.Print("\n[x] Preview generated\nWrite these changes? [y/N]: ")
	answer, e := reader.ReadString('\n')
	if e != nil {
		return e
	}
	if value := strings.ToLower(strings.TrimSpace(answer)); value != "y" && value != "yes" {
		fmt.Println("Cancelled. No files changed.")
		return nil
	}
	if e = plan.Apply(); e != nil {
		return e
	}
	fmt.Println("[x] Instructions and selections saved. Use worktools agent preview to review future updates.")
	return nil
}

func chooseMany(reader *bufio.Reader, out io.Writer, choices []pickerChoice, defaults []string, title string) ([]string, error) {
	if _, e := exec.LookPath("fzf"); e == nil {
		return chooseManyFZF(choices, defaults, title)
	}
	return checklist(reader, out, choices, defaults, title)
}

func checklist(reader *bufio.Reader, out io.Writer, choices []pickerChoice, defaults []string, title string) ([]string, error) {
	selected := map[string]bool{}
	for _, name := range defaults {
		selected[name] = true
	}
	for {
		fmt.Fprintf(out, "\nSelect %s:\n", title)
		for i, choice := range choices {
			mark := " "
			if selected[choice.Value] {
				mark = "x"
			}
			fmt.Fprintf(out, "  %2d. [%s] %s\n", i+1, mark, choice.Label)
		}
		fmt.Fprint(out, "Toggle numbers (e.g. 1 3), all, none; Enter to continue: ")
		answer, e := reader.ReadString('\n')
		if e != nil {
			return nil, e
		}
		answer = strings.TrimSpace(answer)
		switch answer {
		case "":
			values := []string{}
			for _, choice := range choices {
				if selected[choice.Value] {
					values = append(values, choice.Value)
				}
			}
			if len(values) > 0 {
				return values, nil
			}
			fmt.Fprintln(out, "Select at least one item.")
		case "all", "none":
			for _, choice := range choices {
				selected[choice.Value] = answer == "all"
			}
		default:
			indices := []int{}
			valid := true
			for _, value := range strings.Fields(strings.ReplaceAll(answer, ",", " ")) {
				n, e := strconv.Atoi(value)
				if e != nil || n < 1 || n > len(choices) {
					valid = false
					break
				}
				indices = append(indices, n-1)
			}
			if !valid {
				fmt.Fprintln(out, "Enter numbers from the list, all, or none.")
				continue
			}
			for _, i := range indices {
				selected[choices[i].Value] = !selected[choices[i].Value]
			}
		}
	}
}

func chooseManyFZF(choices []pickerChoice, defaults []string, title string) ([]string, error) {
	selected := map[string]bool{}
	for _, name := range defaults {
		selected[name] = true
	}
	bindings := []string{}
	var input strings.Builder
	_, noColor := os.LookupEnv("NO_COLOR")
	for i, choice := range choices {
		label := choice.Label
		if !noColor && choice.Colored != "" {
			label = choice.Colored
		}
		fmt.Fprintf(&input, "%d\t%s\n", i, label)
		if selected[choice.Value] {
			bindings = append(bindings, fmt.Sprintf("pos(%d)", i+1), "select")
		}
	}
	bindings = append(bindings, "first")
	command := exec.Command("fzf", "--multi", "--ansi", "--no-sort", "--prompt", title+"> ", "--delimiter", "\t", "--with-nth", "2..", "--header", "Tab toggles · Enter continues · Esc cancels", "--bind", "load:"+strings.Join(bindings, "+"))
	command.Stdin = strings.NewReader(input.String())
	command.Stderr = os.Stderr
	output, e := command.Output()
	if e != nil {
		return nil, e
	}
	indices := map[int]bool{}
	for _, line := range strings.Split(strings.TrimSuffix(string(output), "\n"), "\n") {
		key, _, _ := strings.Cut(line, "\t")
		i, e := strconv.Atoi(key)
		if e != nil || i < 0 || i >= len(choices) {
			return nil, errors.New("invalid multi-selection")
		}
		indices[i] = true
	}
	values := []string{}
	for i, choice := range choices {
		if indices[i] {
			values = append(values, choice.Value)
		}
	}
	if len(values) == 0 {
		return nil, errors.New("select at least one item")
	}
	return values, nil
}
