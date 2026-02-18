package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"durkalive/app/client/twitch"
	"durkalive/app/config"

	"github.com/mark3labs/mcp-go/client"
	"github.com/samber/do"
	"github.com/tmc/langchaingo/agents"
	"github.com/tmc/langchaingo/chains"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
	"github.com/tmc/langchaingo/memory"
	"github.com/tmc/langchaingo/tools"
)

const (
	responseCooldown = 2 * time.Second
	maxMessageLength = 500
)

type Service struct {
	cfg          *config.Config
	twitchClient *twitch.Client
	llm          llms.Model
	executor     *agents.Executor
	memory       *memory.ChatMessageHistory
	mcpClients   []*mcpClientWrapper
	lastResponse time.Time
	mu           sync.RWMutex
}

type mcpClientWrapper struct {
	client client.MCPClient
	tools  []tools.Tool
	name   string
}

func New(di *do.Injector) (*Service, error) {
	cfg := do.MustInvoke[*config.Config](di)

	llm, err := openai.New(
		openai.WithModel(cfg.OpenAI.Model),
		openai.WithBaseURL(cfg.OpenAI.BaseURL),
		openai.WithToken(cfg.OpenAI.Token),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create LLM: %w", err)
	}

	s := &Service{
		cfg:          cfg,
		twitchClient: do.MustInvoke[*twitch.Client](di),
		llm:          llm,
		memory:       memory.NewChatMessageHistory(),
	}

	if err = s.initializeMCPClients(); err != nil {
		return nil, fmt.Errorf("failed to initialize MCP clients: %w", err)
	}

	if err = s.initializeAgent(); err != nil {
		return nil, fmt.Errorf("failed to initialize executor: %w", err)
	}

	return s, nil
}

func (s *Service) ReactStreamerMessage(ctx context.Context, text string) error {
	return s.processMessage(ctx, "streamer", text, false)
}

func (s *Service) ReactChatMessage(ctx context.Context, username, text string) error {
	return s.processMessage(ctx, username, text, true)
}

func (s *Service) processMessage(ctx context.Context, username, text string, isChat bool) error {
	s.mu.RLock()
	lastTime := s.lastResponse
	s.mu.RUnlock()

	if time.Since(lastTime) < responseCooldown {
		return nil
	}

	shouldRespond := s.shouldRespond(username, text, isChat)

	if err := s.memory.AddUserMessage(ctx, fmt.Sprintf("%s: %s", username, text)); err != nil {
		return fmt.Errorf("failed to add user message: %w", err)
	}

	if !shouldRespond {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	response, err := chains.Run(ctx, s.executor, s.buildPrompt(username, text))
	if err != nil {
		return fmt.Errorf("failed to run LLM chain: %w", err)
	}

	response = strings.TrimSpace(response)
	if response == "" {
		return fmt.Errorf("empty response from LLM chain")
	}
	if len(response) > maxMessageLength {
		return fmt.Errorf("response is too long (%d > %d)", len(response), maxMessageLength)
	}

	if err = s.sendMessage(response); err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	s.mu.Lock()
	s.lastResponse = time.Now()
	s.mu.Unlock()

	if err = s.memory.AddAIMessage(ctx, response); err != nil {
		return fmt.Errorf("failed to add AI message: %w", err)
	}

	return nil
}

func (s *Service) sendMessage(text string) error {
	if s.cfg.Twitch.DisableNotifications {
		slog.Info("Replied to message (notifications disabled)", "text", text, "telegram", true)
		return nil
	}

	if err := s.twitchClient.SendMessage(s.cfg.Twitch.Channel, text); err != nil {
		return fmt.Errorf("failed to send message to twitch: %w", err)
	}

	slog.Info("Replied to message", "text", text, "telegram", true)

	return nil
}

func (s *Service) shouldRespond(username, text string, isChat bool) bool {
	return true
}

func (s *Service) buildPrompt(username, text string) string {
	return fmt.Sprintf(`[Сообщение от %s]: %s

Если тебе нужно запомнить что-то об этом пользователе или разговоре, используй memory tool.`, username, text)
}

func (s *Service) Close() error {
	for _, wrapper := range s.mcpClients {
		if err := wrapper.client.Close(); err != nil {
			fmt.Printf("Error closing MCP client %s: %v\n", wrapper.name, err)
		}
	}
	return nil
}
