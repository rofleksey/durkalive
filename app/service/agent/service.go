package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"durkalive/app/client/twitch"
	"durkalive/app/config"
	"durkalive/app/service/memory"

	_ "embed"

	"github.com/samber/do"
	"github.com/sashabaranov/go-openai"
)

//go:embed system_prompt.txt
var systemPromptTemplate string

const (
	defaultTemperature = 1.3
	maxReasonDuration  = 30 * time.Second
	maxMessageLength   = 500
)

type Service struct {
	cfg          *config.Config
	twitchClient *twitch.Client
	memorySvc    *memory.Service

	client      *openai.Client
	chatHistory ChatHistory

	mu            sync.RWMutex
	summary       string
	lastReplyTime time.Time
}

func New(di *do.Injector) (*Service, error) {
	cfg := do.MustInvoke[*config.Config](di)

	clientConfig := openai.DefaultConfig(cfg.OpenAI.Token)
	clientConfig.BaseURL = cfg.OpenAI.BaseURL
	clientConfig.HTTPClient = &http.Client{
		Timeout: 30 * time.Second,
	}
	client := openai.NewClientWithConfig(clientConfig)

	s := &Service{
		cfg:          cfg,
		twitchClient: do.MustInvoke[*twitch.Client](di),
		memorySvc:    do.MustInvoke[*memory.Service](di),
		client:       client,
	}

	return s, nil
}

func (s *Service) ReactStreamerMessage(ctx context.Context, text string) error {
	defer s.chatHistory.add(s.cfg.Twitch.Channel, text)

	return s.processMessage(ctx, s.cfg.Twitch.Channel, text)
}

func (s *Service) ReactChatMessage(ctx context.Context, username, text string) error {
	defer s.chatHistory.add(username, text)

	return s.processMessage(ctx, username, text)
}

func (s *Service) processMessage(ctx context.Context, username, text string) error {
	result, err := s.callAgent(ctx, username, text)
	if err != nil {
		return fmt.Errorf("callAgent: %w", err)
	}

	s.mu.Lock()
	s.summary = result.NewSummary
	s.mu.Unlock()

	if err = s.memorySvc.RemoveFacts(result.RemoveFacts); err != nil {
		return fmt.Errorf("memorySvc.RemoveFacts: %w", err)
	}

	if err = s.memorySvc.AddFacts(result.AddFacts); err != nil {
		return fmt.Errorf("memorySvc.AddFacts: %w", err)
	}

	if result.Response == "" {
		slog.Info("Response is not required")
		return nil
	}

	if len(result.Response) > maxMessageLength {
		return fmt.Errorf("response is too long (%d > %d)", len(result.Response), maxMessageLength)
	}

	if err = s.sendMessage(result.Response); err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	s.chatHistory.add(s.cfg.Twitch.Username, result.Response)

	s.mu.Lock()
	s.lastReplyTime = time.Now()
	s.mu.Unlock()

	return nil
}

func (s *Service) callAgent(ctx context.Context, username, text string) (*DurkaResponse, error) {
	s.mu.RLock()
	summary := s.summary
	lastReplyTime := s.lastReplyTime
	s.mu.RUnlock()

	templateValues := map[string]any{
		"last_message":    fmt.Sprintf("%s - %s: %s", time.Now().Format("15:04:05"), username, text),
		"last_reply_time": lastReplyTime.Format("15:04:05"),
		"channel":         s.cfg.Twitch.Channel,
		"username":        s.cfg.Twitch.Username,
		"chat_history":    s.chatHistory.format(),
		"summary":         summary,
		"facts":           s.memorySvc.Format(),
	}

	prompt := systemPromptTemplate
	for key, value := range templateValues {
		prompt = strings.ReplaceAll(prompt, "{"+key+"}", fmt.Sprint(value))
	}

	ctx, cancel := context.WithTimeout(ctx, maxReasonDuration)
	defer cancel()

	aiResponse, err := s.client.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Model: s.cfg.OpenAI.Model,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: prompt,
				},
			},
			Temperature:         defaultTemperature,
			MaxCompletionTokens: 10000,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create chat completion: %w", err)
	}

	if len(aiResponse.Choices) == 0 {
		return nil, fmt.Errorf("no chat completion found")
	}

	result := aiResponse.Choices[0].Message.Content
	result = strings.Trim(result, "`")
	result = strings.TrimSpace(result)
	result = strings.TrimPrefix(result, "json")
	result = strings.TrimSpace(result)

	var response DurkaResponse
	if err = json.Unmarshal([]byte(result), &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	response.Response = strings.TrimSpace(response.Response)

	return &response, nil
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

func (s *Service) Close() error {
	return nil
}
