package composer

import (
	"github.com/yetric/worktools/internal/dev/providertest"
	"testing"
)

func TestProvider(t *testing.T) {
	providertest.Check(t, Provider{}, providertest.Case{
		Files: map[string]string{
			"composer.json": "{\"scripts\":{\"test\":[\"phpunit\"]}}",
		},
		Action:  "test",
		Command: []string{"composer", "run", "test"},
	})
}
