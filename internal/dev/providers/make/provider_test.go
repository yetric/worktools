package make

import (
	"github.com/yetric/worktools/internal/dev/providertest"
	"testing"
)

func TestProvider(t *testing.T) {
	providertest.Check(t, Provider{}, providertest.Case{
		Files: map[string]string{
			"Makefile": "test: ## Run tests\n\ttouch MUST_NOT_EXIST\n",
		},
		Action:  "test",
		Command: []string{"make", "test"},
	})
}
