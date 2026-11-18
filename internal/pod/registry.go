package pod

import (
	"github.com/yetric/worktools/internal/pod/provider"
	"github.com/yetric/worktools/internal/pod/providers/docker"
	"github.com/yetric/worktools/internal/pod/providers/nerdctl"
	"github.com/yetric/worktools/internal/pod/providers/podman"
)

func Builtins() []provider.Provider {
	return []provider.Provider{docker.Provider{}, podman.Provider{}, nerdctl.Provider{}}
}
