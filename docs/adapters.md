# Adding a development environment

Adapters discover actions. The core resolves ambiguity, applies project mappings,
builds the picker/JSON output, and executes the selected argument array.
Completion calls the same discovery through `worktools complete dev`.

All environments use the interface in `internal/dev/dev.go`:

```go
type Adapter interface {
    Name() string
    Manifests() []string
    Runners() []string
    Discover(root string) ([]Action, error)
}
```

The result is data, not an executable callback:

```go
Action{
    Name:        "test",
    Runner:      "moon",
    Command:     []string{"moon", "test"},
    Description: "Run Moon tests",
    Reason:      "Moonfile declares a test task",
    Separator:   []string{"--"}, // Optional: inserted before forwarded arguments.
}
```

## Add an adapter

1. Add a file in `internal/dev`, e.g. `moon.go`, containing a static discovery
   function that returns actions.
2. Register it in `Builtins()` with its manifest filenames and runner identifiers:

   ```go
   adapter{"moon", []string{"Moonfile"}, []string{"moon"}, discoverMoon},
   ```

3. Add temporary-project tests. The existing `TestAdapterExtension` uses a fictitious
   Moonfile to demonstrate root detection, descriptions, and argument separators
   without editing the CLI.
4. Run `go test ./...` and rebuild using `./install.sh`.

Existing built-ins are separated into `scripts.go` (Make and package scripts),
`languages.go` (Go, Rust, Java), and `python.go`. Contributors can add separate
files without growing a central dispatch switch. No CLI or completion changes
are required for a new environment or action.

`Manifests` declares marker filenames for project discovery. An adapter is called
only if one of those files exists at the selected root. Runner identifiers can
be selected with `[dev] runner = "moon"`. `Command` is an executable and argument
array, without shell parsing. `Separator` is inserted only when the user forwards
additional arguments. Execution always uses the project root; explicit project
mappings can select another working directory.

## Discovery rules

- Read project files; do not execute task files, subprocesses, or application code.
- Do not change the working directory, mutate files, contact services, or install
  dependencies while discovering actions.
- Return no action for a capability that cannot be identified confidently.
- Prefer project wrappers/version declarations to global guesses.
- Use `start`, `test`, `check`, `build` where meaningful. Other actions are welcome.
- Action names support letters, digits, underscores, hyphens, dots, colons, slashes.
  `list` and `explain` are reserved and omitted from discovered actions.
- Supply a description and reason so humans and agents can understand the choice.
- Do not turn unknown tests into a command that can pass without running them.

Adapters are compiled Go code. There is no dynamic library loading, automatic
Python package discovery, or project-local executable plugin mechanism. This
reduces runtime code-loading paths while leaving contributions straightforward.
For one unusual project, a `.worktools.toml` command mapping is sufficient and
avoids needing an adapter at all.
