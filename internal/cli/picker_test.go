package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yetric/worktools/internal/dev"
)

func TestActionPickerAlignmentAndControlCharacters(t *testing.T) {
	rows, header := actionChoices([]dev.Record{
		{Name: "dev", Status: "ready", Description: "Start development"},
		{Name: "frontend", Status: "ambiguous", Description: "Choose a runner\n\x1b[31m"},
	})
	descriptionColumn := strings.Index(header, "DESCRIPTION")
	for i, row := range rows {
		if strings.ContainsAny(row.Label, "\t\n\x1b") {
			t.Fatalf("unsafe plain label: %q", row.Label)
		}
		description := []string{"Start development", "Choose a runner"}[i]
		if strings.Index(row.Label, description) != descriptionColumn {
			t.Fatalf("misaligned: %q vs %q", header, row.Label)
		}
		if !strings.Contains(row.Colored, "\x1b[") {
			t.Fatal("missing action/status colors")
		}
	}
}

func TestPickerSelectionUsesHiddenKeyNotStyledLabel(t *testing.T) {
	root := t.TempDir()
	write(t, root, "fzf", "#!/bin/sh\ncat > \"$PICKER_CAPTURE\"\nprintf '1\\tlabel changed by fzf\\n'\n")
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PICKER_CAPTURE", filepath.Join(root, "input"))
	rows, header := actionChoices([]dev.Record{{Name: "dev", Status: "ready", Description: "Start"}, {Name: "frontend", Status: "ready", Description: "Frontend"}})
	selected, err := chooseFZF(rows, "action", header)
	if err != nil || selected != "frontend" {
		t.Fatal(selected, err)
	}
	input, _ := os.ReadFile(filepath.Join(root, "input"))
	if !strings.HasPrefix(string(input), "0\t") || !strings.Contains(string(input), "1\t") {
		t.Fatal(string(input))
	}
	t.Setenv("NO_COLOR", "1")
	if _, err := chooseFZF(rows, "action", header); err != nil {
		t.Fatal(err)
	}
	input, _ = os.ReadFile(filepath.Join(root, "input"))
	if strings.ContainsRune(string(input), '\x1b') {
		t.Fatal("NO_COLOR ignored")
	}
}
