package stacklab

import "fmt"

const LocalSchema = "sirsi.stacklab.local.v1"

type LocalState struct {
	Schema   string `json:"schema"`
	Enabled  bool   `json:"enabled"`
	Provider string `json:"provider"`
}

func defaultLocalState() LocalState { return LocalState{Schema: LocalSchema, Provider: "local"} }
func (s LocalState) Validate() error {
	if s.Schema != LocalSchema || s.Provider != "local" && s.Provider != "frontier" && s.Provider != "self-hosted" {
		return fmt.Errorf("Stack Lab local state: unsupported schema/provider; no provider was invoked")
	}
	return nil
}
