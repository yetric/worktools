package gradle

import (
	"github.com/yetric/worktools/internal/dev/provider"
	"regexp"
	"strings"
)

type Provider struct{}

var _ provider.Provider = Provider{}

func (Provider) Name() string { return "gradle" }
func (Provider) Manifests() []string {
	return []string{"build.gradle", "build.gradle.kts", "settings.gradle", "settings.gradle.kts"}
}
func (Provider) Runners() []string { return []string{"gradle"} }

func (Provider) Discover(root string) ([]provider.Action, error) {
	result := []provider.Action{}
	gradle := []string{}
	for _, name := range []string{"build.gradle", "build.gradle.kts"} {
		if provider.File(root, name) {
			text, e := provider.Read(root, name)
			if e != nil {
				return nil, e
			}
			gradle = append(gradle, text)
		}
	}
	if len(gradle) > 0 {
		command := "gradle"
		if provider.File(root, "gradlew") {
			command = "./gradlew"
		}
		for _, name := range []string{"test", "check", "build", "clean"} {
			result = append(result, provider.NewAction(name, "gradle", []string{command, name}, "Gradle conventional lifecycle; prefer project wrapper"))
		}
		text := regexp.MustCompile(`(?s)/\*.*?\*/|//[^\n]*`).ReplaceAllString(strings.Join(gradle, "\n"), "")
		if regexp.MustCompile(`["']org\.springframework\.boot["']`).MatchString(text) {
			result = append(result, provider.NewAction("start", "gradle", []string{command, "bootRun"}, "Spring Boot Gradle plugin declared"))
		} else if regexp.MustCompile(`\bapplication\b`).MatchString(text) {
			result = append(result, provider.NewAction("start", "gradle", []string{command, "run"}, "Gradle application plugin convention"))
		}
	}
	return result, nil
}
