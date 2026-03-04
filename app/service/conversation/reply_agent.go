package conversation

import (
	"context"
	"durkalive/app/config"
	"durkalive/app/database"
	"durkalive/app/service/embedding"
	"durkalive/app/service/recentmemory"
	"fmt"
	"strings"
	"time"

	_ "embed"

	"github.com/samber/do"
	"github.com/sashabaranov/go-openai"
)

//go:embed reply_prompt_template.txt
var replyPromptTemplate string

type ReplyAgent struct {
	cfg             *config.Config
	db              *database.Service
	embeddingSvc    *embedding.Service
	recentMemorySvc *recentmemory.Service

	client *openai.Client
	model  string

	state *State
}

func NewReplyAgent(
	di *do.Injector,
	client *openai.Client,
	model string,
	state *State,
	recentMemorySvc *recentmemory.Service,
) *ReplyAgent {
	return &ReplyAgent{
		cfg:             do.MustInvoke[*config.Config](di),
		db:              do.MustInvoke[*database.Service](di),
		embeddingSvc:    do.MustInvoke[*embedding.Service](di),
		recentMemorySvc: recentMemorySvc,
		client:          client,
		model:           model,
		state:           state,
	}
}

func (a *ReplyAgent) Call(ctx context.Context, username, text string, tags, usernames []string) (string, error) {
	a.state.mu.RLock()
	similarFactsStr, err := formatFactsByChatHistory(ctx, a.db, a.embeddingSvc, a.state, usernames)
	if err != nil {
		a.state.mu.RUnlock()
		return "", fmt.Errorf("failed to format similar facts: %w", err)
	}
	historyStr := a.state.chatHistory.format()
	a.state.mu.RUnlock()

	recentMemoryStr := "Нет записей"
	if a.recentMemorySvc != nil {
		recentMemoryStr = a.recentMemorySvc.Format()
	}
	now := time.Now()
	templateValues := map[string]any{
		"last_message":  fmt.Sprintf("%s - %s: %s", formatTime(now), username, text),
		"channel":       a.cfg.Twitch.Channel,
		"username":      a.cfg.Twitch.Username,
		"chat_history":  historyStr,
		"recent_memory": recentMemoryStr,
		"similar_facts": similarFactsStr,
	}

	prompt := replyPromptTemplate
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
			Temperature:         1.0,
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
