package composer

import (
	"encoding/json"
	"github.com/yetric/worktools/internal/dev/provider"
	"sort"
)

type Provider struct{}

var _ provider.Provider = Provider{}

func (Provider) Name() string        { return "composer" }
func (Provider) Manifests() []string { return []string{"composer.json"} }
func (Provider) Runners() []string   { return []string{"composer"} }

func (Provider) Discover(root string) ([]provider.Action, error) {
	text, e := provider.Read(root, "composer.json")
	if e != nil {
		return nil, e
	}
	var manifest struct {
		Scripts      map[string]json.RawMessage `json:"scripts"`
		Descriptions map[string]string          `json:"scripts-descriptions"`
	}
	if e = json.Unmarshal([]byte(text), &manifest); e != nil {
		return nil, e
	}
	result := []provider.Action{}
	names := []string{}
	for n := range manifest.Scripts {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		if provider.ValidName(name) {
			a := provider.NewAction(name, "composer", []string{"composer", "run", name}, "composer.json script")
			a.Separator = []string{"--"}
			if d := manifest.Descriptions[name]; d != "" {
				a.Description = d
			}
			result = append(result, a)
		}
	}
	if _, ok := manifest.Scripts["dev"]; ok {
		a := provider.NewAction("start", "composer", []string{"composer", "run", "dev"}, "composer.json dev script used for startup")
		a.Separator = []string{"--"}
		result = append(result, a)
	}
	return result, nil
}
