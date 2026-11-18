package cargo

import (
	"github.com/BurntSushi/toml"
	"github.com/yetric/worktools/internal/dev/provider"
)

type Provider struct{}

var _ provider.Provider = Provider{}

func (Provider) Name() string        { return "cargo" }
func (Provider) Manifests() []string { return []string{"Cargo.toml"} }
func (Provider) Runners() []string   { return []string{"cargo"} }

func (Provider) Discover(root string) ([]provider.Action, error) {
	var manifest struct {
		Package struct {
			Name       string
			DefaultRun string `toml:"default-run"`
		}
		Bin []struct{ Name, Path string }
	}
	if _, e := toml.DecodeFile(root+"/Cargo.toml", &manifest); e != nil {
		return nil, e
	}
	result := []provider.Action{}
	for _, name := range []string{"test", "check", "build", "clean"} {
		result = append(result, provider.NewAction(name, "cargo", []string{"cargo", name}, "Cargo.toml lifecycle"))
	}
	count := len(manifest.Bin)
	mainDeclared := false
	for _, b := range manifest.Bin {
		if b.Path == "src/main.rs" || b.Name == manifest.Package.Name {
			mainDeclared = true
		}
	}
	if provider.File(root, "src/main.rs") && !mainDeclared {
		count++
	}
	if manifest.Package.DefaultRun != "" || (manifest.Package.Name != "" && count == 1) {
		result = append(result, provider.NewAction("start", "cargo", []string{"cargo", "run"}, "Cargo binary or package.default-run"))
	}
	return result, nil
}
