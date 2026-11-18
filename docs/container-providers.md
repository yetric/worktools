# Adding a container provider

Container providers live in independent packages under `internal/pod/providers`.
Docker, Podman, and nerdctl are examples. Add a new package and one explicit entry
in `internal/pod/registry.go`; the CLI, picker, JSON inspection, and completion
consume the provider's capability records.

The contract is in `internal/pod/provider`:

```go
type Provider interface {
    Name() string
    Detect(context.Context, Probe) Info
    Plan(Info, Request) (Plan, error)
}
```

`Detect` uses an injected `Probe` for executable lookup, environment reads, and
read-only native commands. Report Compose availability separately from executable
availability. Do not start a daemon or VM, change contexts, create resources, or
execute project scripts during detection. Native probes have bounded timeouts.
Availability does not guarantee that an engine is running.

`Info.Operations` explicitly lists supported operations and descriptions. `Plan`
returns an argument array and working directory without executing anything.
Unsupported operations must fail clearly. Providers with Compose-compatible
arguments can use the pure `provider.ComposePlan` helper; a different CLI can
implement translation itself. Do not silently change the meaning of native flags.
Keep native context/endpoint configuration intact and omit credential values
from inspection records.

Providers may import their domain contract, standard packages, and justified
libraries. They must not import sibling providers, the resolver, or CLI code.
An architecture test enforces this dependency boundary. Shared selection,
configuration, execution, UI, and completion belong in the core.

## Tests and review

- [ ] Add `internal/pod/providers/yourtool/provider.go` and provider tests.
- [ ] Register it in `internal/pod/registry.go`.
- [ ] Test read-only detection, missing CLI/Compose, capabilities, argument
      preservation, explicit errors, and provider-specific configuration.
- [ ] Use `providertest.FakeProbe`; no running engine should be needed.
- [ ] Use `providertest.Check` for a Compose-compatible provider. Different command
      families should provide equivalent contract assertions for their layout.
- [ ] Document supported operations and limits in the README.
- [ ] Run `go test ./...`, `go test -race ./...`, and `go vet ./...`.

Podman deliberately delegates through `podman compose`, retaining its configured
external provider and `PODMAN_COMPOSE_PROVIDER`. Docker-compatible environments
usually need their existing Docker context rather than another provider. Server
software such as nginx and PM2 belongs to the separate design discussion in
[issue #1](https://github.com/yetric/worktools/issues/1).
