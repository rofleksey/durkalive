package conversation

import (
	"context"
	"durkalive/app/config"
	"durkalive/app/service/memory"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "embed"

	"github.com/sashabaranov/go-openai"
)

//go:embed decision_prompt_template.txt
var decisionPromptTemplate string

type DecisionAgent struct {
	cfg       *config.Config
	memorySvc *memory.Service

	client *openai.Client
	model  string

	state *State
}

func NewDecisionAgent(
	cfg *config.Config,
	memorySvc *memory.Service,
	client *openai.Client,
	model string,
	state *State,
) *DecisionAgent {
	return &DecisionAgent{
		cfg:       cfg,
		memorySvc: memorySvc,
		client:    client,
		model:     model,
		state:     state,
	}
}

func (a *DecisionAgent) Call(ctx context.Context, username, text string) (*DecisionResponse, error) {
	a.state.mu.RLock()
	lastReplyTime := a.state.lastReplyTime
	factsStr := a.memorySvc.Format()
	historyStr := a.state.chatHistory.format()
	a.state.mu.RUnlock()

	now := time.Now()

	var lastReply string
	if lastReplyTime.IsZero() {
		lastReply = "Ты еще не писал сообщений в чат"
	} else {
		lastReply = fmt.Sprintf("Ты отвечал %d секунд назад", int(now.Sub(lastReplyTime).Seconds()))
	}

	templateValues := map[string]any{
		"last_message": fmt.Sprintf("%s - %s: %s", formatTime(now), username, text),
		"last_reply":   lastReply,
		"now":          formatTime(now),
		"channel":      a.cfg.Twitch.Channel,
		"username":     a.cfg.Twitch.Username,
		"chat_history": historyStr,
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
			MaxCompletionTokens: 1000,
			Temperature:         1,
			ResponseFormat: &openai.ChatCompletionResponseFormat{
				Type: openai.ChatCompletionResponseFormatTypeJSONObject,
			},
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

	var response DecisionResponse
	if err = json.Unmarshal([]byte(result), &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &response, nil
}
