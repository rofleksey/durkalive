package conversation

import (
	"context"
	"durkalive/app/config"
	"durkalive/app/service/memory"
	"fmt"
	"strings"
	"time"

	_ "embed"

	"github.com/sashabaranov/go-openai"
)

//go:embed reply_prompt_template.txt
var replyPromptTemplate string

type ReplyAgent struct {
	cfg       *config.Config
	memorySvc *memory.Service

	client *openai.Client
	model  string

	state *State
}

func NewReplyAgent(
	cfg *config.Config,
	memorySvc *memory.Service,
	client *openai.Client,
	model string,
	state *State,
) *ReplyAgent {
	return &ReplyAgent{
		cfg:       cfg,
		memorySvc: memorySvc,
		client:    client,
		model:     model,
		state:     state,
	}
}

func (a *ReplyAgent) Call(ctx context.Context, username, text string) (string, error) {
	a.state.mu.RLock()
	summary := a.state.summary
	factsStr := a.memorySvc.Format()
	historyStr := a.state.chatHistory.format()
	a.state.mu.RUnlock()

	now := time.Now()
	templateValues := map[string]any{
		"last_message": fmt.Sprintf("%s - %s: %s", formatTime(now), username, text),
		"channel":      a.cfg.Twitch.Channel,
		"username":     a.cfg.Twitch.Username,
		"chat_history": historyStr,
		"summary":      summary,
		"facts":        factsStr,
	}

	prompt := decisionPromptTemplate
	for key, value := range templateValues {
		prompt = strings.ReplaceAll(prompt, "{"+key+"}", fmt.Sprint(value))
	}

	ctx, cancel := context.WithTimeout(ctx, maxReasonDuration)
	defer cancel()

	aiResponse, err := a.client.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Model: a.model,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: prompt,
				},
			},
			MaxCompletionTokens: 500,
			Temperature:         1.3,
		},
	)
	if err != nil {
		return "", fmt.Errorf("failed to create chat completion: %w", err)
	}

	if len(aiResponse.Choices) == 0 {
		return "", fmt.Errorf("no chat completion found")
	}

	result := aiResponse.Choices[0].Message.Content
	return strings.TrimSpace(result), nil
}
