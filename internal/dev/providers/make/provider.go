package make

import (
	"github.com/yetric/worktools/internal/dev/provider"
	"path/filepath"
	"sort"
	"strings"
)

type Provider struct{}

var _ provider.Provider = Provider{}

func (Provider) Name() string        { return "make" }
func (Provider) Manifests() []string { return []string{"GNUmakefile", "makefile", "Makefile"} }
func (Provider) Runners() []string   { return []string{"make"} }

func (Provider) Discover(root string) ([]provider.Action, error) {
	targets := map[string]string{}
	seen := map[string]bool{}
	var scan func(string) error
	scan = func(name string) error {
		path := filepath.Join(root, name)
		if filepath.IsAbs(name) {
			path = name
		}
		if seen[path] {
			return nil
		}
		seen[path] = true
		text, e := provider.Read(filepath.Dir(path), filepath.Base(path))
		if e != nil {
			return nil
		}
		for _, line := range strings.Split(text, "\n") {
			if line == "" || strings.ContainsRune(" \t#", rune(line[0])) {
				continue
			}
			trim := strings.TrimPrefix(line, "-")
			if strings.HasPrefix(trim, "include ") || strings.HasPrefix(trim, "include\t") {
				for _, name := range strings.Fields(strings.SplitN(trim, "#", 2)[0])[1:] {
					if !strings.ContainsAny(name, "$*?%") {
						if e := scan(name); e != nil {
							return e
						}
					}
				}
				continue
			}
			i := strings.Index(line, ":")
			if i < 0 || strings.ContainsAny(line[:i], "=$") || strings.HasPrefix(line[i+1:], "=") {
				continue
			}
			description := ""
			if d := strings.Index(line, "##"); d >= 0 {
				description = strings.TrimSpace(line[d+2:])
			}
			for _, name := range strings.Fields(line[:i]) {
				if provider.ValidName(name) && !strings.HasPrefix(name, ".") {
					d := description
					if d == "" {
						d = "Make target " + name
					}
					targets[name] = d
				}
			}
		}
		return nil
	}
	for _, name := range []string{"GNUmakefile", "makefile", "Makefile"} {
		if provider.File(root, name) {
			if e := scan(name); e != nil {
				return nil, e
			}
			break
		}
	}
	names := []string{}
	for name := range targets {
		names = append(names, name)
	}
	sort.Strings(names)
	result := []provider.Action{}
	for _, name := range names {
		a := provider.NewAction(name, "make", []string{"make", name}, "Makefile target")
		a.Description = targets[name]
		result = append(result, a)
	}
	if d, ok := targets["dev"]; ok {
		a := provider.NewAction("start", "make", []string{"make", "dev"}, "Makefile dev target used for startup")
		a.Description = d
		result = append(result, a)
	}
	return result, nil
}
