package dev

import (
	"github.com/yetric/worktools/internal/dev/providers/cargo"
	"github.com/yetric/worktools/internal/dev/providers/composer"
	"github.com/yetric/worktools/internal/dev/providers/golang"
	"github.com/yetric/worktools/internal/dev/providers/gradle"
	"github.com/yetric/worktools/internal/dev/providers/javascript"
	"github.com/yetric/worktools/internal/dev/providers/make"
	"github.com/yetric/worktools/internal/dev/providers/maven"
	"github.com/yetric/worktools/internal/dev/providers/python"
)

// Builtins is the only registration point. Each provider owns an independent package.
func Builtins() []Adapter {
	return []Adapter{
		make.Provider{},
		javascript.Provider{},
		composer.Provider{},
		golang.Provider{},
		cargo.Provider{},
		maven.Provider{},
		gradle.Provider{},
		python.Provider{},
	}
}
