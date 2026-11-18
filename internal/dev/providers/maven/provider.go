package maven

import (
	"encoding/xml"
	"fmt"
	"github.com/yetric/worktools/internal/dev/provider"
	"io"
	"os"
)

type Provider struct{}

var _ provider.Provider = Provider{}

func (Provider) Name() string        { return "maven" }
func (Provider) Manifests() []string { return []string{"pom.xml"} }
func (Provider) Runners() []string   { return []string{"maven"} }

func (Provider) Discover(root string) ([]provider.Action, error) {
	result := []provider.Action{}
	if provider.File(root, "pom.xml") {
		command := "mvn"
		if provider.File(root, "mvnw") {
			command = "./mvnw"
		}
		for _, pair := range [][2]string{{"test", "test"}, {"check", "verify"}, {"build", "package"}, {"clean", "clean"}} {
			result = append(result, provider.NewAction(pair[0], "maven", []string{command, pair[1]}, "pom.xml lifecycle; prefer project wrapper"))
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
			result = append(result, provider.NewAction("start", "maven", []string{command, "spring-boot:run"}, "Spring Boot Maven plugin declared"))
		}
	}
	return result, nil
}
