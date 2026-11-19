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
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
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
	out, code = invoke(t, root, "complete", "pod", "--kind", "unknown")
	if code != 2 {
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

func TestPodContainersWorksWithoutComposeAndPreservesNativeArguments(t *testing.T) {
	root := fakeDocker(t)
	write(t, root, "docker", `#!/bin/sh
case "$1 $2" in
  'compose version') exit 1 ;;
  'context show') echo test-context; exit 0 ;;
esac
printf '%s\n' "$@" > "$TEST_CAPTURE"
exit 7
`)
	write(t, root, ".worktools.toml", "[pod]\nfiles=['compose.yml']\n")
	out, code := invoke(t, root, "pod", "containers", "--all", "--", "--filter", "name=two words")
	if code != 7 {
		t.Fatal(out, code)
	}
	b, _ := os.ReadFile(filepath.Join(root, "captured"))
	if string(b) != "ps\n--format\ntable {{.Names}}\t{{.Status}}\t{{.Ports}}\n--all\n--filter\nname=two words\n" {
		t.Fatal(string(b))
	}
	out, code = invoke(t, root, "pod", "up", "--dry-run")
	if code != 2 || !strings.Contains(out, "compose version failed") {
		t.Fatal(out, code)
	}
	out, code = invoke(t, root, "complete", "pod")
	if code != 0 || !strings.Contains(out, "containers:") || strings.Contains(out, "up:") {
		t.Fatal(out, code)
	}
}

func TestPodContainersExplainAndScopeValidation(t *testing.T) {
	root := fakeDocker(t)
	out, code := invoke(t, root, "pod", "explain", "containers", "-a", "--json")
	if code != 0 || !strings.Contains(out, `"--all"`) {
		t.Fatal(out, code)
	}
	if _, e := os.Stat(filepath.Join(root, "captured")); !os.IsNotExist(e) {
		t.Fatal("explain executed ps")
	}
	for _, args := range [][]string{{"containers", "--file", "compose.yml"}, {"containers", "app"}, {"up", "--all"}} {
		out, code := invoke(t, root, append([]string{"pod"}, args...)...)
		if code != 2 {
			t.Fatal(args, out, code)
		}
	}
}

func TestPodUserDefaultDoesNotOverrideProjectOrExplicitChoice(t *testing.T) {
	root := fakeDocker(t)
	write(t, root, "podman", "#!/bin/sh\necho version\n")
	write(t, root, "config/worktools/config.toml", "[pod]\nprovider='podman'\n")
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"pod", "explain", "containers", "--json"}, "podman"},
		{[]string{"pod", "explain", "containers", "--provider", "docker", "--json"}, "docker"},
	} {
		out, code := invoke(t, root, tc.args...)
		if code != 0 || !strings.Contains(out, `"provider": "`+tc.want+`"`) {
			t.Fatal(out, code)
		}
	}
	write(t, root, ".worktools.toml", "[pod]\nprovider='docker'\n")
	out, code := invoke(t, root, "pod", "explain", "containers", "--json")
	if code != 0 || !strings.Contains(out, `"provider": "docker"`) {
		t.Fatal(out, code)
	}
}

func TestPodContainerJSONIsConsistentAndKeepsNativeFailures(t *testing.T) {
	root := fakeDocker(t)
	write(t, root, "docker", `#!/bin/sh
case "$1 $2" in
  'compose version') exit 1 ;;
  'context show') echo test-context; exit 0 ;;
esac
printf '%s\n' "$@" > "$TEST_CAPTURE"
printf '%s' "$TEST_CONTAINER_JSON"
exit "${TEST_CONTAINER_EXIT:-0}"
`)
	t.Setenv("TEST_CONTAINER_JSON", `{"id":"123","name":"app","status":"Up","ports":""}`)
	out, code := invoke(t, root, "pod", "containers", "--json")
	var result struct {
		Provider, Scope, Context string
		Containers               []struct{ ID, Name, Status, Ports string }
	}
	if e := json.Unmarshal([]byte(out), &result); e != nil || code != 0 || result.Provider != "docker" || result.Scope != "engine" || len(result.Containers) != 1 || result.Containers[0].Name != "app" {
		t.Fatal(out, code, e)
	}
	t.Setenv("TEST_CONTAINER_JSON", "")
	out, code = invoke(t, root, "pod", "containers", "--json")
	if code != 0 || !strings.Contains(out, `"containers": []`) {
		t.Fatal(out, code)
	}
	t.Setenv("TEST_CONTAINER_JSON", "broken")
	out, code = invoke(t, root, "pod", "containers", "--json")
	if code != 2 || !strings.Contains(out, "invalid JSON") {
		t.Fatal(out, code)
	}
	t.Setenv("TEST_CONTAINER_EXIT", "9")
	out, code = invoke(t, root, "pod", "containers", "--json")
	if code != 9 || strings.Contains(out, `"containers"`) {
		t.Fatal(out, code)
	}
}
