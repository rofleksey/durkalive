package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tmc/langchaingo/agents"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/memory"
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

func (s *Service) initializeAgent() error {
	var allTools []tools.Tool
	for _, wrapper := range s.mcpClients {
		allTools = append(allTools, wrapper.tools...)
	}

	systemPrompt := `Привет! Ты — полезный помощник для чата Twitch, созданный для стримера Dead by Daylight.

ВАЖНЫЕ ПРАВИЛА:
* Постарайся отвечать на русском языке
* Будь дружелюбным и вовлекай сообщество в общение.
* Ты можешь использовать инструменты памяти MCP, чтобы запоминать информацию о зрителях и детали разговоров.
* Старайся отвечать кратко и уместно для чата Twitch.
* Если ты в чём-то не уверен или какой-то инструмент не сработал — не отвечай вообще.
* Отвечай, только если тебя упомянули, задали прямой вопрос или если сообщение содержит важные ключевые слова.
* Не реагируй на каждое сообщение — будь избирательным.
* Учитывай контекст недавних сообщений в чате.`

	if err := s.memory.AddMessage(context.Background(), llms.SystemChatMessage{
		Content: systemPrompt,
	}); err != nil {
		return fmt.Errorf("failed to add system chat message: %w", err)
	}

	agent := agents.NewConversationalAgent(
		s.llm,
		allTools,
	)

	executor := agents.NewExecutor(
		agent,
		agents.WithMemory(memory.NewConversationWindowBuffer(5,
			memory.WithChatHistory(s.memory),
		)),
		agents.WithMaxIterations(3),
		agents.WithCallbacksHandler(&LogCallbackHandler{}),
	)

	s.executor = executor

	return nil
}
