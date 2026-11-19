package docker

import (
	"context"
	"strings"

	"github.com/yetric/worktools/internal/pod/provider"
)

type Provider struct{}

var _ provider.Provider = Provider{}

func (Provider) Name() string { return "docker" }
func (Provider) Detect(ctx context.Context, probe provider.Probe) provider.Info {
	info := provider.Info{Operations: []provider.Operation{}, Name: "docker", Context: "native Docker context", Reason: "docker executable not found"}
	path, err := probe.LookPath("docker")
	if err != nil {
		return info
	}
	info.Executable = path
	info.Available = true
	info.Operations = provider.ContainerOperations()
	info.Compose = []string{path, "compose"}
	info.ComposeProvider = "Docker Compose"
	if value := probe.Getenv("DOCKER_CONTEXT"); value != "" {
		info.Context = value
	} else if output, err := probe.Run(ctx, path, "context", "show"); err == nil && strings.TrimSpace(string(output)) != "" {
		info.Context = strings.TrimSpace(string(output))
	}
	if probe.Getenv("DOCKER_HOST") != "" {
		info.EndpointEnvironment = "DOCKER_HOST (set; inherited)"
	}
	if _, err := probe.Run(ctx, path, "compose", "version"); err != nil {
		info.Reason = "Docker CLI found, but docker compose version failed"
		return info
	}
	info.ComposeAvailable = true
	info.Operations = append(info.Operations, provider.ComposeOperations()...)
	info.Reason = "Docker CLI and Compose available; daemon connectivity not probed"
	return info
}
func (Provider) Plan(info provider.Info, request provider.Request) (provider.Plan, error) {
	if request.Action == "containers" {
		return provider.ContainerPlan(info, request)
	}
	return provider.ComposePlan(info, request)
}
