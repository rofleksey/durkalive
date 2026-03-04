package conversation

import (
	"context"
	"durkalive/app/config"
	"fmt"
	"strings"
	"time"

	_ "embed"

	"github.com/samber/do"
	"github.com/sashabaranov/go-openai"
)

//go:embed review_prompt_template.txt
var reviewPromptTemplate string

type ReviewAgent struct {
	cfg *config.Config

	client *openai.Client
	model  string
}

func NewReviewAgent(di *do.Injector, client *openai.Client, model string) *ReviewAgent {
	return &ReviewAgent{
		cfg:    do.MustInvoke[*config.Config](di),
		client: client,
		model:  model,
	}
}

// Approve returns true if the reply is acceptable to send (on-topic, not bot-like, etc.).
func (a *ReviewAgent) Approve(ctx context.Context, lastMessage, replyCandidate, contextLines string) (bool, error) {
	prompt := reviewPromptTemplate
	prompt = strings.ReplaceAll(prompt, "{last_message}", lastMessage)
	prompt = strings.ReplaceAll(prompt, "{reply_candidate}", replyCandidate)
	prompt = strings.ReplaceAll(prompt, "{context}", contextLines)

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	resp, err := a.client.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Model: a.model,
			Messages: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleUser, Content: prompt},
			},
			MaxCompletionTokens: 80,
			Temperature:         0,
		},
	)
	if err != nil {
		return false, fmt.Errorf("review completion: %w", err)
	}
	if len(resp.Choices) == 0 {
		return false, fmt.Errorf("no review completion choice")
	}

	text := strings.TrimSpace(strings.ToUpper(resp.Choices[0].Message.Content))
	if idx := strings.Index(text, " "); idx > 0 {
		text = text[:idx]
	}
	return text == "YES", nil
}
