package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeDocker(t *testing.T) string {
	root := t.TempDir()
	write(t, root, "docker", `#!/bin/sh
case "$1 $2" in
  'compose version') echo 'Docker Compose version 2'; exit 0 ;;
  'context show') echo 'existing-context'; exit 0 ;;
  'compose config') printf 'app\ndb\n'; exit 0 ;;
esac
printf '%s\n' "$@" > "$TEST_CAPTURE"
printf '%s\n' "$PWD" "$DOCKER_CONTEXT" "$COMPOSE_FILE"
exit 7
`)
	t.Setenv("PATH", root)
	t.Setenv("TEST_CAPTURE", filepath.Join(root, "captured"))
	t.Setenv("WORKTOOLS_POD_PROVIDER", "")
	return root
}
func TestPodExplainAndDryRunDoNotExecuteOperation(t *testing.T) {
	root := fakeDocker(t)
	for _, args := range [][]string{{"pod", "explain", "up", "--json"}, {"pod", "up", "--dry-run", "--json"}} {
		out, code := invoke(t, root, args...)
		if code != 0 {
			t.Fatal(out, code)
		}
		var plan struct {
			Provider, Context string
			Command           []string
		}
		if e := json.Unmarshal([]byte(out), &plan); e != nil {
			t.Fatal(out, e)
		}
		if plan.Provider != "docker" || plan.Context != "existing-context" || plan.Command[len(plan.Command)-1] != "up" {
			t.Fatal(plan)
		}
		if _, e := os.Stat(filepath.Join(root, "captured")); !os.IsNotExist(e) {
			t.Fatal("inspection executed an operation")
		}
	}
}
func TestPodExecutionPreservesArgumentsContextDirectoryAndStatus(t *testing.T) {
	root := fakeDocker(t)
	t.Setenv("DOCKER_CONTEXT", "chosen-context")
	t.Setenv("COMPOSE_FILE", "custom.yml")
	out, code := invoke(t, root, "pod", "exec", "app", "--", "sh", "-c", "echo two words")
	if code != 7 || !strings.Contains(out, root+"\nchosen-context\ncustom.yml\n") {
		t.Fatal(out, code)
	}
	b, _ := os.ReadFile(filepath.Join(root, "captured"))
	if string(b) != "compose\nexec\napp\nsh\n-c\necho two words\n" {
		t.Fatal(string(b))
	}
}
func TestPodServicesAndCompletion(t *testing.T) {
	root := fakeDocker(t)
	out, code := invoke(t, root, "pod", "services", "--json")
	if code != 0 || !strings.Contains(out, "app") {
		t.Fatal(out, code)
	}
	out, code = invoke(t, root, "complete", "pod", "--kind", "services")
	if code != 0 || out != "app\ndb\n" {
		t.Fatal(out, code)
	}
}
func TestPodAmbiguityRequiresSelection(t *testing.T) {
	root := fakeDocker(t)
	write(t, root, "podman", "#!/bin/sh\necho version\n")
	out, code := invoke(t, root, "pod", "up", "--dry-run")
	if code != 2 || !strings.Contains(out, "multiple providers") {
		t.Fatal(out, code)
	}
	out, code = invoke(t, root, "pod", "--provider", "docker", "up", "--dry-run", "--json")
	if code != 0 {
		t.Fatal(out, code)
	}
}
func TestPodMissingComposeProvider(t *testing.T) {
	root := t.TempDir()
	write(t, root, "podman", "#!/bin/sh\nexit 1\n")
	t.Setenv("PATH", root)
	out, code := invoke(t, root, "pod", "--provider", "podman", "up")
	if code != 2 || !strings.Contains(out, "external Compose provider") {
		t.Fatal(out, code)
	}
}
