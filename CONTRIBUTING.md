# Contributing providers

Worktools grows through independent, compiled Go providers. Adding another
environment should not require learning or modifying the picker, CLI parser,
completion, or process execution code.

## Package boundaries

Each provider owns one package with its implementation and tests:

```text
internal/dev/
  provider/                 # Domain contract and small pure helpers
  providers/
    cargo/
      provider.go
      provider_test.go
    maven/
      provider.go
      provider_test.go
    yourtool/
      provider.go
      provider_test.go
  providertest/             # Reusable contract checks
  registry.go               # Explicit provider registration
```

A provider may depend on its domain's contract, standard Go packages, and justified
libraries. It must not depend on another provider, the CLI, or the resolver.
An architecture test enforces the internal-package boundary. A provider-specific
fix should stay in that provider's package.

The core owns selection, ambiguity, project overrides, output formats, pickers,
completion, and execution. Providers return typed data and command argument arrays.
They never build shell strings for the core to execute.

## A provider PR should contain

1. The provider's package and tests.
2. One registry entry.
3. A short description of detection, supported actions, and limitations.

Use the [development-provider guide](docs/adapters.md) for a working template.
The reusable contract checks use temporary fixtures, validate returned actions,
check repeatability, and detect project-file modifications. Runtime tools are
not required for discovery tests. Root discovery, ambiguity, and CLI behavior
are covered separately by integration tests.

```sh
go test ./internal/dev/providers/yourtool
go test ./...
go test -race ./...
go vet ./...
```

If a capability cannot be identified reliably, leave it unavailable and document
the project override. Do not silently pick whichever executable is installed.

## Container-provider design

`pod` is planned, not shipped yet. Its providers must follow the same package and
PR boundaries: `internal/pod/provider`, `internal/pod/providers/docker`,
`internal/pod/providers/podman`, `internal/pod/providers/nerdctl`, plus a registry
and reusable contract tests.

The container contract should be specific to containers, not reuse the development
discovery interface or grow a universal provider interface. Each provider owns:

- Read-only availability detection, including the separate Compose provider.
- Explicit capability reporting; unsupported operations fail clearly.
- Translating supported operations into argument arrays.
- Provider-specific fixtures and tests without requiring a running engine.

Shared `pod` code will own provider selection, context preservation, UI, JSON,
and execution. Detection must not start daemons/VMs or switch contexts. Docker-
compatible environments can use the Docker provider; distinct command families
need their own providers. Backend-specific flags must not be silently translated
into something with a different meaning.

## Runtime extensions

Providers are reviewed through PRs and compiled into the binary. Worktools does
not automatically import installed packages, project-local plugins, or shared
libraries. A one-off project can use a `.worktools.toml` action mapping instead of
adding a provider.
