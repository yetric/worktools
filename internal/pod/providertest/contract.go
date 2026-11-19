package providertest

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/yetric/worktools/internal/pod/provider"
)

type FakeProbe struct {
	Paths  map[string]string
	Output map[string]string
	Env    map[string]string
	mu     sync.Mutex
	Calls  [][]string
}

func (f *FakeProbe) LookPath(name string) (string, error) {
	if path := f.Paths[name]; path != "" {
		return path, nil
	}
	return "", fmt.Errorf("not found")
}
func (f *FakeProbe) Getenv(name string) string { return f.Env[name] }
func (f *FakeProbe) Run(ctx context.Context, argv ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, append([]string{}, argv...))
	if value, ok := f.Output[strings.Join(argv, " ")]; ok {
		return []byte(value), nil
	}
	return nil, fmt.Errorf("unexpected/failed probe")
}

func Check(t *testing.T, p provider.Provider) {
	t.Helper()
	name := p.Name()
	probe := &FakeProbe{Paths: map[string]string{name: name}, Output: map[string]string{name + " compose version": "version 1", name + " context show": "test-context"}}
	info := p.Detect(context.Background(), probe)
	if !info.Available || info.Name != name || len(info.Operations) == 0 {
		t.Fatalf("provider detection failed: %+v", info)
	}
	for _, op := range info.Operations {
		args := []string{"app", "two words"}
		if op.Name == "services" {
			args = nil
		}
		files := []string{"/project/compose.yml"}
		if op.Name == "containers" {
			files = nil
			args = []string{"--all", "--filter", "name=two words"}
		}
		plan, e := p.Plan(info, provider.Request{Action: op.Name, Directory: "/project", Files: files, Arguments: args})
		if e != nil {
			t.Fatal(e)
		}
		want := []string{name, "compose", "--file", "/project/compose.yml", op.Name}
		if op.Name == "services" {
			want = []string{name, "compose", "--file", "/project/compose.yml", "config", "--services"}
		}
		if op.Name == "containers" {
			want = []string{name, "ps", "--format", provider.ContainerFormat}
		}
		want = append(want, args...)
		if !reflect.DeepEqual(plan.Command, want) || plan.Directory != "/project" {
			t.Fatalf("%s: got %+v, want %v", op.Name, plan, want)
		}
	}
	if _, e := p.Plan(info, provider.Request{Action: "unsupported"}); e == nil {
		t.Fatal("unsupported capability accepted")
	}
	unavailable := p.Detect(context.Background(), &FakeProbe{})
	if unavailable.Available {
		t.Fatal("missing CLI marked available")
	}
	if _, e := p.Plan(unavailable, provider.Request{Action: "up"}); e == nil {
		t.Fatal("unavailable provider planned an operation")
	}
	engineOnly := p.Detect(context.Background(), &FakeProbe{Paths: map[string]string{name: name}})
	if !engineOnly.Available || engineOnly.ComposeAvailable || !provider.Supports(engineOnly, "containers") || provider.Supports(engineOnly, "up") {
		t.Fatalf("engine-only capabilities incorrect: %+v", engineOnly)
	}
	if _, e := p.Plan(engineOnly, provider.Request{Action: "containers"}); e != nil {
		t.Fatal(e)
	}
	if _, e := p.Plan(engineOnly, provider.Request{Action: "up"}); e == nil {
		t.Fatal("planned Compose operation without Compose")
	}
	if _, e := p.Plan(info, provider.Request{Action: "containers", Files: []string{"compose.yml"}}); e == nil {
		t.Fatal("applied project files to global listing")
	}
	for _, args := range [][]string{{"--quiet"}, {"--format", "{{.ID}}"}, {"--format={{.ID}}"}} {
		plan, e := p.Plan(engineOnly, provider.Request{Action: "containers", Arguments: args})
		want := append([]string{name, "ps"}, args...)
		if e != nil || !reflect.DeepEqual(plan.Command, want) {
			t.Fatal(plan, e, want)
		}
		if _, e := p.Plan(engineOnly, provider.Request{Action: "containers", JSON: true, Arguments: args}); e == nil {
			t.Fatal("accepted conflicting JSON output flags")
		}
	}
	plan, e := p.Plan(engineOnly, provider.Request{Action: "containers", JSON: true})
	if e != nil || !reflect.DeepEqual(plan.Command, []string{name, "ps", "--no-trunc", "--format", provider.ContainerJSONFormat}) {
		t.Fatal(plan, e)
	}
	for _, call := range probe.Calls {
		joined := strings.Join(call, " ")
		if joined != name+" compose version" && joined != name+" context show" {
			t.Fatalf("unsafe availability probe: %v", call)
		}
	}
}
