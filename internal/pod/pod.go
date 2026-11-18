package pod

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/yetric/worktools/internal/pod/provider"
)

type NativeProbe struct{ Directory string }

func (NativeProbe) LookPath(name string) (string, error) { return exec.LookPath(name) }
func (NativeProbe) Getenv(name string) string            { return os.Getenv(name) }
func (p NativeProbe) Run(ctx context.Context, argv ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, argv[0], argv[1:]...)
	command.Dir = p.Directory
	return command.Output() // Probe stderr may contain native config details; keep it out of inspection output.
}

type Config struct {
	Provider   string   `toml:"provider"`
	Files      []string `toml:"files"`
	ConfigFile string   `toml:"-"`
}

func LoadConfig(directory string) (Config, error) {
	config := Config{}
	absolute, e := filepath.Abs(directory)
	if e != nil {
		return config, e
	}
	if s, e := os.Stat(absolute); e != nil || !s.IsDir() {
		return config, errors.New("project directory does not exist")
	}
	for path := absolute; ; path = filepath.Dir(path) {
		file := filepath.Join(path, ".worktools.toml")
		if _, e := os.Stat(file); e == nil {
			var value struct {
				Pod Config `toml:"pod"`
			}
			if _, e := toml.DecodeFile(file, &value); e != nil {
				return config, e
			}
			config = value.Pod
			config.ConfigFile = file
			for i, name := range config.Files {
				if name == "" {
					return config, errors.New("pod.files cannot contain an empty filename")
				}
				if !filepath.IsAbs(name) {
					config.Files[i] = filepath.Join(path, name)
				}
			}
			return config, nil
		}
		if _, e := os.Stat(filepath.Join(path, ".git")); e == nil {
			break
		}
		if filepath.Dir(path) == path {
			break
		}
	}
	return config, nil
}
func Preference(cli string, config Config, env string) (string, string) {
	if cli != "" {
		return cli, "--provider"
	}
	if env != "" {
		return env, "WORKTOOLS_POD_PROVIDER"
	}
	if config.Provider != "" {
		return config.Provider, "project .worktools.toml"
	}
	return "", "only available provider"
}
func Inspect(ctx context.Context, registry []provider.Provider, probe provider.Probe) []provider.Info {
	rows := make([]provider.Info, len(registry))
	var wg sync.WaitGroup
	for i, p := range registry {
		wg.Add(1)
		go func(i int, p provider.Provider) { defer wg.Done(); rows[i] = p.Detect(ctx, probe) }(i, p)
	}
	wg.Wait()
	return rows
}
func Select(ctx context.Context, registry []provider.Provider, probe provider.Probe, preferred string) (provider.Provider, provider.Info, error) {
	if preferred != "" {
		for _, p := range registry {
			if p.Name() == preferred {
				info := p.Detect(ctx, probe)
				if !info.Available {
					return nil, info, fmt.Errorf("provider %s unavailable: %s", preferred, info.Reason)
				}
				return p, info, nil
			}
		}
		return nil, provider.Info{}, fmt.Errorf("unknown provider %s", preferred)
	}
	rows := Inspect(ctx, registry, probe)
	available := []int{}
	for i, info := range rows {
		if info.Available {
			available = append(available, i)
		}
	}
	if len(available) == 0 {
		return nil, provider.Info{}, errors.New("no available Compose provider; use pod providers for details")
	}
	if len(available) > 1 {
		names := []string{}
		for _, i := range available {
			names = append(names, rows[i].Name)
		}
		return nil, provider.Info{}, fmt.Errorf("multiple providers available (%s); select --provider or configure [pod] provider", strings.Join(names, ", "))
	}
	i := available[0]
	return registry[i], rows[i], nil
}
func Services(ctx context.Context, p provider.Provider, info provider.Info, probe provider.Probe, request provider.Request) ([]string, error) {
	request.Action = "services"
	request.Arguments = nil
	plan, e := p.Plan(info, request)
	if e != nil {
		return nil, e
	}
	b, e := probe.Run(ctx, plan.Command...)
	if e != nil {
		return nil, errors.New("service discovery failed; check your Compose configuration with its native tool")
	}
	result := []string{}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(b), "\n") {
		name := strings.TrimSpace(line)
		valid := name != ""
		for _, c := range name {
			valid = valid && (c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '.' || c == '-')
		}
		if valid && !seen[name] {
			result = append(result, name)
			seen[name] = true
		}
	}
	sort.Strings(result)
	return result, nil
}
