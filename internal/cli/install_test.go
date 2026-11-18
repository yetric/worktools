package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallerPreservesConflictsAndMigratesOwnedLinks(t *testing.T) {
	root := t.TempDir()
	installer, e := os.ReadFile(filepath.Join("..", "..", "install.sh"))
	if e != nil {
		t.Fatal(e)
	}
	script := write(t, root, "install.sh", string(installer))
	write(t, root, "completions/_worktools", "#compdef worktools dev svc db dots\n")
	prebuilt := write(t, root, "prebuilt", "#!/bin/sh\nprintf 'mock binary\\n'\n")
	prefix := filepath.Join(root, "prefix")
	invokeInstaller := func(prefix string) ([]byte, error) {
		return exec.Command("bash", script, "--prefix", prefix, "--binary", prebuilt).CombinedOutput()
	}
	// The first version installed entry points from bin/. Only these exact owned
	// links may migrate automatically; arbitrary files must remain untouched.
	os.MkdirAll(filepath.Join(prefix, "bin"), 0755)
	os.Symlink(filepath.Join(root, "bin/dev"), filepath.Join(prefix, "bin/dev"))
	for i := 0; i < 2; i++ {
		if b, e := invokeInstaller(prefix); e != nil {
			t.Fatal(string(b), e)
		}
	}
	for _, name := range []string{"worktools", "dev", "svc", "db", "dots"} {
		target, e := os.Readlink(filepath.Join(prefix, "bin", name))
		if e != nil || target != filepath.Join(root, "build/worktools") {
			t.Fatal(name, target, e)
		}
	}
	b, e := exec.Command(filepath.Join(prefix, "bin/dev"), "--help").CombinedOutput()
	if e != nil || strings.TrimSpace(string(b)) != "mock binary" {
		t.Fatal(string(b), e)
	}
	conflict := filepath.Join(root, "conflict")
	write(t, conflict, "bin/db", "keep me")
	if b, e := invokeInstaller(conflict); e == nil {
		t.Fatalf("overwrote conflict: %s", b)
	}
	b, _ = os.ReadFile(filepath.Join(conflict, "bin/db"))
	if string(b) != "keep me" {
		t.Fatal("changed unrelated file")
	}
	if _, e := os.Lstat(filepath.Join(conflict, "bin/worktools")); !os.IsNotExist(e) {
		t.Fatal("partially linked a conflicting installation")
	}
}
