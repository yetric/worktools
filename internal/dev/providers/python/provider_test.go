package python

import (
	"github.com/yetric/worktools/internal/dev/providertest"
	"testing"
)

func TestProvider(t *testing.T) {
	providertest.Check(t, Provider{}, providertest.Case{
		Files: map[string]string{
			"pyproject.toml": "[tool.pytest.ini_options]\n",
		},
		Action:  "test",
		Command: []string{"python3", "-m", "pytest"},
	})
}
