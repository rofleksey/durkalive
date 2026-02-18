package agent

import (
	"context"
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
	callRequest.Params.Arguments = map[string]interface{}{
		"input": input,
	}

	response, err := m.client.CallTool(ctx, callRequest)
	if err != nil {
		return "", fmt.Errorf("MCP tool call failed: %w", err)
	}

	var result strings.Builder
	for _, content := range response.Content {
		if textContent, ok := content.(mcp.TextContent); ok {
			result.WriteString(textContent.Text)
			result.WriteString("\n\n")
		}
	}

	return result.String(), nil
}
