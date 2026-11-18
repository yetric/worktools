// Package provider defines the standalone container-provider contract.
package provider

import (
	"context"
	"fmt"
	"strings"
)

type Operation struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}
type Info struct {
	Name                string      `json:"name"`
	Available           bool        `json:"available"`
	Executable          string      `json:"executable,omitempty"`
	Compose             []string    `json:"compose_command,omitempty"`
	ComposeProvider     string      `json:"compose_provider,omitempty"`
	Context             string      `json:"context,omitempty"`
	EndpointEnvironment string      `json:"endpoint_environment,omitempty"`
	Reason              string      `json:"reason"`
	Operations          []Operation `json:"operations"`
}

// Probe allows read-only checks to be tested without installed engines.
type Probe interface {
	LookPath(string) (string, error)
	Run(context.Context, ...string) ([]byte, error)
	Getenv(string) string
}
type Request struct {
	Action, Directory string
	Files, Arguments  []string
}
type Plan struct {
	Provider            string   `json:"provider"`
	Action              string   `json:"action"`
	Command             []string `json:"command"`
	Directory           string   `json:"directory"`
	Context             string   `json:"context"`
	ComposeProvider     string   `json:"compose_provider"`
	EndpointEnvironment string   `json:"endpoint_environment,omitempty"`
}
type Provider interface {
	Name() string
	Detect(context.Context, Probe) Info
	Plan(Info, Request) (Plan, error)
}

func ComposeOperations() []Operation {
	return []Operation{
		{"up", "Create and start project services"}, {"down", "Stop and remove project containers and networks"},
		{"ps", "List this project's containers"}, {"logs", "Read project service logs"},
		{"exec", "Run a command inside a service"}, {"restart", "Restart project services"},
		{"build", "Build project service images"}, {"services", "List configured service names"},
	}
}
func Supports(info Info, name string) bool {
	for _, operation := range info.Operations {
		if operation.Name == name {
			return true
		}
	}
	return false
}

// ComposePlan only shares the common Compose argument layout. A provider with a
// different CLI can implement Plan directly without coupling to another backend.
func ComposePlan(info Info, request Request) (Plan, error) {
	if !info.Available {
		return Plan{}, fmt.Errorf("provider %s is unavailable: %s", info.Name, info.Reason)
	}
	if !Supports(info, request.Action) {
		return Plan{}, fmt.Errorf("provider %s does not support %s", info.Name, request.Action)
	}
	argv := append([]string{}, info.Compose...)
	for _, file := range request.Files {
		argv = append(argv, "--file", file)
	}
	if request.Action == "services" {
		argv = append(argv, "config", "--services")
	} else {
		argv = append(argv, request.Action)
	}
	for _, arg := range request.Arguments {
		if strings.ContainsRune(arg, 0) {
			return Plan{}, fmt.Errorf("NUL in argument")
		}
	}
	argv = append(argv, request.Arguments...)
	return Plan{Provider: info.Name, Action: request.Action, Command: argv, Directory: request.Directory, Context: info.Context, ComposeProvider: info.ComposeProvider, EndpointEnvironment: info.EndpointEnvironment}, nil
}
