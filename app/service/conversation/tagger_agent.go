package conversation

import (
	"context"
	"durkalive/app/config"
	_ "embed"
	"fmt"
	"strings"
	"time"

	"github.com/samber/do"
	"github.com/sashabaranov/go-openai"
)

//go:embed tagger_prompt_template.txt
var taggerPromptTemplate string

type TaggerAgent struct {
	cfg *config.Config

	client *openai.Client
	model  string

	state *State
}

func NewTaggerAgent(
	di *do.Injector,
	client *openai.Client,
	model string,
	state *State,
) *TaggerAgent {
	return &TaggerAgent{
		cfg:    do.MustInvoke[*config.Config](di),
		client: client,
		model:  model,
		state:  state,
	}
}

func (a *TaggerAgent) Call(ctx context.Context, username, text string) ([]string, error) {
	a.state.mu.RLock()
	historyStr := a.state.chatHistory.format()
	a.state.mu.RUnlock()

	now := time.Now()

	templateValues := map[string]any{
		"channel":      a.cfg.Twitch.Channel,
		"username":     a.cfg.Twitch.Username,
		"chat_history": historyStr,
		"last_message": fmt.Sprintf("%s - %s: %s", formatTime(now), username, text),
	}

	prompt := taggerPromptTemplate
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
			MaxCompletionTokens: 256,
			Temperature:         1,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create chat completion: %w", err)
	}

	if len(aiResponse.Choices) == 0 {
		return nil, fmt.Errorf("no chat completion found")
	}

	result := aiResponse.Choices[0].Message.Content
	result = strings.TrimSpace(result)
	if result == "" {
		return nil, nil
	}

	return strings.Split(result, ","), nil
}
