// Package provider defines the standalone development-provider contract.
// Implementations depend on this package, never on the CLI or resolver.
package provider

import (
	"os"
	"path/filepath"
	"regexp"
)

type Action struct {
	Name        string   `json:"name"`
	Runner      string   `json:"runner"`
	Command     []string `json:"command"`
	Description string   `json:"description"`
	Reason      string   `json:"reason"`
	Separator   []string `json:"argument_separator,omitempty"`
	Directory   string   `json:"directory"`
}

// Provider inspects project files without executing commands or changing state.
type Provider interface {
	Name() string
	Manifests() []string
	Runners() []string
	Discover(root string) ([]Action, error)
}

var actionName = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9_.:/-]*$`)

func ValidName(name string) bool { return actionName.MatchString(name) }
func File(root, name string) bool {
	s, e := os.Stat(filepath.Join(root, name))
	return e == nil && !s.IsDir()
}
func Read(root, name string) (string, error) {
	b, e := os.ReadFile(filepath.Join(root, name))
	return string(b), e
}
func Contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
func NewAction(name, runner string, command []string, reason string) Action {
	return Action{Name: name, Runner: runner, Command: command, Description: runner + " " + name, Reason: reason}
}
