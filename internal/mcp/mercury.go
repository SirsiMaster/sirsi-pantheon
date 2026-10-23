package mcp

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/SirsiMaster/sirsi-pantheon/internal/mercury"
)

type mercuryReadFunc func(context.Context) mercury.Read

func registerMercuryTool(s *Server) {
	additionalProperties := false
	s.RegisterTool(Tool{
		Name:        "mercury_status",
		Description: "Read the current Mercury interconnect dashboard snapshot. Read-only: it has no endpoint override or run controls.",
		InputSchema: InputSchema{
			Type:                 "object",
			Properties:           map[string]SchemaField{},
			AdditionalProperties: &additionalProperties,
		},
	}, handleMercuryStatus)
}

func handleMercuryStatus(args map[string]interface{}) (*ToolResult, error) {
	return readMercuryStatus(args, func(ctx context.Context) mercury.Read {
		return mercury.NewConfigured().Read(ctx)
	})
}

func readMercuryStatus(args map[string]interface{}, read mercuryReadFunc) (*ToolResult, error) {
	if len(args) != 0 {
		return textResult("Error: mercury_status does not accept arguments", true), nil
	}
	if read == nil {
		return nil, errors.New("Mercury reader is unavailable")
	}

	result := read(context.Background())
	raw, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return &ToolResult{Content: []ContentBlock{{Type: "text", MimeType: "application/json", Text: string(raw)}}}, nil
}
