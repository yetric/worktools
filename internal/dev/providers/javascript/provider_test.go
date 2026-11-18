package javascript

import (
	"github.com/yetric/worktools/internal/dev/providertest"
	"testing"
)

func TestProvider(t *testing.T) {
	providertest.Check(t, Provider{}, providertest.Case{
		Files: map[string]string{
			"package.json": "{\"scripts\":{\"test\":\"vitest\"},\"packageManager\":\"pnpm@10\"}",
		},
		Action:  "test",
		Command: []string{"pnpm", "run", "test"},
	})
}
