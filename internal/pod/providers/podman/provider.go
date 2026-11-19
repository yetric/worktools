package podman

import (
	"context"

	"github.com/yetric/worktools/internal/pod/provider"
)

type Provider struct{}

var _ provider.Provider = Provider{}

func (Provider) Name() string { return "podman" }
func (Provider) Detect(ctx context.Context, probe provider.Probe) provider.Info {
	info := provider.Info{Operations: []provider.Operation{}, Name: "podman", Context: "native Podman configuration", ComposeProvider: "configured by Podman", Reason: "podman executable not found"}
	path, err := probe.LookPath("podman")
	if err != nil {
		return info
	}
	info.Executable = path
	info.Available = true
	info.Operations = provider.ContainerOperations()
	info.Compose = []string{path, "compose"}
	if probe.Getenv("PODMAN_COMPOSE_PROVIDER") != "" {
		info.ComposeProvider = "PODMAN_COMPOSE_PROVIDER (set; inherited)"
	}
	if value := probe.Getenv("CONTAINER_CONNECTION"); value != "" {
		info.Context = value
	}
	if probe.Getenv("CONTAINER_HOST") != "" {
		info.EndpointEnvironment = "CONTAINER_HOST (set; inherited)"
	}
	if _, err := probe.Run(ctx, path, "compose", "version"); err != nil {
		info.Reason = "Podman CLI found, but its external Compose provider could not run; configure PODMAN_COMPOSE_PROVIDER or containers.conf"
		return info
	}
	info.ComposeAvailable = true
	info.Operations = append(info.Operations, provider.ComposeOperations()...)
	info.Reason = "Podman and its configured Compose provider available; daemon connectivity not probed"
	return info
}
func (Provider) Plan(info provider.Info, request provider.Request) (provider.Plan, error) {
	if request.Action == "containers" {
		return provider.ContainerPlan(info, request)
	}
	return provider.ComposePlan(info, request)
}
