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
	return m.tool.Description
}

func (m *mcpToolAdapter) Call(ctx context.Context, input string) (string, error) {
	callRequest := mcp.CallToolRequest{
		Request: mcp.Request{
			Method: "tools/call",
		},
	}

	callRequest.Params.Name = m.tool.Name

	// Parse the input as JSON if it looks like JSON
	var args map[string]interface{}
	if strings.TrimSpace(input)[0] == '{' {
		if err := json.Unmarshal([]byte(input), &args); err == nil {
			callRequest.Params.Arguments = args
		} else {
			// If JSON parsing fails, wrap the input
			callRequest.Params.Arguments = map[string]interface{}{
				"input": input,
			}
		}
	} else {
		// For non-JSON input, try to determine the expected argument
		if len(m.tool.InputSchema.Properties) > 0 {
			// Use the first property name from the schema
			for propName := range m.tool.InputSchema.Properties {
				callRequest.Params.Arguments = map[string]interface{}{
					propName: input,
				}
				break
			}
		} else {
			callRequest.Params.Arguments = map[string]interface{}{
				"input": input,
			}
		}
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
