package cargo

import (
	"github.com/yetric/worktools/internal/dev/providertest"
	"testing"
)

func TestProvider(t *testing.T) {
	providertest.Check(t, Provider{}, providertest.Case{
		Files: map[string]string{
			"Cargo.toml": "[package]\nname=\"example\"\nversion=\"0.1.0\"\n",
		},
		Action:  "test",
		Command: []string{"cargo", "test"},
	})
}
