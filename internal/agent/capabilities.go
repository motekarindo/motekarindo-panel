package agent

import (
	"encoding/json"
	"strings"
)

type Capabilities struct {
	Actions []string `json:"actions"`
}

func DefaultCapabilities() Capabilities {
	return Capabilities{
		Actions: DefaultRegistry().Actions(),
	}
}

func (c Capabilities) String() string {
	encoded, err := json.Marshal(c)
	if err != nil {
		return `{"actions":[]}`
	}
	return strings.TrimSpace(string(encoded))
}
