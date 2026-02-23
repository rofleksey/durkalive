package conversation

import (
	"context"
	"durkalive/app/config"
	"durkalive/app/database"
	"durkalive/app/service/embedding"
	"durkalive/app/service/memory"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sashabaranov/go-openai"
)

const (
	maxRandomFacts                     = 25
	maxSimilarFacts                    = 25
	similarFactsEmbeddingHistoryLength = 5
)

func createClient(cfg config.ModelConfig) *openai.Client {
	clientConfig := openai.DefaultConfig(cfg.Token)

	clientConfig.BaseURL = cfg.BaseURL
	clientConfig.HTTPClient = &http.Client{
		Timeout: 30 * time.Second,
	}

	return openai.NewClientWithConfig(clientConfig)
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "никогда"
	}

	return t.Format("15:04:05")
}

func formatRandomFactsByTags(ctx context.Context, memorySvc *memory.Service, tags, usernames []string) string {
	facts := memorySvc.Search(ctx, []string{}, tags, usernames, maxRandomFacts)
	if len(facts) == 0 {
		return "Нет фактов"
	}

	var builder strings.Builder
	for _, fact := range facts {
		builder.WriteString("id=")
		builder.WriteString(fmt.Sprint(fact.ID))
		builder.WriteString(", content=")
		builder.WriteString(fact.Content)
		builder.WriteString("\n")
	}

	return builder.String()
}

func formatFactsByChatHistory(ctx context.Context, db *database.Service, embeddingSvc *embedding.Service,
	state *State, usernames []string) (string, error) {

	msgHistoryText := state.chatHistory.formatEmbedding(similarFactsEmbeddingHistoryLength)

	msgHistoryEmbedding, err := embeddingSvc.CreateEmbedding(ctx, msgHistoryText)
	if err != nil {
		return "", fmt.Errorf("failed to create embedding: %w", err)
	}

	facts, err := db.FindSimilarFacts(ctx, usernames, msgHistoryEmbedding, 0, maxSimilarFacts)
	if err != nil {
		return "", fmt.Errorf("failed to find similar facts: %w", err)
	}

	if len(facts) == 0 {
		return "Нет фактов", nil
	}

	var builder strings.Builder
	for _, fact := range facts {
		builder.WriteString("id=")
		builder.WriteString(fmt.Sprint(fact.ID))
		builder.WriteString(", content=")
		builder.WriteString(fact.Content)
		builder.WriteString("\n")
	}

	return builder.String(), nil
}
