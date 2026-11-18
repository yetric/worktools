package maven

import (
	"github.com/yetric/worktools/internal/dev/providertest"
	"testing"
)

func TestProvider(t *testing.T) {
	providertest.Check(t, Provider{}, providertest.Case{
		Files: map[string]string{
			"pom.xml": "<project/>",
		},
		Action:  "test",
		Command: []string{"mvn", "test"},
	})
}
