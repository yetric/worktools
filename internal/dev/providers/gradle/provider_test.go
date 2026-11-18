package gradle

import (
	"github.com/yetric/worktools/internal/dev/providertest"
	"testing"
)

func TestProvider(t *testing.T) {
	providertest.Check(t, Provider{}, providertest.Case{
		Files: map[string]string{
			"build.gradle.kts": "plugins { java }",
		},
		Action:  "test",
		Command: []string{"gradle", "test"},
	})
}
