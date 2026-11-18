package javascript

import (
	"encoding/json"
	"fmt"
	"github.com/yetric/worktools/internal/dev/provider"
	"sort"
	"strings"
)

type Provider struct{}

var _ provider.Provider = Provider{}

func (Provider) Name() string        { return "javascript" }
func (Provider) Manifests() []string { return []string{"package.json"} }
func (Provider) Runners() []string   { return []string{"npm", "pnpm", "yarn", "bun"} }

func (Provider) Discover(root string) ([]provider.Action, error) {
	text, e := provider.Read(root, "package.json")
	if e != nil {
		return nil, e
	}
	var manifest struct {
		Scripts        map[string]string `json:"scripts"`
		PackageManager string            `json:"packageManager"`
	}
	if e = json.Unmarshal([]byte(text), &manifest); e != nil {
		return nil, e
	}
	runners := []string{}
	if manifest.PackageManager != "" {
		name := strings.SplitN(manifest.PackageManager, "@", 2)[0]
		if !provider.Contains([]string{"npm", "pnpm", "yarn", "bun"}, name) {
			return nil, fmt.Errorf("unsupported packageManager %q", name)
		}
		runners = append(runners, name)
	} else {
		for _, entry := range []struct {
			runner string
			files  []string
		}{{"npm", []string{"package-lock.json", "npm-shrinkwrap.json"}}, {"pnpm", []string{"pnpm-lock.yaml"}}, {"yarn", []string{"yarn.lock"}}, {"bun", []string{"bun.lock", "bun.lockb"}}} {
			for _, name := range entry.files {
				if provider.File(root, name) {
					runners = append(runners, entry.runner)
					break
				}
			}
		}
		if len(runners) == 0 {
			runners = []string{"npm"}
		}
	}
	result := []provider.Action{}
	names := []string{}
	for n := range manifest.Scripts {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, runner := range runners {
		for _, name := range names {
			if provider.ValidName(name) {
				a := provider.NewAction(name, runner, []string{runner, "run", name}, "package.json script")
				a.Description = "Package script " + name
				if runner == "npm" {
					a.Separator = []string{"--"}
				}
				result = append(result, a)
			}
		}
		if _, ok := manifest.Scripts["dev"]; ok {
			a := provider.NewAction("start", runner, []string{runner, "run", "dev"}, "package.json dev script used for startup")
			if runner == "npm" {
				a.Separator = []string{"--"}
			}
			result = append(result, a)
		}
	}
	return result, nil
}
