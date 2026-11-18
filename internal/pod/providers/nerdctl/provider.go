package nerdctl

import (
	"context"

	"github.com/yetric/worktools/internal/pod/provider"
)

type Provider struct{}

var _ provider.Provider = Provider{}

func (Provider) Name() string { return "nerdctl" }
func (Provider) Detect(ctx context.Context, probe provider.Probe) provider.Info {
	info := provider.Info{Name: "nerdctl", Context: "native nerdctl configuration", ComposeProvider: "nerdctl Compose", Operations: provider.ComposeOperations(), Reason: "nerdctl executable not found"}
	path, err := probe.LookPath("nerdctl")
	if err != nil {
		return info
	}
	info.Executable = path
	info.Compose = []string{path, "compose"}
	if _, err := probe.Run(ctx, path, "compose", "version"); err != nil {
		info.Reason = "nerdctl found, but nerdctl compose version failed"
		return info
	}
	info.Available = true
	info.Reason = "nerdctl Compose available; containerd connectivity not probed"
	if value := probe.Getenv("CONTAINERD_NAMESPACE"); value != "" {
		info.Context = "namespace " + value
	}
	if probe.Getenv("CONTAINERD_ADDRESS") != "" {
		info.EndpointEnvironment = "CONTAINERD_ADDRESS (set; inherited)"
	}
	return info
}
func (Provider) Plan(info provider.Info, request provider.Request) (provider.Plan, error) {
	return provider.ComposePlan(info, request)
}
