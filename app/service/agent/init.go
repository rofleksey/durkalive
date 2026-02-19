package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tmc/langchaingo/tools"
)

func (s *Service) createMCPClient(command string, args ...string) (client.MCPClient, error) {
	return client.NewStdioMCPClient(
		command,
		nil,
		args...,
	)
}

func (s *Service) initializeMCPClients() error {
	mcpServers := []struct {
		name    string
		command string
		args    []string
	}{
		{
			name:    "memory",
			command: "docker",
			args:    []string{"run", "--rm", "-i", "mcp/memory"},
		},
	}

	for _, server := range mcpServers {
		mcpClient, err := s.createMCPClient(server.command, server.args...)
		if err != nil {
			return fmt.Errorf("failed to create MCP client for %s: %w", server.name, err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		initRequest := mcp.InitializeRequest{}
		initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
		initRequest.Params.ClientInfo = mcp.Implementation{
			Name:    "twitch-bot-executor",
			Version: "1.0.0",
		}

		_, err = mcpClient.Initialize(ctx, initRequest)
		if err != nil {
			return fmt.Errorf("failed to initialize MCP client %s: %w", server.name, err)
		}

		toolsRequest := mcp.ListToolsRequest{}
		toolsResponse, err := mcpClient.ListTools(ctx, toolsRequest)
		if err != nil {
			return fmt.Errorf("failed to list tools from %s: %w", server.name, err)
		}

		langchainTools := make([]tools.Tool, 0, len(toolsResponse.Tools))
		for _, mcpTool := range toolsResponse.Tools {
			tool := &mcpToolAdapter{
				client: mcpClient,
				tool:   mcpTool,
				name:   fmt.Sprintf("%s_%s", server.name, mcpTool.Name),
			}
			langchainTools = append(langchainTools, tool)
		}

		s.mcpClients = append(s.mcpClients, &mcpClientWrapper{
			client: mcpClient,
			tools:  langchainTools,
			name:   server.name,
		})
	}

	return nil
}
