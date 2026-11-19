package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/yetric/worktools/internal/catalog"
	"github.com/yetric/worktools/internal/dev"
	"github.com/yetric/worktools/internal/pod"
	"github.com/yetric/worktools/internal/pod/provider"
)

func podSettings(o options) (string, pod.Config, error) {
	directory := o.Values["--directory"]
	if directory == "" {
		directory = "."
	}
	directory, e := filepath.Abs(expandHome(directory))
	if e != nil {
		return "", pod.Config{}, e
	}
	config, e := pod.LoadConfig(directory)
	if e != nil {
		return "", config, e
	}
	config.DefaultProvider, e = pod.LoadDefaultProvider()
	if e != nil {
		return "", config, e
	}
	if file := o.Values["--file"]; file != "" {
		file = expandHome(file)
		if !filepath.IsAbs(file) {
			file = filepath.Join(directory, file)
		}
		config.Files = []string{file}
	}
	return directory, config, nil
}
func runPod(args []string) (int, error) {
	o, e := parse(args, []string{"--json", "--dry-run", "--pick", "--detach", "--follow", "--all"}, []string{"--directory", "--provider", "--file"})
	if e != nil {
		return 0, e
	}
	if o.Flags["--help"] {
		fmt.Print(catalog.Help("pod"))
		return 0, nil
	}
	action := "ps"
	if len(o.Pos) > 0 {
		action = o.Pos[0]
	} else if terminal(os.Stdin) && terminal(os.Stdout) && !o.Flags["--json"] && !o.Flags["--dry-run"] {
		o.Flags["--pick"] = true
	}
	directory, config, e := podSettings(o)
	if e != nil {
		return 0, e
	}
	registry := pod.Builtins()
	probe := pod.NativeProbe{Directory: directory}
	ctx := context.Background()
	if action == "providers" {
		if len(o.Pos) > 1 || len(o.Native) > 0 {
			return 0, errors.New("providers does not accept operation arguments")
		}
		rows := pod.Inspect(ctx, registry, probe)
		if o.Flags["--json"] {
			return 0, emit(rows, true)
		}
		for _, info := range rows {
			status := "unavailable"
			if info.Available {
				status = "ready"
			}
			fmt.Printf("%-10s %-12s %s\n", info.Name, status, info.Reason)
		}
		return 0, nil
	}
	preferred, source := pod.Preference(o.Values["--provider"], config, os.Getenv("WORKTOOLS_POD_PROVIDER"))
	var p provider.Provider
	var info provider.Info
	if o.Flags["--pick"] && preferred == "" {
		rows := pod.Inspect(ctx, registry, probe)
		choices := []pickerChoice{}
		indices := map[string]int{}
		for i, row := range rows {
			if row.Available {
				choices = append(choices, pickerChoice{Value: row.Name, Label: row.Name + "  " + row.Context})
				indices[row.Name] = i
			}
		}
		if len(choices) == 0 {
			return 0, errors.New("no available container provider; use pod providers")
		}
		selected := choices[0].Value
		if len(choices) > 1 {
			selected, e = chooseRows(choices, "provider", "")
			if e != nil {
				return 0, e
			}
			source = "interactive selection"
		}
		i := indices[selected]
		p = registry[i]
		info = rows[i]
	} else {
		p, info, e = pod.Select(ctx, registry, probe, preferred)
		if e != nil {
			return 0, e
		}
	}
	if action == "actions" {
		if len(o.Pos) > 1 || len(o.Native) > 0 {
			return 0, errors.New("actions does not accept operation arguments")
		}
		if o.Flags["--json"] {
			return 0, emit(info.Operations, true)
		}
		for _, op := range info.Operations {
			fmt.Printf("%-12s %s\n", op.Name, op.Description)
		}
		return 0, nil
	}
	if o.Flags["--pick"] {
		records := []dev.Record{}
		for _, op := range info.Operations {
			records = append(records, dev.Record{Name: op.Name, Description: op.Description, Status: "ready"})
		}
		rows, header := actionChoices(records)
		action, e = chooseRows(rows, "operation", header)
		if e != nil {
			return 0, e
		}
	}
	arguments := []string{}
	if action == "explain" {
		action = "ps"
		if len(o.Pos) > 1 {
			action = o.Pos[1]
		}
		if len(o.Pos) > 2 {
			arguments = append(arguments, o.Pos[2:]...)
		}
	} else if len(o.Pos) > 1 {
		arguments = append(arguments, o.Pos[1:]...)
	}
	if o.Flags["--detach"] {
		if action != "up" {
			return 0, errors.New("--detach is supported for up; pass native flags after --")
		}
		arguments = append([]string{"--detach"}, arguments...)
	}
	if o.Flags["--follow"] {
		if action != "logs" {
			return 0, errors.New("--follow is supported for logs")
		}
		arguments = append([]string{"--follow"}, arguments...)
	}
	arguments = append(arguments, o.Native...)
	if o.Flags["--all"] {
		if action != "containers" {
			return 0, errors.New("--all is supported for containers; pass other native flags after --")
		}
		arguments = append([]string{"--all"}, arguments...)
	}
	request := provider.Request{Action: action, Directory: directory, Files: config.Files, Arguments: arguments}
	if action == "containers" {
		if o.Values["--file"] != "" {
			return 0, errors.New("--file applies to Compose project operations, not containers")
		}
		request.Files = nil
		request.JSON = o.Flags["--json"]
		posCount := 1
		if len(o.Pos) > 0 && o.Pos[0] == "explain" {
			posCount = 2
		}
		if len(o.Pos) > posCount {
			return 0, errors.New("containers does not take names; pass native filters after --")
		}
	}
	plan, e := p.Plan(info, request)
	if e != nil {
		return 0, e
	}
	inspect := len(o.Pos) > 0 && o.Pos[0] == "explain" || o.Flags["--dry-run"]
	if inspect {
		result := struct {
			provider.Plan
			Selection string `json:"selection"`
		}{plan, source}
		if o.Flags["--json"] {
			return 0, emit(result, true)
		}
		fmt.Printf("provider: %s (%s)\nscope: %s\ncontext: %s\ndirectory: %s\ncommand: %s\n", plan.Provider, source, plan.Scope, singleLine(plan.Context), plan.Directory, commandString(plan.Command))
		if plan.ComposeProvider != "" {
			fmt.Printf("compose provider: %s\n", plan.ComposeProvider)
		}
		if plan.EndpointEnvironment != "" {
			fmt.Printf("endpoint: %s\n", plan.EndpointEnvironment)
		}
		return 0, nil
	}
	if action == "services" {
		if len(arguments) > 0 {
			return 0, errors.New("services does not accept operation arguments")
		}
		names, e := pod.Services(ctx, p, info, probe, request)
		if e != nil {
			return 0, e
		}
		if o.Flags["--json"] {
			return 0, emit(names, true)
		}
		for _, name := range names {
			fmt.Println(name)
		}
		return 0, nil
	}
	if action == "containers" && o.Flags["--json"] {
		command := exec.Command(plan.Command[0], plan.Command[1:]...)
		command.Dir = plan.Directory
		command.Stderr = os.Stderr
		output, e := command.Output()
		if e != nil {
			return 0, e
		}
		rows, e := pod.DecodeContainers(bytes.NewReader(output))
		if e != nil {
			return 0, e
		}
		return 0, emit(struct {
			Provider   string          `json:"provider"`
			Scope      string          `json:"scope"`
			Context    string          `json:"context"`
			Containers []pod.Container `json:"containers"`
		}{plan.Provider, plan.Scope, plan.Context, rows}, true)
	}
	if o.Flags["--json"] {
		return 0, errors.New("--json is for providers/actions/services/containers/explain or dry-run; native output flags follow --")
	}
	if action == "exec" && len(arguments) < 2 {
		return 0, errors.New("usage: pod exec SERVICE COMMAND [ARGS...]")
	}
	fmt.Fprintf(os.Stderr, "pod (%s): %s\n", info.Name, commandString(plan.Command))
	return 0, execute(plan.Command, plan.Directory, nil, "")
}
