package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

type mcpToolAdapter struct {
	client client.MCPClient
	tool   mcp.Tool
	name   string
}

func (m *mcpToolAdapter) Name() string {
	return m.name
}

func (m *mcpToolAdapter) Description() string {
	inputSchemaBytes, _ := m.tool.InputSchema.MarshalJSON()
	outputSchemaBytes, _ := m.tool.OutputSchema.MarshalJSON()
	return fmt.Sprintf("%s - JSON INPUT SCHEMA: %s - JSON OUTPUT SCHEMA: %s", m.tool.Description, string(inputSchemaBytes), string(outputSchemaBytes))
}

func (m *mcpToolAdapter) Call(ctx context.Context, input string) (string, error) {
	input = strings.TrimSpace(input)

	var jsonData any
	if err := json.Unmarshal([]byte(input), &jsonData); err != nil {
		jsonData = input
	}

	callRequest := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      m.tool.Name,
			Arguments: jsonData,
		},
		Request: mcp.Request{
			Method: "tools/call",
		},
	}

	response, err := m.client.CallTool(ctx, callRequest)
	if err != nil {
		return "", fmt.Errorf("MCP tool call failed: %w", err)
	}

	var result strings.Builder
	for _, content := range response.Content {
		if textContent, ok := content.(mcp.TextContent); ok {
			result.WriteString(textContent.Text)
			result.WriteString("\n")
		}
	}

	return strings.TrimSpace(result.String()), nil
}
