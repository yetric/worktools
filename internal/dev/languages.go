package dev

import (
	"encoding/xml"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

func discoverGo(root string) ([]Action, error) {
	result := []Action{action("test", "go", []string{"go", "test", "./..."}, "go.mod default"), action("check", "go", []string{"go", "vet", "./..."}, "go.mod default"), action("build", "go", []string{"go", "build", "./..."}, "go.mod default")}
	hasMain := func(directory string) bool {
		files, _ := filepath.Glob(filepath.Join(root, directory, "*.go"))
		for _, file := range files {
			if strings.HasSuffix(file, "_test.go") {
				continue
			}
			parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.PackageClauseOnly)
			if err == nil && parsed.Name.Name == "main" {
				return true
			}
		}
		return false
	}
	if hasMain(".") {
		result = append(result, action("start", "go", []string{"go", "run", "."}, "Root Go main package"))
	} else {
		entries, _ := os.ReadDir(filepath.Join(root, "cmd"))
		commands := []string{}
		for _, entry := range entries {
			if entry.IsDir() && hasMain(filepath.Join("cmd", entry.Name())) {
				commands = append(commands, "./cmd/"+entry.Name())
			}
		}
		if len(commands) == 1 {
			result = append(result, action("start", "go", []string{"go", "run", commands[0]}, "Single Go command under cmd/"))
		}
	}
	return result, nil
}

func discoverRust(root string) ([]Action, error) {
	var manifest struct {
		Package struct {
			Name       string
			DefaultRun string `toml:"default-run"`
		}
		Bin []struct{ Name, Path string }
	}
	if _, e := toml.DecodeFile(root+"/Cargo.toml", &manifest); e != nil {
		return nil, e
	}
	result := []Action{}
	for _, name := range []string{"test", "check", "build", "clean"} {
		result = append(result, action(name, "cargo", []string{"cargo", name}, "Cargo.toml lifecycle"))
	}
	count := len(manifest.Bin)
	mainDeclared := false
	for _, b := range manifest.Bin {
		if b.Path == "src/main.rs" || b.Name == manifest.Package.Name {
			mainDeclared = true
		}
	}
	if File(root, "src/main.rs") && !mainDeclared {
		count++
	}
	if manifest.Package.DefaultRun != "" || (manifest.Package.Name != "" && count == 1) {
		result = append(result, action("start", "cargo", []string{"cargo", "run"}, "Cargo binary or package.default-run"))
	}
	return result, nil
}

func discoverJava(root string) ([]Action, error) {
	result := []Action{}
	if File(root, "pom.xml") {
		command := "mvn"
		if File(root, "mvnw") {
			command = "./mvnw"
		}
		for _, pair := range [][2]string{{"test", "test"}, {"check", "verify"}, {"build", "package"}, {"clean", "clean"}} {
			result = append(result, action(pair[0], "maven", []string{command, pair[1]}, "pom.xml lifecycle; prefer project wrapper"))
		}
		f, e := os.Open(root + "/pom.xml")
		if e != nil {
			return nil, e
		}
		defer f.Close()
		decoder := xml.NewDecoder(f)
		plugins := 0
		spring := false
		for {
			token, e := decoder.Token()
			if e == io.EOF {
				break
			}
			if e != nil {
				return nil, fmt.Errorf("invalid pom.xml: %w", e)
			}
			switch t := token.(type) {
			case xml.StartElement:
				if t.Name.Local == "plugin" {
					plugins++
				}
				if plugins > 0 && t.Name.Local == "artifactId" {
					var name string
					if e := decoder.DecodeElement(&name, &t); e != nil {
						return nil, e
					}
					spring = spring || name == "spring-boot-maven-plugin"
				}
			case xml.EndElement:
				if t.Name.Local == "plugin" {
					plugins--
				}
			}
		}
		if spring {
			result = append(result, action("start", "maven", []string{command, "spring-boot:run"}, "Spring Boot Maven plugin declared"))
		}
	}
	gradle := []string{}
	for _, name := range []string{"build.gradle", "build.gradle.kts"} {
		if File(root, name) {
			text, e := read(root, name)
			if e != nil {
				return nil, e
			}
			gradle = append(gradle, text)
		}
	}
	if len(gradle) > 0 {
		command := "gradle"
		if File(root, "gradlew") {
			command = "./gradlew"
		}
		for _, name := range []string{"test", "check", "build", "clean"} {
			result = append(result, action(name, "gradle", []string{command, name}, "Gradle conventional lifecycle; prefer project wrapper"))
		}
		text := regexp.MustCompile(`(?s)/\*.*?\*/|//[^\n]*`).ReplaceAllString(strings.Join(gradle, "\n"), "")
		if regexp.MustCompile(`["']org\.springframework\.boot["']`).MatchString(text) {
			result = append(result, action("start", "gradle", []string{command, "bootRun"}, "Spring Boot Gradle plugin declared"))
		} else if regexp.MustCompile(`\bapplication\b`).MatchString(text) {
			result = append(result, action("start", "gradle", []string{command, "run"}, "Gradle application plugin convention"))
		}
	}
	return result, nil
}
