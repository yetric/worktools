package docker

import (
	"github.com/yetric/worktools/internal/pod/providertest"
	"testing"
)

func TestProvider(t *testing.T) { providertest.Check(t, Provider{}) }
