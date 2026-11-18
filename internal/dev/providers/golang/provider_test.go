package golang

import (
	"github.com/yetric/worktools/internal/dev/providertest"
	"testing"
)

func TestProvider(t *testing.T) {
	providertest.Check(t, Provider{}, providertest.Case{
		Files: map[string]string{
			"go.mod": "module example.test/app\n",
		},
		Action:  "test",
		Command: []string{"go", "test", "./..."},
	})
}
