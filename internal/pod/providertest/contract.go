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
		plan, e := p.Plan(info, provider.Request{Action: op.Name, Directory: "/project", Files: []string{"/project/compose.yml"}, Arguments: args})
		if e != nil {
			t.Fatal(e)
		}
		want := []string{name, "compose", "--file", "/project/compose.yml", op.Name}
		if op.Name == "services" {
			want = []string{name, "compose", "--file", "/project/compose.yml", "config", "--services"}
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
	for _, call := range probe.Calls {
		joined := strings.Join(call, " ")
		if joined != name+" compose version" && joined != name+" context show" {
			t.Fatalf("unsafe availability probe: %v", call)
		}
	}
}
