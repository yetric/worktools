# Adding a development provider

Every provider lives in its own package under `internal/dev/providers/`. Providers
import only the domain contract (`internal/dev/provider`), not the resolver, CLI,
or sibling providers. Package boundaries are checked by a test.

## Implement the contract

Add `internal/dev/providers/moon/provider.go`:

```go
package moon

import "github.com/yetric/worktools/internal/dev/provider"

type Provider struct{}

var _ provider.Provider = Provider{}

func (Provider) Name() string        { return "moon" }
func (Provider) Manifests() []string { return []string{"Moonfile"} }
func (Provider) Runners() []string   { return []string{"moon"} }

func (Provider) Discover(root string) ([]provider.Action, error) {
    // Inspect Moonfile to discover tasks. Do not execute it.
    return []provider.Action{{
        Name:        "test",
        Runner:      "moon",
        Command:     []string{"moon", "test"},
        Description: "Run Moon tests",
        Reason:      "Moonfile declares a test task",
        Separator:   []string{"--"}, // Only inserted for forwarded arguments.
    }}, nil
}
```

`Discover` is called only when a declared manifest exists at the selected project
root. Return no action for capabilities the project does not declare. Marker
filenames also participate in project-root detection automatically.

Register it in `internal/dev/registry.go`: import the new package and add
`moon.Provider{}` to `Builtins()`. No edits to the CLI, picker, completion, or
execution code are needed.

## Add standalone tests

Create `internal/dev/providers/moon/provider_test.go`:

```go
package moon

import (
    "testing"
    "github.com/yetric/worktools/internal/dev/providertest"
)

func TestProvider(t *testing.T) {
    providertest.Check(t, Provider{}, providertest.Case{
        Files:   map[string]string{"Moonfile": "test: moon test\n"},
        Action:  "test",
        Command: []string{"moon", "test"},
    })
}
```

The shared check verifies metadata, valid action records, repeatable discovery,
expected arguments, and unchanged project files. Add provider-specific tests for
invalid configuration, missing capabilities, wrappers, and version declarations.
Use fixtures and fake tools; do not require an installed Moon runtime.

```sh
go test ./internal/dev/providers/moon
go test ./...
go vet ./...
```

The core's fictitious-environment test separately checks that a new provider
participates in root detection and action resolution through the same interface.

## Action semantics

- `Command` is an executable and argument array, with no shell interpolation.
- `Runner` must be one of the provider's declared identifiers. Users may select it
  with `[dev] runner = "moon"`.
- `Separator` is inserted only when the user forwards additional arguments.
- `Description` and `Reason` explain the choice in the picker, completion, and JSON.
- Execution uses the project root. Explicit project mappings may choose another
  working directory.
- Prefer `start`, `test`, `check`, `build` where meaningful; other actions are welcome.
- Names support letters, digits, underscores, hyphens, dots, colons, and slashes.
  `list` and `explain` are reserved inspection commands.

Discovery must only read files. Do not run build tools, import application code,
mutate files, change directory, contact services, or install dependencies. Prefer
project wrappers and version declarations. Do not invent startup for a library,
or tests that can pass without exercising the project's actual test framework.

Providers are compiled into Go. There is no automatic runtime plugin loading.
For a single unusual project, explicit `.worktools.toml` actions are sufficient.
See [CONTRIBUTING.md](../CONTRIBUTING.md) for the cross-tool rules and the planned
container-provider layout.
