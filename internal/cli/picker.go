package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"unicode"

	"github.com/yetric/worktools/internal/dev"
)

// Keep identifiers separate from presentation: fzf may strip ANSI and transform
// visible fields, and descriptions must never become executable action names.
type pickerChoice struct {
	Value   string
	Label   string
	Colored string
}

func singleLine(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value)
}

func actionChoices(records []dev.Record) ([]pickerChoice, string) {
	nameWidth, statusWidth := len("ACTION"), len("STATUS")
	for _, r := range records {
		nameWidth = max(nameWidth, len(r.Name))
		statusWidth = max(statusWidth, len(r.Status))
	}
	header := fmt.Sprintf("%-*s  %-*s  %s", nameWidth, "ACTION", statusWidth, "STATUS", "DESCRIPTION")
	rows := make([]pickerChoice, 0, len(records))
	for _, r := range records {
		name := fmt.Sprintf("%-*s", nameWidth, singleLine(r.Name))
		status := fmt.Sprintf("%-*s", statusWidth, singleLine(r.Status))
		description := singleLine(r.Description)
		color := "33"
		switch r.Status {
		case "ready":
			color = "32"
		case "unavailable":
			color = "31"
		}
		rows = append(rows, pickerChoice{
			Value:   r.Name,
			Label:   name + "  " + status + "  " + description,
			Colored: "\x1b[1;36m" + name + "\x1b[0m  \x1b[" + color + "m" + status + "\x1b[0m  " + description,
		})
	}
	return rows, header
}

func chooseFZF(choices []pickerChoice, title, header string) (string, error) {
	args := []string{"--ansi", "--prompt", title + "> ", "--delimiter", "\t", "--with-nth", "2.."}
	if header != "" {
		args = append(args, "--header", header)
	}
	command := exec.Command("fzf", args...)
	var input strings.Builder
	_, noColor := os.LookupEnv("NO_COLOR")
	for index, choice := range choices {
		label := choice.Label
		if !noColor && choice.Colored != "" {
			label = choice.Colored
		}
		fmt.Fprintf(&input, "%d\t%s\n", index, label)
	}
	command.Stdin = strings.NewReader(input.String())
	command.Stderr = os.Stderr
	output, err := command.Output()
	if err != nil {
		return "", err
	}
	key, _, _ := strings.Cut(strings.TrimSpace(string(output)), "\t")
	index, err := strconv.Atoi(key)
	if err != nil || index < 0 || index >= len(choices) {
		return "", fmt.Errorf("invalid picker selection")
	}
	return choices[index].Value, nil
}
