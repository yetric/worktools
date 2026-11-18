package pod

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yetric/worktools/internal/pod/provider"
	"github.com/yetric/worktools/internal/pod/providertest"
)

func probeFor(names ...string) *providertest.FakeProbe {
	probe := &providertest.FakeProbe{Paths: map[string]string{}, Output: map[string]string{}, Env: map[string]string{}}
	for _, name := range names {
		probe.Paths[name] = name
		probe.Output[name+" compose version"] = "version 1"
	}
	return probe
}
func TestSelectDoesNotGuessBetweenProviders(t *testing.T) {
	_, _, e := Select(context.Background(), Builtins(), probeFor("docker", "podman"), "")
	if e == nil || !strings.Contains(e.Error(), "multiple") {
		t.Fatal(e)
	}
	p, info, e := Select(context.Background(), Builtins(), probeFor("nerdctl"), "")
	if e != nil || p.Name() != "nerdctl" || !info.Available {
		t.Fatal(info, e)
	}
}
func TestExplicitProviderNeverFallsBack(t *testing.T) {
	probe := probeFor("docker")
	_, _, e := Select(context.Background(), Builtins(), probe, "podman")
	if e == nil {
		t.Fatal("fell back from explicit backend")
	}
	if len(probe.Calls) != 0 {
		t.Fatal("probed unrelated backend", probe.Calls)
	}
}
func TestPreference(t *testing.T) {
	for _, tc := range []struct{ cli, env, configured, want string }{{"nerdctl", "docker", "podman", "nerdctl"}, {"", "docker", "podman", "docker"}, {"", "", "podman", "podman"}} {
		got, _ := Preference(tc.cli, Config{Provider: tc.configured}, tc.env)
		if got != tc.want {
			t.Fatal(got, tc)
		}
	}
}
func TestComposeConfigurationAndFileResolution(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, ".worktools.toml"), []byte("[pod]\nprovider='podman'\nfiles=['compose.yml','compose.dev.yml']\n"), 0644)
	nested := filepath.Join(root, "src")
	os.Mkdir(nested, 0755)
	config, e := LoadConfig(nested)
	if e != nil || config.Provider != "podman" || !reflect.DeepEqual(config.Files, []string{filepath.Join(root, "compose.yml"), filepath.Join(root, "compose.dev.yml")}) {
		t.Fatal(config, e)
	}
	os.Mkdir(filepath.Join(nested, ".git"), 0755)
	config, e = LoadConfig(nested)
	if e != nil || config.Provider != "" {
		t.Fatal("crossed repository boundary", config, e)
	}
}
func TestServicesUsesProviderPlanAndFiltersOutput(t *testing.T) {
	probe := probeFor("docker")
	probe.Output["docker compose config --services"] = "db\napp\napp\nnot a service\n"
	p, info, e := Select(context.Background(), Builtins(), probe, "docker")
	if e != nil {
		t.Fatal(e)
	}
	names, e := Services(context.Background(), p, info, probe, provider.Request{Directory: "/project"})
	if e != nil || !reflect.DeepEqual(names, []string{"app", "db"}) {
		t.Fatal(names, e)
	}
}
func TestEndpointValuesAreNotExposed(t *testing.T) {
	probe := probeFor("docker", "podman", "nerdctl")
	probe.Env = map[string]string{"DOCKER_HOST": "tcp://user:secret@host", "CONTAINER_HOST": "ssh://user:secret@host", "CONTAINERD_ADDRESS": "secret"}
	for _, info := range Inspect(context.Background(), Builtins(), probe) {
		if strings.Contains(info.EndpointEnvironment, "secret") {
			t.Fatal(info)
		}
	}
}
