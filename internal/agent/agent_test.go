package agent

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func put(t *testing.T, root, name, text string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if e := os.MkdirAll(filepath.Dir(path), 0755); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(path, []byte(text), 0644); e != nil {
		t.Fatal(e)
	}
	return path
}
func get(t *testing.T, path string) string {
	t.Helper()
	data, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	return string(data)
}

func TestPreviewWritesNothingAndSyncIsIdempotent(t *testing.T) {
	root := t.TempDir()
	path := put(t, root, "AGENTS.md", "# Handwritten instructions\n\nKeep this exactly.\n")
	original := get(t, path)
	plan, e := Prepare(root, []string{"dev"}, []string{"AGENTS.md"})
	if e != nil {
		t.Fatal(e)
	}
	if get(t, path) != original {
		t.Fatal("preview modified instructions")
	}
	if _, e := os.Lstat(filepath.Join(root, ".worktools")); !os.IsNotExist(e) {
		t.Fatal("preview created selection directory")
	}
	if !plan.Changed || !strings.Contains(plan.Files[0].Diff, "+"+Start) {
		t.Fatal(plan)
	}
	if e = plan.Apply(); e != nil {
		t.Fatal(e)
	}
	written := get(t, path)
	if !strings.HasPrefix(written, original) || !strings.Contains(written, Start) {
		t.Fatal(written)
	}
	info, _ := os.Stat(path)
	selection, saved, e := LoadSelection(root)
	if e != nil || !saved || !reflect.DeepEqual(selection.Tools, []string{"dev"}) {
		t.Fatal(selection, saved, e)
	}
	plan, e = Prepare(root, nil, nil)
	if e != nil || plan.Changed {
		t.Fatal(plan, e)
	}
	if e = plan.Apply(); e != nil {
		t.Fatal(e)
	}
	now, _ := os.Stat(path)
	if !os.SameFile(info, now) || !info.ModTime().Equal(now.ModTime()) {
		t.Fatal("no-op sync rewrote instructions")
	}
}

func TestReplacementPreservesHandwrittenBytesAndLineEndings(t *testing.T) {
	for _, newline := range []string{"\n", "\r\n"} {
		prefix := "# Keep exactly" + newline + newline
		suffix := newline + "\tHandwritten tail without a newline"
		original := prefix + Start + newline + "old" + newline + End + suffix
		block := Start + "\nnew\n" + End + "\n"
		updated, legacy, e := Replace(original, block, false)
		want := prefix + Start + newline + "new" + newline + End + suffix
		if e != nil || legacy || updated != want {
			t.Fatalf("got %q; want %q (%v)", updated, want, e)
		}
	}
}

func TestMalformedOrMixedMarkersRejectWholePlan(t *testing.T) {
	for _, invalid := range []string{Start, End, End + "\n" + Start, Start + "\n" + Start + "\n" + End, "inline " + Start + "\n" + End, Start + " extra\n" + End, Start + "\n" + End + "\n" + LegacyStart + "\n" + LegacyEnd} {
		root := t.TempDir()
		put(t, root, "AGENTS.md", "handwritten")
		put(t, root, "CLAUDE.md", invalid)
		if _, e := Prepare(root, []string{"pod"}, nil); e == nil {
			t.Fatal("accepted malformed markers", invalid)
		}
		if get(t, filepath.Join(root, "AGENTS.md")) != "handwritten" {
			t.Fatal("modified valid sibling before rejecting malformed file")
		}
		if _, e := os.Stat(filepath.Join(root, SelectionFile)); !os.IsNotExist(e) {
			t.Fatal("created configuration on failed preview")
		}
	}
}

func TestLegacyMigrationAndDeselection(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"AGENTS.md", "CLAUDE.md"} {
		put(t, root, name, "keep\n\n"+LegacyStart+"\nold aliases\n"+LegacyEnd+"\ntail\n")
	}
	plan, e := Prepare(root, []string{"pod"}, nil)
	if e != nil || !plan.Files[0].Migrated {
		t.Fatal(plan, e)
	}
	if e = plan.Apply(); e != nil {
		t.Fatal(e)
	}
	plan, e = Prepare(root, nil, []string{"AGENTS.md"})
	if e != nil {
		t.Fatal(e)
	}
	if e = plan.Apply(); e != nil {
		t.Fatal(e)
	}
	claude := get(t, filepath.Join(root, "CLAUDE.md"))
	if strings.Contains(claude, Start) || !strings.HasPrefix(claude, "keep\n") || !strings.HasSuffix(claude, "tail\n") {
		t.Fatal(claude)
	}
}

func TestSharedSymlinksAreUpdatedOnceAndPreserved(t *testing.T) {
	root := t.TempDir()
	target := put(t, root, "instructions.md", "handwritten\n")
	for _, name := range []string{"AGENTS.md", "CLAUDE.md"} {
		if e := os.Symlink("instructions.md", filepath.Join(root, name)); e != nil {
			t.Fatal(e)
		}
	}
	plan, e := Prepare(root, []string{"pod"}, nil)
	if e != nil || len(plan.Files) != 2 || len(plan.Files[0].Aliases) != 2 {
		t.Fatal(plan, e)
	}
	if e = plan.Apply(); e != nil {
		t.Fatal(e)
	}
	if strings.Count(get(t, target), Start) != 1 {
		t.Fatal("duplicate shared block")
	}
	for _, name := range []string{"AGENTS.md", "CLAUDE.md"} {
		if link, e := os.Readlink(filepath.Join(root, name)); e != nil || link != "instructions.md" {
			t.Fatal(link, e)
		}
	}
	plan, e = Prepare(root, nil, []string{"AGENTS.md"})
	if e != nil {
		t.Fatal(e)
	}
	if e = plan.Apply(); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(get(t, target), Start) {
		t.Fatal("deselected alias removed a retained shared reference")
	}
}

func TestOutsideDanglingAndConfigurationSymlinksRejected(t *testing.T) {
	for _, kind := range []string{"outside", "dangling", "config"} {
		root, outside := t.TempDir(), t.TempDir()
		path := put(t, outside, "shared.md", "outside instructions")
		switch kind {
		case "outside":
			os.Symlink(path, filepath.Join(root, "AGENTS.md"))
		case "dangling":
			os.Symlink("missing.md", filepath.Join(root, "AGENTS.md"))
		case "config":
			os.Symlink(outside, filepath.Join(root, ".worktools"))
		}
		if _, e := Prepare(root, []string{"pod"}, []string{"AGENTS.md"}); e == nil {
			t.Fatal("accepted unsafe link", kind)
		}
		if get(t, path) != "outside instructions" {
			t.Fatal("modified external file")
		}
	}
}

func TestConcurrentEditsRejectBeforeAnyWrites(t *testing.T) {
	root := t.TempDir()
	put(t, root, "AGENTS.md", "keep agents")
	claude := put(t, root, "CLAUDE.md", "keep claude")
	plan, e := Prepare(root, []string{"pod"}, nil)
	if e != nil {
		t.Fatal(e)
	}
	put(t, root, "CLAUDE.md", "edited while previewing")
	if e = plan.Apply(); e == nil || !strings.Contains(e.Error(), "changed since preview") {
		t.Fatal(e)
	}
	if get(t, filepath.Join(root, "AGENTS.md")) != "keep agents" || get(t, claude) != "edited while previewing" {
		t.Fatal("overwrote concurrent/user content")
	}
	if _, e := os.Stat(filepath.Join(root, SelectionFile)); !os.IsNotExist(e) {
		t.Fatal("wrote selection despite conflict")
	}
}

func TestExistingPermissionsAndProjectConfigRemainIntact(t *testing.T) {
	root := t.TempDir()
	path := put(t, root, "AGENTS.md", "handwritten\n")
	if e := os.Chmod(path, 0600); e != nil {
		t.Fatal(e)
	}
	project := put(t, root, ".worktools.toml", "# Project comments\n[dev.actions.check]\ncommand=['true']\n")
	original := get(t, project)
	plan, e := Prepare(root, []string{"dev"}, []string{"AGENTS.md"})
	if e != nil {
		t.Fatal(e)
	}
	if e = plan.Apply(); e != nil {
		t.Fatal(e)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 || get(t, project) != original {
		t.Fatal("changed permissions or project configuration")
	}
}

func TestProjectDescriptionsAndPreferencesAreReadOnlyAndEscaped(t *testing.T) {
	root := t.TempDir()
	put(t, root, "Makefile", "test: ## Test | <script> `text`\n\ttouch MUST_NOT_EXIST\n")
	config := "[dev]\nrunner='make'\n[pod]\nprovider='nerdctl'\n"
	put(t, root, ".worktools.toml", config)
	plan, e := Prepare(root, []string{"dev", "pod"}, []string{"AGENTS.md"})
	if e != nil {
		t.Fatal(e)
	}
	text := string(plan.Files[0].updated)
	for _, want := range []string{"dev test", "Test &#124; &lt;script&gt; &#96;text&#96;", "<code>make</code>", "<code>nerdctl</code>"} {
		if !strings.Contains(text, want) {
			t.Fatal("missing project metadata", want, text)
		}
	}
	if _, e := os.Stat(filepath.Join(root, "MUST_NOT_EXIST")); !os.IsNotExist(e) {
		t.Fatal("executed project command")
	}
	if get(t, filepath.Join(root, ".worktools.toml")) != config {
		t.Fatal("rewrote project preferences")
	}
}

func TestSelectionValidationAndMalformedConfiguration(t *testing.T) {
	for _, selection := range []Selection{{Version: 2, Tools: []string{"dev"}, Files: []string{"AGENTS.md"}}, {Version: 1, Tools: []string{"unknown"}, Files: []string{"AGENTS.md"}}, {Version: 1, Tools: []string{"dev"}, Files: []string{"../AGENTS.md"}}, {Version: 1, Files: []string{"AGENTS.md"}}} {
		if _, e := Normalize(selection); e == nil {
			t.Fatal("accepted invalid selection", selection)
		}
	}
	root := t.TempDir()
	put(t, root, SelectionFile, "version=1\ntools=['dev']\nfiles=['AGENTS.md']\nextra='unexpected'\n")
	if _, _, e := LoadSelection(root); e == nil {
		t.Fatal("ignored unsupported settings")
	}
}

func TestUnifiedDiffIncludesContextAndMissingNewlines(t *testing.T) {
	if diff := unifiedDiff("AGENTS.md", "old", "new\n", false); !strings.Contains(diff, "-old\n\\ No newline at end of file\n+new\n") {
		t.Fatal(diff)
	}
	if diff := unifiedDiff("AGENTS.md", "", "new\n", true); !strings.Contains(diff, "--- /dev/null\n") || !strings.Contains(diff, "@@ -0,0 +1 @@") {
		t.Fatal(diff)
	}
}
