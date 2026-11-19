package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"github.com/yetric/worktools/internal/catalog"
	"github.com/yetric/worktools/internal/dev"
)

type options struct {
	Pos, Native []string
	Values      map[string]string
	Flags       map[string]bool
}

func parse(args []string, booleans, values []string) (options, error) {
	o := options{Values: map[string]string{}, Flags: map[string]bool{}}
	has := func(values []string, v string) bool {
		for _, x := range values {
			if x == v {
				return true
			}
		}
		return false
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			o.Native = args[i+1:]
			break
		}
		if arg == "-h" {
			arg = "--help"
		}
		if arg == "-f" {
			arg = "--follow"
		}
		if arg == "-n" {
			arg = "--lines"
		}
		if arg == "-d" {
			arg = "--detach"
		}
		if arg == "-a" {
			arg = "--all"
		}
		if strings.HasPrefix(arg, "-") {
			name, value, assigned := strings.Cut(arg, "=")
			if name == "--help" || has(booleans, name) {
				if assigned {
					return o, fmt.Errorf("%s does not take a value", name)
				}
				o.Flags[name] = true
				continue
			}
			if !has(values, name) {
				return o, fmt.Errorf("unknown option %s; pass runner arguments after --", name)
			}
			if !assigned {
				i++
				if i == len(args) {
					return o, fmt.Errorf("missing value for %s", name)
				}
				value = args[i]
			}
			o.Values[name] = value
		} else {
			o.Pos = append(o.Pos, arg)
		}
	}
	return o, nil
}
func emit(value any, asJSON bool) error {
	if asJSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(value)
	}
	return json.NewEncoder(os.Stdout).Encode(value)
}
func terminal(file *os.File) bool {
	var state syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, file.Fd(), syscall.TCGETS, uintptr(unsafe.Pointer(&state)))
	return errno == 0
}
func execute(argv []string, directory string, env []string, stdin string) error {
	if len(argv) == 0 {
		return errors.New("empty command")
	}
	if directory != "" {
		if e := os.Chdir(directory); e != nil {
			return e
		}
	}
	path, e := exec.LookPath(argv[0])
	if e != nil {
		return e
	}
	if stdin != "" {
		file, e := os.Open(stdin)
		if e != nil {
			return e
		}
		defer file.Close()
		if e = syscall.Dup2(int(file.Fd()), 0); e != nil {
			return e
		}
	}
	if env == nil {
		env = os.Environ()
	}
	return syscall.Exec(path, argv, env)
}
func capture(argv ...string) ([]byte, error) {
	command := exec.Command(argv[0], argv[1:]...)
	command.Stderr = os.Stderr
	return command.Output()
}
func choose(choices []string, title string) (string, error) {
	rows := make([]pickerChoice, 0, len(choices))
	for _, choice := range choices {
		rows = append(rows, pickerChoice{Value: choice, Label: singleLine(choice)})
	}
	return chooseRows(rows, title, "")
}

func chooseRows(choices []pickerChoice, title, header string) (string, error) {
	if !terminal(os.Stdin) || !terminal(os.Stdout) {
		return "", errors.New("picker requires a terminal; pass a name explicitly")
	}
	if _, e := exec.LookPath("fzf"); e == nil {
		return chooseFZF(choices, title, header)
	}
	if header != "" {
		fmt.Printf("   %s\n", header)
	}
	for i, choice := range choices {
		fmt.Printf("%d. %s\n", i+1, choice.Label)
	}
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("%s (number, Ctrl+C to cancel): ", title)
		line, e := reader.ReadString('\n')
		if e != nil {
			return "", e
		}
		n, e := strconv.Atoi(strings.TrimSpace(line))
		if e == nil && n > 0 && n <= len(choices) {
			return choices[n-1].Value, nil
		}
		fmt.Println("Choose a number from the list.")
	}
}
func commandString(argv []string) string {
	parts := []string{}
	for _, arg := range argv {
		if arg != "" && !strings.ContainsAny(arg, " \t\n'\"$;&|<>*?()") {
			parts = append(parts, arg)
		} else {
			parts = append(parts, "'"+strings.ReplaceAll(arg, "'", "'\\''")+"'")
		}
	}
	return strings.Join(parts, " ")
}

func Run(argv []string) int {
	tool := filepath.Base(argv[0])
	args := argv[1:]
	if tool == "worktools" {
		if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
			fmt.Println("Usage: worktools TOOL [arguments]\n\nTools:")
			for _, tool := range catalog.Tools() {
				fmt.Printf("  %-7s %s\n", tool.Name, tool.Description)
			}
			fmt.Println("\nEach tool supports --help. dev, pod, svc, db and dots also have short entry points.")
			return 0
		}
		tool = args[0]
		args = args[1:]
	}
	var code int
	var e error
	switch tool {
	case "complete":
		code, e = runComplete(args)
	case "agent":
		code, e = runAgent(args)
	case "dev":
		code, e = runDev(args)
	case "pod":
		code, e = runPod(args)
	case "svc":
		code, e = runSvc(args)
	case "db":
		code, e = runDB(args)
	case "dots":
		code, e = runDots(args)
	default:
		e = fmt.Errorf("unknown tool %s", tool)
	}
	if e != nil {
		if errors.Is(e, io.EOF) {
			return 130
		}
		var exit *exec.ExitError
		if errors.As(e, &exit) {
			if n := exit.ExitCode(); n > 0 {
				return n
			}
			return 130
		}
		fmt.Fprintf(os.Stderr, "%s: %v\n", tool, e)
		return 2
	}
	return code
}

func explain(r dev.Record, asJSON bool) error {
	if asJSON {
		return emit(r, true)
	}
	fmt.Printf("%s: %s\n  %s\n", r.Name, r.Status, r.Reason)
	if len(r.Command) > 0 {
		fmt.Printf("  command: %s\n  directory: %s\n  executable available: %v\n", commandString(r.Command), r.Directory, r.ExecutableAvailable)
	} else {
		for _, c := range r.Candidates {
			fmt.Printf("  %s: %s\n", c.Runner, commandString(c.Command))
		}
	}
	return nil
}

func runDev(args []string) (int, error) {
	o, e := parse(args, []string{"--json", "--dry-run", "--pick"}, []string{"--directory"})
	if e != nil {
		return 0, e
	}
	if o.Flags["--help"] {
		fmt.Print(catalog.Help("dev"))
		return 0, nil
	}
	name := "start"
	if len(o.Pos) > 0 {
		name = o.Pos[0]
	} else if terminal(os.Stdin) && terminal(os.Stdout) && !o.Flags["--dry-run"] && !o.Flags["--json"] {
		o.Flags["--pick"] = true
	}
	inspect := name == "list" || name == "explain"
	if inspect && len(o.Native) > 0 {
		return 0, errors.New("inspection commands do not accept runner arguments")
	}
	if name == "explain" {
		if len(o.Pos) != 2 {
			return 0, errors.New("usage: dev explain ACTION")
		}
	} else if len(o.Pos) > 1 {
		return 0, errors.New("pass runner arguments after --")
	}
	if o.Flags["--pick"] && inspect {
		return 0, errors.New("use --pick without list or explain")
	}
	if o.Flags["--json"] && !inspect && !o.Flags["--dry-run"] {
		return 0, errors.New("--json is for inspection; pass a runner's --json after --")
	}
	directory := o.Values["--directory"]
	if directory == "" {
		directory, e = os.Getwd()
		if e != nil {
			return 0, e
		}
	}
	registry := dev.Builtins()
	root, e := dev.Root(expandHome(directory), registry)
	if e != nil {
		return 0, e
	}
	records, e := dev.Discover(root, registry)
	if e != nil {
		return 0, e
	}
	if name == "list" {
		if o.Flags["--json"] {
			return 0, emit(records, true)
		}
		for _, r := range records {
			fmt.Printf("%-18s %-12s %s\n", r.Name, r.Status, r.Description)
		}
		return 0, nil
	}
	if name == "explain" {
		name = o.Pos[1]
	}
	if o.Flags["--pick"] {
		choices, header := actionChoices(records)
		if len(choices) == 0 {
			return 0, errors.New("no actions found; configure dev.actions in .worktools.toml")
		}
		choice, e := chooseRows(choices, "action", header)
		if e != nil {
			return 0, e
		}
		name = choice
	}
	var selected *dev.Record
	for i := range records {
		if records[i].Name == name {
			selected = &records[i]
			break
		}
	}
	if selected == nil {
		return 0, fmt.Errorf("unknown project action %s; use dev list", name)
	}
	if len(o.Native) > 0 && len(selected.Command) > 0 {
		selected.Command = append(append(append([]string{}, selected.Command...), selected.Separator...), o.Native...)
	}
	if inspect || o.Flags["--dry-run"] {
		code := 0
		if selected.Status != "ready" {
			code = 1
		}
		return code, explain(*selected, o.Flags["--json"])
	}
	if selected.Status != "ready" {
		return 0, fmt.Errorf("%s is %s; %s", name, selected.Status, selected.Reason)
	}
	fmt.Fprintf(os.Stderr, "dev: %s\n", commandString(selected.Command))
	return 0, execute(selected.Command, selected.Directory, nil, "")
}

func expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(path, "~"), "/"))
	}
	return path
}
