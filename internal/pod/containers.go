package pod

import (
	"encoding/json"
	"errors"
	"io"
)

// Container is the shared read-only summary returned by all engine providers.
type Container struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Ports  string `json:"ports"`
}

func DecodeContainers(input io.Reader) ([]Container, error) {
	rows := []Container{}
	decoder := json.NewDecoder(input)
	for {
		var row Container
		if e := decoder.Decode(&row); e == io.EOF {
			return rows, nil
		} else if e != nil {
			return nil, errors.New("container provider returned invalid JSON")
		}
		if row.ID == "" || row.Name == "" {
			return nil, errors.New("container provider returned a summary without an ID or name")
		}
		rows = append(rows, row)
	}
}
