package conversation

import (
	"context"
	"durkalive/app/config"
	"durkalive/app/database"
	"durkalive/app/service/embedding"
	"durkalive/app/service/recentmemory"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "embed"

	"github.com/samber/do"
	"github.com/sashabaranov/go-openai"
)

//go:embed decision_prompt_template.txt
var decisionPromptTemplate string

type DecisionAgent struct {
	cfg             *config.Config
	db              *database.Service
	embeddingSvc    *embedding.Service
	recentMemorySvc *recentmemory.Service

	client *openai.Client
	model  string

	state *State
}

func NewDecisionAgent(
	di *do.Injector,
	client *openai.Client,
	model string,
	state *State,
) *DecisionAgent {
	return &DecisionAgent{
		cfg:             do.MustInvoke[*config.Config](di),
		db:              do.MustInvoke[*database.Service](di),
		embeddingSvc:    do.MustInvoke[*embedding.Service](di),
		recentMemorySvc: do.MustInvoke[*recentmemory.Service](di),
		client:          client,
		model:           model,
		state:           state,
	}
}

func (a *DecisionAgent) Call(ctx context.Context, username, text string, usernames []string) (*DecisionResponse, error) {
	a.state.mu.RLock()
	lastReplyTime := a.state.lastReplyTime
	similarFactsStr, err := formatFactsByChatHistory(ctx, a.db, a.embeddingSvc, a.state, usernames)
	if err != nil {
		a.state.mu.RUnlock()
		return nil, fmt.Errorf("failed to format similar facts: %w", err)
	}
	historyStr := a.state.chatHistory.format()
	a.state.mu.RUnlock()

	now := time.Now()

	var lastReply string
	if lastReplyTime.IsZero() {
		lastReply = "Ты еще не писал сообщений в чат"
	} else {
		lastReply = fmt.Sprintf("Ты отвечал %d секунд назад", int(now.Sub(lastReplyTime).Seconds()))
	}

	recentMemoryStr := "Нет записей"
	if a.recentMemorySvc != nil {
		recentMemoryStr = a.recentMemorySvc.Format()
	}
	templateValues := map[string]any{
		"last_message":  fmt.Sprintf("%s - %s: %s", formatTime(now), username, text),
		"last_reply":    lastReply,
		"now":           formatTime(now),
		"channel":       a.cfg.Twitch.Channel,
		"username":      a.cfg.Twitch.Username,
		"chat_history":  historyStr,
		"recent_memory": recentMemoryStr,
		"similar_facts": similarFactsStr,
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
