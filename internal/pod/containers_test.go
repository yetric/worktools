package pod

import (
	"strings"
	"testing"
)

func TestDecodeContainerSummaries(t *testing.T) {
	rows, e := DecodeContainers(strings.NewReader("\n"))
	if e != nil || rows == nil || len(rows) != 0 {
		t.Fatal(rows, e)
	}
	rows, e = DecodeContainers(strings.NewReader(`{"id":"123","name":"app","status":"Up","ports":""}` + "\n" + `{"id":"456","name":"db","status":"Exited","ports":"127.0.0.1:3306"}`))
	if e != nil || len(rows) != 2 || rows[1].Name != "db" {
		t.Fatal(rows, e)
	}
	for _, value := range []string{`not json`, `null`, `{}`, `{"id":"123"}`, `{"id":"123","name":[]}`, `{"id":"123","name":"app"} broken`} {
		if _, e := DecodeContainers(strings.NewReader(value)); e == nil {
			t.Fatal("accepted invalid container output", value)
		}
	}
}
